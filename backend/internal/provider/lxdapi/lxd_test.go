package lxdapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	providercontract "vps-billing/backend/internal/provider"
)

func TestAdapterContractAndIdempotency(t *testing.T) {
	var mu sync.Mutex
	created := false
	createCount, stateActionCount := 0, 0
	instanceID := "01990000-0000-7000-8000-000000000001"
	instanceName := instanceName(instanceID)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1.0" && r.URL.Query().Get("project") != "billing" {
			t.Errorf("project query = %q", r.URL.Query().Get("project"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0":
			writeEnvelope(t, w, http.StatusOK, map[string]any{"environment": map[string]any{"server_version": "6.8"}}, "")
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/images":
			writeEnvelope(t, w, http.StatusOK, []any{map[string]any{"fingerprint": "fingerprint", "aliases": []any{map[string]any{"name": "ubuntu-24.04"}}, "properties": map[string]string{"description": "Ubuntu", "os": "Ubuntu", "release": "24.04", "architecture": "amd64"}}}, "")
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/"+instanceName:
			mu.Lock()
			exists := created
			mu.Unlock()
			if !exists {
				writeError(t, w, http.StatusNotFound, "Instance not found")
				return
			}
			writeEnvelope(t, w, http.StatusOK, map[string]any{"name": instanceName, "status": "Running", "created_at": "2026-09-23T00:00:00Z", "config": map[string]string{"limits.cpu": "2", "limits.memory": "2048MiB", "user.vps_billing.instance_id": instanceID, "user.vps_billing.idempotency_key": "create-key"}}, "")
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/"+instanceName+"/state":
			writeEnvelope(t, w, http.StatusOK, map[string]any{"status": "Running", "memory": map[string]any{"usage": 104857600}, "network": map[string]any{"eth0": map[string]any{"addresses": []any{map[string]any{"family": "inet", "address": "192.0.2.42", "scope": "global"}}, "counters": map[string]any{"bytes_received": 1000, "bytes_sent": 2000}}}}, "")
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/instances":
			if r.URL.Query().Get("target") != "member-1" {
				t.Errorf("target query = %q", r.URL.Query().Get("target"))
			}
			mu.Lock()
			created = true
			createCount++
			mu.Unlock()
			writeEnvelope(t, w, http.StatusAccepted, nil, "/1.0/operations/create-op")
		case r.Method == http.MethodPut && r.URL.Path == "/1.0/instances/"+instanceName+"/state":
			mu.Lock()
			stateActionCount++
			mu.Unlock()
			writeEnvelope(t, w, http.StatusAccepted, nil, "/1.0/operations/state-op")
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/instances/"+instanceName+"/rebuild":
			writeEnvelope(t, w, http.StatusAccepted, nil, "/1.0/operations/rebuild-op")
		case r.Method == http.MethodDelete && r.URL.Path == "/1.0/instances/"+instanceName:
			writeEnvelope(t, w, http.StatusAccepted, nil, "/1.0/operations/delete-op")
		case r.Method == http.MethodGet && (r.URL.Path == "/1.0/operations/create-op/wait" || r.URL.Path == "/1.0/operations/state-op/wait" || r.URL.Path == "/1.0/operations/rebuild-op/wait" || r.URL.Path == "/1.0/operations/delete-op/wait"):
			writeEnvelope(t, w, http.StatusOK, map[string]any{"status": "Success", "status_code": 200, "err": ""}, "")
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			writeError(t, w, http.StatusNotFound, "not found")
		}
	}))
	defer server.Close()
	adapter, err := New(Config{Endpoint: server.URL, Project: "billing", AllowInsecureHTTP: true, OperationTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if health, err := adapter.Health(ctx); err != nil || health.Version != "6.8" {
		t.Fatalf("Health()=%+v error=%v", health, err)
	}
	if capabilities, err := adapter.Capabilities(ctx); err != nil || !capabilities.CreateInstance || capabilities.NAT || capabilities.ResetPassword {
		t.Fatalf("Capabilities()=%+v error=%v", capabilities, err)
	}
	if images, err := adapter.ListImages(ctx, "member-1"); err != nil || len(images) != 1 || images[0].ID != "ubuntu-24.04" {
		t.Fatalf("ListImages()=%+v error=%v", images, err)
	}
	createRequest := providercontract.CreateInstanceRequest{OperationID: "operation-1", IdempotencyKey: "create-key", NodeID: "member-1", InstanceID: instanceID, Name: "VPS", CPUCores: 2, MemoryMB: 2048, DiskGB: 30, IPv4Count: 1, Image: "ubuntu-24.04", Virtualization: "kvm"}
	if _, err := adapter.CreateInstance(ctx, createRequest); err != nil {
		t.Fatal(err)
	}
	if replay, err := adapter.CreateInstance(ctx, createRequest); err != nil || replay.Metadata["idempotent_replay"] != true {
		t.Fatalf("CreateInstance replay=%+v error=%v", replay, err)
	}
	conflictingCreate := createRequest
	conflictingCreate.InstanceID = "01990000-0000-7000-8000-000000000002"
	if _, err := adapter.CreateInstance(ctx, conflictingCreate); !isCode(err, providercontract.ErrorInstanceAlreadyExists) {
		t.Fatalf("CreateInstance conflicting idempotency key error=%v", err)
	}
	instance, err := adapter.GetInstance(ctx, providercontract.GetInstanceRequest{PlatformInstanceID: instanceID})
	if err != nil || instance.State != "running" || len(instance.IPv4) != 1 {
		t.Fatalf("GetInstance()=%+v error=%v", instance, err)
	}
	action := providercontract.InstanceActionRequest{OperationID: "operation-2", IdempotencyKey: "start-key", NodeID: "member-1", ProviderInstanceID: instanceName}
	if _, err := adapter.StartInstance(ctx, action); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.StartInstance(ctx, action); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.StopInstance(ctx, providercontract.InstanceActionRequest{OperationID: "operation-3", IdempotencyKey: "stop-key", ProviderInstanceID: instanceName}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.RestartInstance(ctx, providercontract.InstanceActionRequest{OperationID: "operation-4", IdempotencyKey: "restart-key", ProviderInstanceID: instanceName}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ReinstallInstance(ctx, providercontract.ReinstallInstanceRequest{InstanceActionRequest: providercontract.InstanceActionRequest{OperationID: "operation-5", IdempotencyKey: "reinstall-key", ProviderInstanceID: instanceName}, Image: "ubuntu-24.04"}); err != nil {
		t.Fatal(err)
	}
	if replay, err := adapter.ReinstallInstance(ctx, providercontract.ReinstallInstanceRequest{InstanceActionRequest: providercontract.InstanceActionRequest{OperationID: "operation-5", IdempotencyKey: "reinstall-key", ProviderInstanceID: instanceName}, Image: "ubuntu-24.04"}); err != nil || replay.Metadata["idempotent_replay"] != true {
		t.Fatalf("ReinstallInstance replay=%+v error=%v", replay, err)
	}
	if _, err := adapter.StartInstance(ctx, providercontract.InstanceActionRequest{OperationID: "operation-7", IdempotencyKey: "start-key", ProviderInstanceID: "another-instance"}); !isCode(err, providercontract.ErrorInstanceAlreadyExists) {
		t.Fatalf("StartInstance conflicting idempotency key error=%v", err)
	}
	if usage, err := adapter.GetUsage(ctx, providercontract.GetInstanceRequest{ProviderInstanceID: instanceName}); err != nil || usage.MemoryUsedMB != 100 {
		t.Fatalf("GetUsage()=%+v error=%v", usage, err)
	}
	if traffic, err := adapter.GetTraffic(ctx, providercontract.GetTrafficRequest{GetInstanceRequest: providercontract.GetInstanceRequest{ProviderInstanceID: instanceName}}); err != nil || traffic.RXBytes != 1000 || traffic.TXBytes != 2000 {
		t.Fatalf("GetTraffic()=%+v error=%v", traffic, err)
	}
	if _, err := adapter.ResetPassword(ctx, providercontract.ResetPasswordRequest{}); !isCode(err, providercontract.ErrorUnsupportedOperation) {
		t.Fatalf("ResetPassword() error=%v", err)
	}
	if _, err := adapter.AddPortForward(ctx, providercontract.AddPortForwardRequest{}); !isCode(err, providercontract.ErrorUnsupportedOperation) {
		t.Fatalf("AddPortForward() error=%v", err)
	}
	if _, err := adapter.DeleteInstance(ctx, providercontract.InstanceActionRequest{OperationID: "operation-6", IdempotencyKey: "delete-key", ProviderInstanceID: instanceName}); err != nil {
		t.Fatal(err)
	}
	if replay, err := adapter.DeleteInstance(ctx, providercontract.InstanceActionRequest{OperationID: "operation-6", IdempotencyKey: "delete-key", ProviderInstanceID: instanceName}); err != nil || replay.Metadata["idempotent_replay"] != true {
		t.Fatalf("DeleteInstance replay=%+v error=%v", replay, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if createCount != 1 || stateActionCount != 5 {
		t.Fatalf("create requests=%d state actions=%d", createCount, stateActionCount)
	}
}

func TestAdapterSecurityAndErrorNormalization(t *testing.T) {
	if _, err := New(Config{Endpoint: "http://lxd.example"}); err == nil {
		t.Fatal("New() accepted insecure HTTP")
	}
	if _, err := NewFromFactory(providercontract.FactoryConfig{Endpoint: "http://lxd.example", Config: json.RawMessage(`{"allow_insecure_http_for_test":true}`)}); err == nil {
		t.Fatal("NewFromFactory() accepted persisted insecure HTTP override")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(t, w, http.StatusUnauthorized, "not authorized")
	}))
	defer server.Close()
	adapter, err := New(Config{Endpoint: server.URL, AllowInsecureHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Health(context.Background()); !isCode(err, providercontract.ErrorAuthFailed) {
		t.Fatalf("Health() error=%v", err)
	}
	timeoutAdapter, err := New(Config{Endpoint: server.URL, AllowInsecureHTTP: true, Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := timeoutAdapter.Health(context.Background()); !isCode(err, providercontract.ErrorTimeout) {
		t.Fatalf("timeout Health() error=%v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func writeEnvelope(t *testing.T, w http.ResponseWriter, status int, metadata any, operation string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{"type": "sync", "status": "Success", "status_code": status, "operation": operation, "error_code": 0, "error": "", "metadata": metadata}); err != nil {
		t.Error(err)
	}
}

func writeError(t *testing.T, w http.ResponseWriter, status int, message string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{"type": "error", "status": "Failure", "status_code": status, "error_code": status, "error": message, "metadata": nil}); err != nil {
		t.Error(err)
	}
}

func TestProviderErrorsAreTyped(t *testing.T) {
	err := normalizeHTTP(http.StatusInsufficientStorage, "no space left", errors.New("raw"))
	var providerErr *providercontract.Error
	if !errors.As(err, &providerErr) || providerErr.Code != providercontract.ErrorResourceExhausted || !providerErr.Retryable {
		t.Fatalf("normalized error=%+v", err)
	}
}
