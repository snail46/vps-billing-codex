package runmanprovider

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	providercontract "vps-billing/backend/internal/provider"
	"vps-billing/backend/internal/runman"
)

func TestAdapterContractLifecycleIdempotencyAndState(t *testing.T) {
	nodeID, instanceID := uuid.New(), uuid.New()
	store := &contractStore{
		commands: map[string]runman.Command{},
		vm:       runman.VMState{InstanceID: instanceID, Status: "running", CPUPercent: 12.5, RAMUsedMB: 256, MonthlyIn: 100, MonthlyOut: 200, IPs: []string{"192.0.2.8", "2001:db8::8"}, ObservedAt: time.Now().UTC()},
		spec:     runman.InstanceSpec{CPU: 2, MemoryMB: 2048, DiskGB: 40, BandwidthMbps: 100},
	}
	adapter := New(store, runman.NewBroker(store, onlineSender{}, time.Second), nil)

	capabilities, err := adapter.Capabilities(context.Background())
	if err != nil || !capabilities.CreateInstance || !capabilities.NAT || !capabilities.Traffic {
		t.Fatalf("Capabilities() = %#v, %v", capabilities, err)
	}
	request := providercontract.CreateInstanceRequest{OperationID: uuid.NewString(), IdempotencyKey: "create-once", NodeID: nodeID.String(), InstanceID: instanceID.String(), CPUCores: 2, MemoryMB: 2048, DiskGB: 40, Image: "ubuntu-24.04"}
	first, err := adapter.CreateInstance(context.Background(), request)
	if err != nil || !first.Accepted {
		t.Fatalf("CreateInstance() = %#v, %v", first, err)
	}
	second, err := adapter.CreateInstance(context.Background(), request)
	if err != nil || second.ProviderOperationID != first.ProviderOperationID {
		t.Fatalf("duplicate CreateInstance() = %#v, %v", second, err)
	}
	if store.createCalls != 2 || len(store.commands) != 1 {
		t.Fatalf("idempotent commands = %d calls, %d records", store.createCalls, len(store.commands))
	}

	instance, err := adapter.GetInstance(context.Background(), providercontract.GetInstanceRequest{NodeID: nodeID.String(), PlatformInstanceID: instanceID.String()})
	if err != nil || instance.State != "running" || len(instance.IPv4) != 1 || len(instance.IPv6) != 1 || instance.MemoryMB != 2048 {
		t.Fatalf("GetInstance() = %#v, %v", instance, err)
	}
	usage, err := adapter.GetUsage(context.Background(), providercontract.GetInstanceRequest{NodeID: nodeID.String(), PlatformInstanceID: instanceID.String()})
	if err != nil || usage.CPUPercent != 12.5 || usage.MemoryTotalMB != 2048 {
		t.Fatalf("GetUsage() = %#v, %v", usage, err)
	}
	traffic, err := adapter.GetTraffic(context.Background(), providercontract.GetTrafficRequest{GetInstanceRequest: providercontract.GetInstanceRequest{NodeID: nodeID.String(), PlatformInstanceID: instanceID.String()}})
	if err != nil || traffic.RXBytes != 100 || traffic.TXBytes != 200 {
		t.Fatalf("GetTraffic() = %#v, %v", traffic, err)
	}

	action := providercontract.InstanceActionRequest{OperationID: uuid.NewString(), IdempotencyKey: "restart-once", NodeID: nodeID.String(), ProviderInstanceID: instanceID.String()}
	if result, actionErr := adapter.RestartInstance(context.Background(), action); actionErr != nil || !result.Accepted || store.lastKind != "restart" {
		t.Fatalf("RestartInstance() = %#v, %v", result, actionErr)
	}
	if result, actionErr := adapter.ReinstallInstance(context.Background(), providercontract.ReinstallInstanceRequest{InstanceActionRequest: providercontract.InstanceActionRequest{OperationID: uuid.NewString(), IdempotencyKey: "reinstall-once", NodeID: nodeID.String(), ProviderInstanceID: instanceID.String()}, Image: "debian-13"}); actionErr != nil || !result.Accepted || store.lastKind != "reinstall" {
		t.Fatalf("ReinstallInstance() = %#v, %v", result, actionErr)
	}
}

func TestAdapterNormalizesOfflineNode(t *testing.T) {
	store := &contractStore{commands: map[string]runman.Command{}, commandStatus: "queued"}
	adapter := New(store, runman.NewBroker(store, offlineSender{}, time.Second), nil)
	_, err := adapter.StartInstance(context.Background(), providercontract.InstanceActionRequest{OperationID: uuid.NewString(), IdempotencyKey: "offline", NodeID: uuid.NewString(), ProviderInstanceID: uuid.NewString()})
	providerErr, ok := err.(*providercontract.Error)
	if !ok || providerErr.Code != providercontract.ErrorNodeOffline || !providerErr.Retryable {
		t.Fatalf("StartInstance() error = %#v", err)
	}
}

type contractStore struct {
	runman.Store
	commands      map[string]runman.Command
	createCalls   int
	lastKind      string
	commandStatus string
	vm            runman.VMState
	spec          runman.InstanceSpec
}

func (s *contractStore) CreateCommand(_ context.Context, node uuid.UUID, _ *uuid.UUID, key, kind string, payload any) (runman.Command, error) {
	s.createCalls++
	if command, exists := s.commands[key]; exists {
		return command, nil
	}
	raw, _ := json.Marshal(payload)
	status := s.commandStatus
	if status == "" {
		status = "succeeded"
	}
	command := runman.Command{ID: uuid.New(), NodeID: node, Type: kind, Payload: raw, Status: status}
	s.commands[key], s.lastKind = command, kind
	return command, nil
}
func (s *contractStore) VM(context.Context, uuid.UUID, uuid.UUID) (runman.VMState, error) {
	return s.vm, nil
}
func (s *contractStore) InstanceSpec(context.Context, uuid.UUID) (runman.InstanceSpec, error) {
	return s.spec, nil
}

type onlineSender struct{}

func (onlineSender) Send(context.Context, runman.Command) error { return nil }
func (onlineSender) Online(uuid.UUID) bool                      { return true }

type offlineSender struct{}

func (offlineSender) Send(context.Context, runman.Command) error { return nil }
func (offlineSender) Online(uuid.UUID) bool                      { return false }
