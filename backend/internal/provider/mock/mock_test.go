package mock

import (
	"context"
	"errors"
	"testing"
	"time"

	providercontract "vps-billing/backend/internal/provider"
)

var _ providercontract.Provider = (*Provider)(nil)

func TestProviderContractLifecycleAndIdempotency(t *testing.T) {
	ctx := context.Background()
	adapter := New()
	health, err := adapter.Health(ctx)
	if err != nil || health.Status != "healthy" {
		t.Fatalf("Health()=%#v error=%v", health, err)
	}
	capabilities, err := adapter.Capabilities(ctx)
	if err != nil || !capabilities.CreateInstance || !capabilities.NAT {
		t.Fatalf("Capabilities()=%#v error=%v", capabilities, err)
	}
	images, err := adapter.ListImages(ctx, "node-1")
	if err != nil || len(images) == 0 {
		t.Fatalf("ListImages()=%#v error=%v", images, err)
	}

	password := "do-not-persist"
	create := providercontract.CreateInstanceRequest{OperationID: "operation-1", IdempotencyKey: "create-key-1", NodeID: "node-1", InstanceID: "instance-1", Name: "test", CPUCores: 2, MemoryMB: 2048, DiskGB: 30, Image: images[0].ID, Virtualization: "kvm", RootPassword: &password}
	first, err := adapter.CreateInstance(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	second, err := adapter.CreateInstance(ctx, create)
	if err != nil || second.ProviderOperationID != first.ProviderOperationID {
		t.Fatalf("duplicate CreateInstance()=%#v error=%v", second, err)
	}
	changed := create
	changed.MemoryMB = 4096
	if _, err := adapter.CreateInstance(ctx, changed); providerErrorCode(err) != providercontract.ErrorInstanceAlreadyExists {
		t.Fatalf("changed idempotent create error=%v", err)
	}

	instance, err := adapter.GetInstance(ctx, providercontract.GetInstanceRequest{NodeID: "node-1", PlatformInstanceID: "instance-1"})
	if err != nil || instance.State != "running" {
		t.Fatalf("GetInstance()=%#v error=%v", instance, err)
	}
	action := providercontract.InstanceActionRequest{OperationID: "operation-2", IdempotencyKey: "stop-key", NodeID: "node-1", ProviderInstanceID: instance.ProviderInstanceID}
	if _, err := adapter.StopInstance(ctx, action); err != nil {
		t.Fatal(err)
	}
	stopped, _ := adapter.GetInstance(ctx, providercontract.GetInstanceRequest{ProviderInstanceID: instance.ProviderInstanceID})
	if stopped.State != "stopped" {
		t.Fatalf("stopped state=%s", stopped.State)
	}
	action.OperationID, action.IdempotencyKey = "operation-3", "start-key"
	if _, err := adapter.StartInstance(ctx, action); err != nil {
		t.Fatal(err)
	}
	action.OperationID, action.IdempotencyKey = "operation-4", "restart-key"
	if _, err := adapter.RestartInstance(ctx, action); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ReinstallInstance(ctx, providercontract.ReinstallInstanceRequest{InstanceActionRequest: providercontract.InstanceActionRequest{OperationID: "operation-5", IdempotencyKey: "reinstall-key", ProviderInstanceID: instance.ProviderInstanceID}, Image: "ubuntu-24.04", RootPassword: &password}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ResetPassword(ctx, providercontract.ResetPasswordRequest{InstanceActionRequest: providercontract.InstanceActionRequest{OperationID: "operation-6", IdempotencyKey: "password-key", ProviderInstanceID: instance.ProviderInstanceID}, RootPassword: password}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.GetUsage(ctx, providercontract.GetInstanceRequest{ProviderInstanceID: instance.ProviderInstanceID}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.GetTraffic(ctx, providercontract.GetTrafficRequest{GetInstanceRequest: providercontract.GetInstanceRequest{ProviderInstanceID: instance.ProviderInstanceID}, From: time.Now().Add(-time.Hour), To: time.Now()}); err != nil {
		t.Fatal(err)
	}

	add := providercontract.AddPortForwardRequest{InstanceActionRequest: providercontract.InstanceActionRequest{OperationID: "operation-7", IdempotencyKey: "nat-add-key", ProviderInstanceID: instance.ProviderInstanceID}, Protocol: "tcp", PublicIP: "192.0.2.10", PublicPort: 2201, GuestPort: 22}
	if _, err := adapter.AddPortForward(ctx, add); err != nil {
		t.Fatal(err)
	}
	forwards, err := adapter.ListPortForwards(ctx, providercontract.GetInstanceRequest{ProviderInstanceID: instance.ProviderInstanceID})
	if err != nil || len(forwards) != 1 {
		t.Fatalf("ListPortForwards()=%#v error=%v", forwards, err)
	}
	if _, err := adapter.DeletePortForward(ctx, providercontract.DeletePortForwardRequest{InstanceActionRequest: providercontract.InstanceActionRequest{OperationID: "operation-8", IdempotencyKey: "nat-delete-key", ProviderInstanceID: instance.ProviderInstanceID}, ProviderMappingID: forwards[0].ProviderMappingID}); err != nil {
		t.Fatal(err)
	}
	action.OperationID, action.IdempotencyKey = "operation-9", "delete-key"
	if _, err := adapter.DeleteInstance(ctx, action); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.GetInstance(ctx, providercontract.GetInstanceRequest{ProviderInstanceID: instance.ProviderInstanceID}); providerErrorCode(err) != providercontract.ErrorInstanceNotFound {
		t.Fatalf("deleted GetInstance() error=%v", err)
	}
}

func TestUnsupportedCapabilityIsNormalized(t *testing.T) {
	adapter := NewWithCapabilities(providercontract.Capabilities{})
	_, err := adapter.CreateInstance(context.Background(), providercontract.CreateInstanceRequest{})
	var providerErr *providercontract.Error
	if !errors.As(err, &providerErr) || providerErr.Code != providercontract.ErrorUnsupportedOperation || providerErr.Retryable {
		t.Fatalf("error=%#v", err)
	}
}

func TestCreateSuccessButResponseLostDoesNotCreateSecondInstance(t *testing.T) {
	adapter := New()
	request := providercontract.CreateInstanceRequest{OperationID: "lost-response-operation", IdempotencyKey: "lost-response-key", NodeID: "node-1", InstanceID: "instance-lost-response", CPUCores: 1, MemoryMB: 1024, DiskGB: 20, Image: "ubuntu-24.04"}
	// The first response is intentionally discarded to model a successful
	// provider create followed by a network timeout at the caller.
	if _, err := adapter.CreateInstance(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	replayed, err := adapter.CreateInstance(context.Background(), request)
	if err != nil || !replayed.Accepted {
		t.Fatalf("replayed create = %#v, %v", replayed, err)
	}
	adapter.mu.RLock()
	instanceCount := len(adapter.instances)
	adapter.mu.RUnlock()
	if instanceCount != 1 {
		t.Fatalf("provider created %d instances after response loss", instanceCount)
	}
}

func providerErrorCode(err error) string {
	var providerErr *providercontract.Error
	if errors.As(err, &providerErr) {
		return providerErr.Code
	}
	return ""
}
