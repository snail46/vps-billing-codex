package portforward

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vps-billing/backend/internal/operation"
	providercontract "vps-billing/backend/internal/provider"
)

type input struct {
	Protocol    string `json:"protocol"`
	PublicPort  int    `json:"public_port"`
	GuestPort   int    `json:"guest_port"`
	Description string `json:"description"`
}

type target struct {
	operationType, providerInstanceID, providerNodeID, publicIP string
	operationID, resourceID, instanceID, providerID, userID     uuid.UUID
	providerMappingID                                           string
	request                                                     input
}

type Repository struct{ pool *pgxpool.Pool }

type persistenceError struct {
	code string
	err  error
}

func (e *persistenceError) Error() string { return e.err.Error() }
func (e *persistenceError) Unwrap() error { return e.err }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) context(ctx context.Context, operationID uuid.UUID) (target, error) {
	var value target
	var raw []byte
	var userID *uuid.UUID
	var providerID, instanceID *uuid.UUID
	var providerInstanceID, providerNodeID, publicIP, mappingID *string
	err := r.pool.QueryRow(ctx, `SELECT o.type,o.id,o.resource_id,o.input,o.user_id,
		CASE WHEN o.type='port_forward_add' THEN i.id ELSE di.id END,
		CASE WHEN o.type='port_forward_add' THEN i.provider_id ELSE di.provider_id END,
		CASE WHEN o.type='port_forward_add' THEN i.provider_instance_id ELSE di.provider_instance_id END,
		CASE WHEN o.type='port_forward_add' THEN n.provider_node_id ELSE dn.provider_node_id END,
		CASE WHEN o.type='port_forward_add' THEN host(COALESCE(i.primary_ipv4,(SELECT x.address FROM instance_networks x WHERE x.instance_id=i.id AND x.type IN ('shared_ipv4','ipv4') ORDER BY CASE x.type WHEN 'shared_ipv4' THEN 0 ELSE 1 END LIMIT 1))) ELSE host(dpf.public_ip) END,
		dpf.provider_mapping_id
		FROM operations o
		LEFT JOIN instances i ON o.type='port_forward_add' AND i.id=o.resource_id
		LEFT JOIN nodes n ON n.id=i.node_id
		LEFT JOIN port_forwards dpf ON o.type='port_forward_delete' AND dpf.id=o.resource_id
		LEFT JOIN instances di ON di.id=dpf.instance_id
		LEFT JOIN nodes dn ON dn.id=di.node_id
		WHERE o.id=$1`, operationID).Scan(&value.operationType, &value.operationID, &value.resourceID, &raw, &userID, &instanceID, &providerID, &providerInstanceID, &providerNodeID, &publicIP, &mappingID)
	if err != nil {
		return target{}, err
	}
	if userID == nil || providerID == nil || instanceID == nil || providerInstanceID == nil || providerNodeID == nil || publicIP == nil {
		return target{}, errors.New("port forward placement is incomplete")
	}
	value.userID, value.providerID, value.instanceID = *userID, *providerID, *instanceID
	value.providerInstanceID, value.providerNodeID, value.publicIP = *providerInstanceID, *providerNodeID, *publicIP
	if mappingID != nil {
		value.providerMappingID = *mappingID
	}
	if value.operationType == "port_forward_add" {
		if err := json.Unmarshal(raw, &value.request); err != nil {
			return target{}, fmt.Errorf("decode port forward input: %w", err)
		}
	}
	return value, nil
}

func (r *Repository) validateAdd(ctx context.Context, value target) error {
	var quota, active int
	var supported bool
	err := r.pool.QueryRow(ctx, `SELECT pl.nat_port_count,
		(SELECT count(*) FROM port_forwards pf WHERE pf.instance_id=i.id AND pf.status<>'deleted'),
		COALESCE((p.capabilities->>'port_forward')::boolean,(p.capabilities->>'nat')::boolean,false)
		FROM instances i JOIN subscriptions s ON s.id=i.subscription_id JOIN plans pl ON pl.id=s.plan_id JOIN providers p ON p.id=i.provider_id
		WHERE i.id=$1 AND s.user_id=$2 AND i.deleted_at IS NULL`, value.instanceID, value.userID).Scan(&quota, &active, &supported)
	if err != nil {
		return err
	}
	if !supported {
		return &operation.WorkflowError{Code: providercontract.ErrorUnsupportedOperation, MessageKey: "operation.portForward.unsupported", Retryable: false}
	}
	if quota <= 0 || active >= quota {
		return &operation.WorkflowError{Code: "PORT_FORWARD_QUOTA_EXCEEDED", MessageKey: "operation.portForward.quotaExceeded", Retryable: false}
	}
	return nil
}

func (r *Repository) persistAdd(ctx context.Context, value target, mapping providercontract.PortForward) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	portID := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO port_forwards(id,instance_id,protocol,public_ip,public_port,guest_port,description,status,provider_mapping_id,operation_id)
		VALUES($1,$2,$3,$4::inet,$5,$6,$7,'active',$8,$9)
		ON CONFLICT(operation_id) WHERE operation_id IS NOT NULL DO UPDATE SET status='active',provider_mapping_id=EXCLUDED.provider_mapping_id,error_code=NULL,updated_at=now()`, portID, value.instanceID, mapping.Protocol, value.publicIP, mapping.PublicPort, mapping.GuestPort, mapping.Description, mapping.ProviderMappingID, value.operationID)
	if err != nil {
		return &persistenceError{code: "PORT_FORWARD_MAPPING_PERSIST_FAILED", err: fmt.Errorf("insert port forward: %w", err)}
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,actor_type,actor_id,action,resource_type,resource_id,after_data,trace_id)
		SELECT $1,'user',$2,'port_forward.created','port_forward',$3,jsonb_build_object('instance_id',$4::uuid,'protocol',$5::text,'public_port',$6::integer,'guest_port',$7::integer),trace_id FROM operations WHERE id=$8`, uuid.New(), value.userID, portID, value.instanceID, mapping.Protocol, mapping.PublicPort, mapping.GuestPort, value.operationID)
	if err != nil {
		return &persistenceError{code: "PORT_FORWARD_AUDIT_FAILED", err: fmt.Errorf("audit port forward creation: %w", err)}
	}
	if err := tx.Commit(ctx); err != nil {
		return &persistenceError{code: "PORT_FORWARD_COMMIT_FAILED", err: fmt.Errorf("commit port forward creation: %w", err)}
	}
	return nil
}

func (r *Repository) persistDelete(ctx context.Context, value target) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `UPDATE port_forwards SET status='deleted',operation_id=$2,error_code=NULL,updated_at=now() WHERE id=$1 AND status<>'deleted'`, value.resourceID, value.operationID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,actor_type,actor_id,action,resource_type,resource_id,after_data,trace_id)
		SELECT $1,'user',$2,'port_forward.deleted','port_forward',$3,jsonb_build_object('instance_id',$4::uuid),trace_id FROM operations WHERE id=$5`, uuid.New(), value.userID, value.resourceID, value.instanceID, value.operationID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type Workflow struct {
	repository *Repository
	providers  providercontract.Resolver
}

func NewWorkflow(repository *Repository, providers providercontract.Resolver) *Workflow {
	return &Workflow{repository: repository, providers: providers}
}

func (w *Workflow) Execute(ctx context.Context, execution *operation.Execution) error {
	value, err := w.repository.context(ctx, execution.OperationID())
	if err != nil {
		return workflowError("PORT_FORWARD_CONTEXT_INVALID", false, err)
	}
	if value.operationType != "port_forward_add" && value.operationType != "port_forward_delete" {
		return workflowError(providercontract.ErrorUnsupportedOperation, false, fmt.Errorf("unknown operation %s", value.operationType))
	}
	if err := execution.Progress(ctx, "running", "validate", 10, "operation.portForward.validating"); err != nil {
		return err
	}
	if value.operationType == "port_forward_add" {
		if err := w.repository.validateAdd(ctx, value); err != nil {
			return err
		}
	}
	if err := execution.Step(ctx, "validate", "succeeded", 100, "", nil, map[string]any{"instance_id": value.instanceID}); err != nil {
		return err
	}
	adapter, err := w.providers.Resolve(ctx, value.providerID)
	if err != nil {
		return workflowError(providercontract.ErrorUnavailable, true, err)
	}
	if err := execution.Progress(ctx, "waiting_provider", "provider", 40, "operation.portForward.provider"); err != nil {
		return err
	}
	base := providercontract.InstanceActionRequest{OperationID: value.operationID.String(), IdempotencyKey: "port-forward:" + value.operationID.String(), NodeID: value.providerNodeID, ProviderInstanceID: value.providerInstanceID}
	var providerOperation *providercontract.Operation
	var mapping providercontract.PortForward
	if value.operationType == "port_forward_add" {
		providerOperation, err = adapter.AddPortForward(ctx, providercontract.AddPortForwardRequest{InstanceActionRequest: base, Protocol: value.request.Protocol, PublicIP: value.publicIP, PublicPort: value.request.PublicPort, GuestPort: value.request.GuestPort, Description: value.request.Description})
	} else {
		providerOperation, err = adapter.DeletePortForward(ctx, providercontract.DeletePortForwardRequest{InstanceActionRequest: base, ProviderMappingID: value.providerMappingID})
	}
	if err != nil {
		return providerError(err)
	}
	if err := execution.Step(ctx, "provider", "succeeded", 100, "", nil, map[string]any{"provider_operation_id": providerOperation.ProviderOperationID}); err != nil {
		return err
	}
	if err := execution.Progress(ctx, "verifying", "persist", 80, "operation.portForward.persisting"); err != nil {
		return err
	}
	if value.operationType == "port_forward_add" {
		mappings, listErr := adapter.ListPortForwards(ctx, providercontract.GetInstanceRequest{NodeID: value.providerNodeID, ProviderInstanceID: value.providerInstanceID, PlatformInstanceID: value.instanceID.String()})
		if listErr != nil {
			return providerError(listErr)
		}
		found := false
		for _, candidate := range mappings {
			if strings.EqualFold(candidate.Protocol, value.request.Protocol) && candidate.PublicPort == value.request.PublicPort && candidate.GuestPort == value.request.GuestPort {
				mapping, found = candidate, true
				break
			}
		}
		if !found {
			return workflowError("PORT_FORWARD_VERIFY_FAILED", true, errors.New("provider mapping not visible after add"))
		}
		if err := w.repository.persistAdd(ctx, value, mapping); err != nil {
			code := "PORT_FORWARD_PERSIST_FAILED"
			var persistErr *persistenceError
			if errors.As(err, &persistErr) {
				code = persistErr.code
			}
			return workflowError(code, true, err)
		}
	} else if err := w.repository.persistDelete(ctx, value); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return workflowError("PORT_FORWARD_PERSIST_FAILED", true, err)
	}
	if err := execution.Step(ctx, "persist", "succeeded", 100, "", nil, map[string]any{"instance_id": value.instanceID}); err != nil {
		return err
	}
	return execution.Progress(ctx, "running", "finish", 99, "operation.portForward.completed")
}

func providerError(err error) error {
	var target *providercontract.Error
	if errors.As(err, &target) {
		return &operation.WorkflowError{Code: target.Code, MessageKey: "operation.providerError", Retryable: target.Retryable, Cause: err}
	}
	return workflowError(providercontract.ErrorUnknown, true, err)
}

func workflowError(code string, retryable bool, err error) *operation.WorkflowError {
	return &operation.WorkflowError{Code: code, MessageKey: "operation.portForward.failed", Retryable: retryable, Cause: err}
}
