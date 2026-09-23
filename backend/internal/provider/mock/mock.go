package mock

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	providercontract "vps-billing/backend/internal/provider"
)

type Provider struct {
	mu             sync.RWMutex
	capabilities   providercontract.Capabilities
	instances      map[string]storedInstance
	byPlatformID   map[string]string
	createRequests map[string]providercontract.CreateInstanceRequest
	operations     map[string]providercontract.Operation
	actionTargets  map[string]string
	portForwards   map[string]map[string]providercontract.PortForward
	images         []providercontract.Image
	now            func() time.Time
}

type storedInstance struct {
	instance providercontract.Instance
	image    string
}

func New() *Provider {
	return &Provider{
		capabilities: providercontract.Capabilities{CreateInstance: true, DeleteInstance: true, Start: true, Stop: true, Restart: true, Reinstall: true, ResetPassword: true, Traffic: true, Metrics: true, NAT: true, IPv4: true, IPv6: true, SupportedRuntimes: []string{"kvm", "lxc"}},
		instances:    make(map[string]storedInstance), byPlatformID: make(map[string]string), createRequests: make(map[string]providercontract.CreateInstanceRequest), operations: make(map[string]providercontract.Operation), actionTargets: make(map[string]string), portForwards: make(map[string]map[string]providercontract.PortForward),
		images: []providercontract.Image{{ID: "ubuntu-24.04", Name: "Ubuntu 24.04", OS: "linux", Version: "24.04", Arch: "amd64", Description: "Mock Ubuntu image"}}, now: time.Now,
	}
}

func NewWithCapabilities(capabilities providercontract.Capabilities) *Provider {
	result := New()
	result.capabilities = capabilities
	return result
}

func (p *Provider) Name() string { return "mock" }

func (p *Provider) Health(context.Context) (*providercontract.Health, error) {
	return &providercontract.Health{Status: "healthy", Version: "v1", CheckedAt: p.now().UTC(), Details: map[string]any{"kind": "in_memory"}}, nil
}

func (p *Provider) Capabilities(context.Context) (*providercontract.Capabilities, error) {
	copy := p.capabilities
	copy.SupportedRuntimes = append([]string(nil), p.capabilities.SupportedRuntimes...)
	return &copy, nil
}

func (p *Provider) ListImages(context.Context, string) ([]providercontract.Image, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]providercontract.Image(nil), p.images...), nil
}

func (p *Provider) CreateInstance(_ context.Context, request providercontract.CreateInstanceRequest) (*providercontract.Operation, error) {
	if !p.capabilities.CreateInstance {
		return nil, unsupported("create_instance")
	}
	if request.OperationID == "" || request.IdempotencyKey == "" || request.InstanceID == "" || request.NodeID == "" {
		return nil, unknown("invalid create request")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if operation, exists := p.operations["create:"+request.IdempotencyKey]; exists {
		original := p.createRequests[request.IdempotencyKey]
		original.RootPassword, request.RootPassword = nil, nil
		if !reflect.DeepEqual(original, request) {
			return nil, providerError(providercontract.ErrorInstanceAlreadyExists, false, "idempotency key reused with different create request")
		}
		copy := operation
		return &copy, nil
	}
	if _, exists := p.byPlatformID[request.InstanceID]; exists {
		return nil, providerError(providercontract.ErrorInstanceAlreadyExists, false, "platform instance already exists")
	}
	providerID := "mock-" + request.InstanceID
	instance := providercontract.Instance{ProviderInstanceID: providerID, State: "running", CPUCores: request.CPUCores, MemoryMB: request.MemoryMB, DiskGB: request.DiskGB, CreatedAt: p.now().UTC(), Metadata: map[string]any{"platform_instance_id": request.InstanceID, "node_id": request.NodeID, "image": request.Image}}
	operation := successfulOperation(request.OperationID, "create")
	storedRequest := request
	storedRequest.RootPassword = nil
	p.instances[providerID] = storedInstance{instance: instance, image: request.Image}
	p.byPlatformID[request.InstanceID] = providerID
	p.createRequests[request.IdempotencyKey] = storedRequest
	p.operations["create:"+request.IdempotencyKey] = operation
	copy := operation
	return &copy, nil
}

func (p *Provider) GetInstance(_ context.Context, request providercontract.GetInstanceRequest) (*providercontract.Instance, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	providerID := request.ProviderInstanceID
	if providerID == "" {
		providerID = p.byPlatformID[request.PlatformInstanceID]
	}
	stored, ok := p.instances[providerID]
	if !ok {
		return nil, providerError(providercontract.ErrorInstanceNotFound, false, "mock instance not found")
	}
	copy := stored.instance
	copy.IPv4 = append([]string(nil), stored.instance.IPv4...)
	copy.IPv6 = append([]string(nil), stored.instance.IPv6...)
	return &copy, nil
}

func (p *Provider) StartInstance(_ context.Context, request providercontract.InstanceActionRequest) (*providercontract.Operation, error) {
	return p.action(request, "start", p.capabilities.Start, "running", false)
}
func (p *Provider) StopInstance(_ context.Context, request providercontract.InstanceActionRequest) (*providercontract.Operation, error) {
	return p.action(request, "stop", p.capabilities.Stop, "stopped", false)
}
func (p *Provider) RestartInstance(_ context.Context, request providercontract.InstanceActionRequest) (*providercontract.Operation, error) {
	return p.action(request, "restart", p.capabilities.Restart, "running", false)
}
func (p *Provider) DeleteInstance(_ context.Context, request providercontract.InstanceActionRequest) (*providercontract.Operation, error) {
	return p.action(request, "delete", p.capabilities.DeleteInstance, "deleted", true)
}

func (p *Provider) ReinstallInstance(_ context.Context, request providercontract.ReinstallInstanceRequest) (*providercontract.Operation, error) {
	if !p.capabilities.Reinstall {
		return nil, unsupported("reinstall")
	}
	if request.Image == "" {
		return nil, providerError(providercontract.ErrorImageNotFound, false, "mock image is required")
	}
	operation, err := p.action(request.InstanceActionRequest, "reinstall", true, "running", false)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	stored := p.instances[request.ProviderInstanceID]
	stored.image = request.Image
	stored.instance.Metadata["image"] = request.Image
	p.instances[request.ProviderInstanceID] = stored
	p.mu.Unlock()
	return operation, nil
}

func (p *Provider) ResetPassword(_ context.Context, request providercontract.ResetPasswordRequest) (*providercontract.Operation, error) {
	if !p.capabilities.ResetPassword {
		return nil, unsupported("reset_password")
	}
	return p.action(request.InstanceActionRequest, "reset_password", true, "", false)
}

func (p *Provider) GetUsage(_ context.Context, request providercontract.GetInstanceRequest) (*providercontract.Usage, error) {
	if !p.capabilities.Metrics {
		return nil, unsupported("metrics")
	}
	instance, err := p.GetInstance(context.Background(), request)
	if err != nil {
		return nil, err
	}
	return &providercontract.Usage{MemoryTotalMB: instance.MemoryMB, DiskTotalGB: float64(instance.DiskGB), ObservedAt: p.now().UTC()}, nil
}

func (p *Provider) GetTraffic(_ context.Context, request providercontract.GetTrafficRequest) (*providercontract.Traffic, error) {
	if !p.capabilities.Traffic {
		return nil, unsupported("traffic")
	}
	if _, err := p.GetInstance(context.Background(), request.GetInstanceRequest); err != nil {
		return nil, err
	}
	return &providercontract.Traffic{From: request.From, To: request.To}, nil
}

func (p *Provider) ListPortForwards(_ context.Context, request providercontract.GetInstanceRequest) ([]providercontract.PortForward, error) {
	if !p.capabilities.NAT {
		return nil, unsupported("nat")
	}
	instance, err := p.GetInstance(context.Background(), request)
	if err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	entries := p.portForwards[instance.ProviderInstanceID]
	result := make([]providercontract.PortForward, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ProviderMappingID < result[j].ProviderMappingID })
	return result, nil
}

func (p *Provider) AddPortForward(_ context.Context, request providercontract.AddPortForwardRequest) (*providercontract.Operation, error) {
	if !p.capabilities.NAT {
		return nil, unsupported("nat")
	}
	if request.OperationID == "" || request.IdempotencyKey == "" {
		return nil, unknown("operation and idempotency identities are required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.instances[request.ProviderInstanceID]; !ok {
		return nil, providerError(providercontract.ErrorInstanceNotFound, false, "mock instance not found")
	}
	key := "nat-add:" + request.IdempotencyKey
	if existing, ok := p.operations[key]; ok {
		if p.actionTargets[key] != request.ProviderInstanceID {
			return nil, providerError(providercontract.ErrorInstanceAlreadyExists, false, "idempotency key reused for another instance")
		}
		copy := existing
		return &copy, nil
	}
	mappingID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(request.ProviderInstanceID+":"+request.IdempotencyKey)).String()
	if p.portForwards[request.ProviderInstanceID] == nil {
		p.portForwards[request.ProviderInstanceID] = make(map[string]providercontract.PortForward)
	}
	p.portForwards[request.ProviderInstanceID][mappingID] = providercontract.PortForward{ProviderMappingID: mappingID, Protocol: request.Protocol, PublicIP: request.PublicIP, PublicPort: request.PublicPort, GuestPort: request.GuestPort, Description: request.Description}
	operation := successfulOperation(request.OperationID, "nat-add")
	p.operations[key] = operation
	p.actionTargets[key] = request.ProviderInstanceID
	return &operation, nil
}

func (p *Provider) DeletePortForward(_ context.Context, request providercontract.DeletePortForwardRequest) (*providercontract.Operation, error) {
	if !p.capabilities.NAT {
		return nil, unsupported("nat")
	}
	if request.OperationID == "" || request.IdempotencyKey == "" {
		return nil, unknown("operation and idempotency identities are required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	key := "nat-delete:" + request.IdempotencyKey
	if existing, ok := p.operations[key]; ok {
		if p.actionTargets[key] != request.ProviderInstanceID {
			return nil, providerError(providercontract.ErrorInstanceAlreadyExists, false, "idempotency key reused for another instance")
		}
		copy := existing
		return &copy, nil
	}
	entries := p.portForwards[request.ProviderInstanceID]
	if _, ok := entries[request.ProviderMappingID]; !ok {
		return nil, providerError(providercontract.ErrorInstanceNotFound, false, "mock port forward not found")
	}
	delete(entries, request.ProviderMappingID)
	operation := successfulOperation(request.OperationID, "nat-delete")
	p.operations[key] = operation
	p.actionTargets[key] = request.ProviderInstanceID
	return &operation, nil
}

func (p *Provider) action(request providercontract.InstanceActionRequest, action string, supported bool, state string, remove bool) (*providercontract.Operation, error) {
	if !supported {
		return nil, unsupported(action)
	}
	if request.OperationID == "" || request.IdempotencyKey == "" || request.ProviderInstanceID == "" {
		return nil, unknown("operation, idempotency, and instance identities are required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	key := action + ":" + request.IdempotencyKey
	if existing, ok := p.operations[key]; ok {
		if p.actionTargets[key] != request.ProviderInstanceID {
			return nil, providerError(providercontract.ErrorInstanceAlreadyExists, false, "idempotency key reused for another instance")
		}
		copy := existing
		return &copy, nil
	}
	stored, ok := p.instances[request.ProviderInstanceID]
	if !ok {
		return nil, providerError(providercontract.ErrorInstanceNotFound, false, "mock instance not found")
	}
	if remove {
		delete(p.instances, request.ProviderInstanceID)
		platformID, _ := stored.instance.Metadata["platform_instance_id"].(string)
		delete(p.byPlatformID, platformID)
	} else if state != "" {
		stored.instance.State = state
		p.instances[request.ProviderInstanceID] = stored
	}
	operation := successfulOperation(request.OperationID, action)
	p.operations[key] = operation
	p.actionTargets[key] = request.ProviderInstanceID
	return &operation, nil
}

func successfulOperation(operationID, action string) providercontract.Operation {
	return providercontract.Operation{ProviderOperationID: "mock-" + action + "-" + operationID, Status: "succeeded", Accepted: true, Metadata: map[string]any{"action": action}}
}
func unsupported(action string) *providercontract.Error {
	return providerError(providercontract.ErrorUnsupportedOperation, false, action+" is unsupported")
}
func unknown(message string) *providercontract.Error {
	return providerError(providercontract.ErrorUnknown, false, message)
}
func providerError(code string, retryable bool, message string) *providercontract.Error {
	return &providercontract.Error{Code: code, Retryable: retryable, Provider: "mock", RawCode: code, RawMessage: message}
}
