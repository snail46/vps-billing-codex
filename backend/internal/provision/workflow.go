package provision

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"vps-billing/backend/internal/infrastructure"
	"vps-billing/backend/internal/operation"
	providercontract "vps-billing/backend/internal/provider"
)

type Workflow struct {
	repository *Repository
	scheduler  *infrastructure.Scheduler
	providers  providercontract.Resolver
}

func NewWorkflow(repository *Repository, scheduler *infrastructure.Scheduler, providers providercontract.Resolver) *Workflow {
	return &Workflow{repository: repository, scheduler: scheduler, providers: providers}
}

func (w *Workflow) Execute(ctx context.Context, execution *operation.Execution) error {
	current, err := w.repository.Context(ctx, execution.OperationID())
	if err != nil {
		return workflowError("PROVISION_CONTEXT_INVALID", false, err)
	}
	if err := execution.Progress(ctx, "running", "validate_subscription", 2, "operation.provision.validating"); err != nil {
		return err
	}
	if err := execution.Step(ctx, "validate_subscription", "succeeded", 100, "", nil, map[string]any{"subscription_id": current.SubscriptionID}); err != nil {
		return err
	}
	if err := execution.Step(ctx, "select_node", "running", 20, "", nil, nil); err != nil {
		return err
	}
	reservation, err := w.scheduler.Reserve(ctx, execution.OperationID(), infrastructure.ScheduleRequest{
		NodeGroupID:          current.NodeGroupID,
		Resources:            infrastructure.Capacity{CPUCores: current.CPUCores, MemoryMB: current.MemoryMB, DiskGB: current.DiskGB, IPv4Count: current.IPv4Count, IPv6Count: current.IPv6Count, NATPortCount: current.NATPortCount},
		RequiredCapabilities: map[string]any{"virtualization": current.Virtualization}, TTL: 30 * time.Minute,
	})
	if errors.Is(err, infrastructure.ErrResourceExhausted) {
		_ = execution.Progress(ctx, "waiting_resource", "select_node", 15, "operation.provision.waitingResource")
		return workflowError(providercontract.ErrorResourceExhausted, true, err)
	}
	if err != nil {
		return workflowError("SCHEDULER_FAILED", true, err)
	}
	if err := execution.Step(ctx, "select_node", "succeeded", 100, "", nil, map[string]any{"node_id": reservation.NodeID}); err != nil {
		return err
	}
	if err := execution.Step(ctx, "reserve_resources", "succeeded", 100, "", nil, map[string]any{"reservation_id": reservation.ID}); err != nil {
		return err
	}
	placement, err := w.repository.Placement(ctx, execution.OperationID())
	if err != nil {
		return workflowError("PLACEMENT_MISSING", true, err)
	}
	if err := w.repository.SetPlacement(ctx, current.InstanceID, placement); err != nil {
		return workflowError("INSTANCE_PLACEMENT_FAILED", true, err)
	}
	adapter, err := w.providers.Resolve(ctx, placement.ProviderID)
	if err != nil {
		return workflowError(providercontract.ErrorUnavailable, true, err)
	}
	if err := execution.Progress(ctx, "waiting_provider", "create_instance", 35, "operation.provision.creating"); err != nil {
		return err
	}
	if err := execution.Step(ctx, "create_instance", "running", 20, "", nil, nil); err != nil {
		return err
	}
	providerOperation, err := adapter.CreateInstance(ctx, providercontract.CreateInstanceRequest{
		OperationID: execution.OperationID().String(), IdempotencyKey: "provision:" + execution.OperationID().String(),
		NodeID: placement.ProviderNodeID, InstanceID: current.InstanceID.String(), Name: current.InstanceName,
		CPUCores: current.CPUCores, MemoryMB: current.MemoryMB, DiskGB: current.DiskGB,
		TrafficGB: current.TrafficGB, BandwidthMbps: current.BandwidthMbps,
		IPv4Count: int(current.IPv4Count), IPv6Count: int(current.IPv6Count), NATPortCount: int(current.NATPortCount),
		Image: current.ImageID, Virtualization: current.Virtualization,
	})
	if err != nil {
		return w.handleProviderError(ctx, execution, current, reservation.ID, err)
	}
	if err := execution.Step(ctx, "create_instance", "succeeded", 100, "", nil, map[string]any{"provider_operation_id": providerOperation.ProviderOperationID}); err != nil {
		return err
	}
	if err := execution.Step(ctx, "wait_provider", "running", 50, "", nil, nil); err != nil {
		return err
	}
	providerInstance, err := adapter.GetInstance(ctx, providercontract.GetInstanceRequest{NodeID: placement.ProviderNodeID, ProviderInstanceID: current.ProviderInstanceID, PlatformInstanceID: current.InstanceID.String()})
	if err != nil {
		return w.handleProviderError(ctx, execution, current, reservation.ID, err)
	}
	if err := execution.Step(ctx, "wait_provider", "succeeded", 100, "", nil, map[string]any{"provider_instance_id": providerInstance.ProviderInstanceID}); err != nil {
		return err
	}
	if err := execution.Step(ctx, "configure_network", "succeeded", 100, "", nil, map[string]any{"ipv4_count": len(providerInstance.IPv4), "ipv6_count": len(providerInstance.IPv6)}); err != nil {
		return err
	}
	if providerInstance.State != "running" {
		_ = execution.Progress(ctx, "verifying", "verify_running", 75, "operation.provision.verifying")
		return workflowError("INSTANCE_NOT_RUNNING", true, fmt.Errorf("provider state is %s", providerInstance.State))
	}
	if err := execution.Step(ctx, "verify_running", "succeeded", 100, "", nil, map[string]any{"state": providerInstance.State}); err != nil {
		return err
	}
	if err := w.repository.SetProviderResult(ctx, current.InstanceID, providerInstance.ProviderInstanceID, "running", providerInstance.IPv4, providerInstance.IPv6); err != nil {
		return workflowError("INSTANCE_PERSIST_FAILED", true, err)
	}
	if err := execution.Step(ctx, "persist_network", "succeeded", 100, "", nil, map[string]any{}); err != nil {
		return err
	}
	if err := execution.Step(ctx, "commit_reservation", "running", 80, "", nil, nil); err != nil {
		return err
	}
	if err := w.repository.Finalize(ctx, current, placement); err != nil {
		return workflowError("PROVISION_FINALIZE_FAILED", true, err)
	}
	if err := execution.Step(ctx, "commit_reservation", "succeeded", 100, "", nil, nil); err != nil {
		return err
	}
	if err := execution.Step(ctx, "activate_subscription", "succeeded", 100, "", nil, map[string]any{"subscription_id": current.SubscriptionID}); err != nil {
		return err
	}
	if err := execution.Step(ctx, "notify", "succeeded", 100, "", nil, nil); err != nil {
		return err
	}
	return execution.Progress(ctx, "running", "finish", 99, "operation.provision.finishing")
}

func (w *Workflow) handleProviderError(ctx context.Context, execution *operation.Execution, current Context, reservationID uuid.UUID, err error) error {
	var providerErr *providercontract.Error
	if errors.As(err, &providerErr) {
		if !providerErr.Retryable {
			_ = w.repository.SetError(ctx, current.InstanceID)
			_, _ = w.scheduler.Release(ctx, reservationID)
		}
		return &operation.WorkflowError{Code: providerErr.Code, MessageKey: "operation.providerError", Retryable: providerErr.Retryable, Cause: err}
	}
	return workflowError(providercontract.ErrorUnknown, true, err)
}

func workflowError(code string, retryable bool, err error) *operation.WorkflowError {
	return &operation.WorkflowError{Code: code, MessageKey: "operation.provision.failed", Retryable: retryable, Cause: err}
}
