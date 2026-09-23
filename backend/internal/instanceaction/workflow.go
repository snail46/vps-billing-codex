package instanceaction

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"vps-billing/backend/internal/operation"
	providercontract "vps-billing/backend/internal/provider"
	db "vps-billing/backend/internal/store/sqlc"
)

type Repository struct{ queries *db.Queries }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{queries: db.New(pool)} }

type actionContext struct {
	instanceID, providerID                                     uuid.UUID
	providerInstanceID, providerNodeID, imageID, operationType string
}

func (r *Repository) context(ctx context.Context, operationID uuid.UUID) (actionContext, error) {
	row, err := r.queries.GetInstanceActionContextByOperation(ctx, operationID)
	if err != nil {
		return actionContext{}, err
	}
	if row.ProviderID == nil || !row.ProviderInstanceID.Valid {
		return actionContext{}, errors.New("instance provider placement is incomplete")
	}
	return actionContext{instanceID: row.ID, providerID: *row.ProviderID, providerInstanceID: row.ProviderInstanceID.String, providerNodeID: row.ProviderNodeID.String, imageID: row.ImageID.String, operationType: row.OperationType}, nil
}

func (r *Repository) setActionState(ctx context.Context, id uuid.UUID, desired, observed string) error {
	return r.queries.SetInstanceActionState(ctx, db.SetInstanceActionStateParams{ID: id, DesiredState: desired, ObservedState: observed})
}
func (r *Repository) setObserved(ctx context.Context, id uuid.UUID, observed string) error {
	return r.queries.SetInstanceObservedState(ctx, db.SetInstanceObservedStateParams{ID: id, ObservedState: observed})
}

type Workflow struct {
	repository *Repository
	providers  providercontract.Resolver
}

func NewWorkflow(repository *Repository, providers providercontract.Resolver) *Workflow {
	return &Workflow{repository: repository, providers: providers}
}

func (w *Workflow) Execute(ctx context.Context, execution *operation.Execution) error {
	current, err := w.repository.context(ctx, execution.OperationID())
	if err != nil {
		return workflowError("INSTANCE_CONTEXT_INVALID", false, err)
	}
	action := current.operationType
	desired, transitional, expected := "running", "restarting", "running"
	if action == "stop" {
		desired, transitional, expected = "stopped", "stopping", "stopped"
	}
	if action == "reinstall" {
		transitional = "reinstalling"
	}
	if action != "start" && action != "stop" && action != "restart" && action != "reinstall" {
		return workflowError("UNSUPPORTED_OPERATION", false, fmt.Errorf("unknown action %s", action))
	}
	if err := execution.Progress(ctx, "running", "validate", 5, "operation.instance.validating"); err != nil {
		return err
	}
	if err := execution.Step(ctx, "validate", "succeeded", 100, "", nil, map[string]any{"instance_id": current.instanceID}); err != nil {
		return err
	}
	if err := w.repository.setActionState(ctx, current.instanceID, desired, transitional); err != nil {
		return workflowError("INSTANCE_STATE_UPDATE_FAILED", true, err)
	}
	adapter, err := w.providers.Resolve(ctx, current.providerID)
	if err != nil {
		return workflowError(providercontract.ErrorUnavailable, true, err)
	}
	if err := execution.Progress(ctx, "waiting_provider", "provider", 30, "operation.instance."+action); err != nil {
		return err
	}
	if err := execution.Step(ctx, "provider", "running", 20, "", nil, nil); err != nil {
		return err
	}
	request := providercontract.InstanceActionRequest{OperationID: execution.OperationID().String(), IdempotencyKey: "instance-action:" + execution.OperationID().String(), NodeID: current.providerNodeID, ProviderInstanceID: current.providerInstanceID}
	var providerOperation *providercontract.Operation
	switch action {
	case "start":
		providerOperation, err = adapter.StartInstance(ctx, request)
	case "stop":
		providerOperation, err = adapter.StopInstance(ctx, request)
	case "restart":
		providerOperation, err = adapter.RestartInstance(ctx, request)
	case "reinstall":
		providerOperation, err = adapter.ReinstallInstance(ctx, providercontract.ReinstallInstanceRequest{InstanceActionRequest: request, Image: current.imageID})
	}
	if err != nil {
		return providerError(err)
	}
	if err := execution.Step(ctx, "provider", "succeeded", 100, "", nil, map[string]any{"provider_operation_id": providerOperation.ProviderOperationID}); err != nil {
		return err
	}
	if err := execution.Progress(ctx, "verifying", "verify", 80, "operation.instance.verifying"); err != nil {
		return err
	}
	instance, err := adapter.GetInstance(ctx, providercontract.GetInstanceRequest{NodeID: current.providerNodeID, ProviderInstanceID: current.providerInstanceID, PlatformInstanceID: current.instanceID.String()})
	if err != nil {
		return providerError(err)
	}
	if instance.State != expected {
		return workflowError("INSTANCE_STATE_MISMATCH", true, fmt.Errorf("expected %s, observed %s", expected, instance.State))
	}
	if err := w.repository.setObserved(ctx, current.instanceID, instance.State); err != nil {
		return workflowError("INSTANCE_STATE_UPDATE_FAILED", true, err)
	}
	if err := execution.Step(ctx, "verify", "succeeded", 100, "", nil, map[string]any{"state": instance.State}); err != nil {
		return err
	}
	return execution.Progress(ctx, "running", "finish", 99, "operation.instance.completed")
}

func providerError(err error) error {
	var target *providercontract.Error
	if errors.As(err, &target) {
		return &operation.WorkflowError{Code: target.Code, MessageKey: "operation.providerError", Retryable: target.Retryable, Cause: err}
	}
	return workflowError(providercontract.ErrorUnknown, true, err)
}
func workflowError(code string, retryable bool, err error) *operation.WorkflowError {
	return &operation.WorkflowError{Code: code, MessageKey: "operation.instance.failed", Retryable: retryable, Cause: err}
}
