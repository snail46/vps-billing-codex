package lxdapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	providercontract "vps-billing/backend/internal/provider"
)

const maxResponseBytes = 8 << 20

type Config struct {
	Endpoint          string
	Project           string
	ImageServer       string
	OperationTimeout  time.Duration
	AllowInsecureHTTP bool
	Client            *http.Client
}

type persistedConfig struct {
	Project                 string `json:"project"`
	ImageServer             string `json:"image_server"`
	OperationTimeoutSeconds int    `json:"operation_timeout_seconds"`
}

type Adapter struct {
	baseURL          *url.URL
	project          string
	imageServer      string
	operationTimeout time.Duration
	client           *http.Client
	mu               sync.Mutex
	actions          map[string]providercontract.Operation
	actionTargets    map[string]string
}

type envelope struct {
	Type       string          `json:"type"`
	Status     string          `json:"status"`
	StatusCode int             `json:"status_code"`
	Operation  string          `json:"operation"`
	ErrorCode  int             `json:"error_code"`
	Error      string          `json:"error"`
	Metadata   json.RawMessage `json:"metadata"`
}

func NewFromFactory(factoryConfig providercontract.FactoryConfig) (providercontract.Provider, error) {
	var stored persistedConfig
	if len(factoryConfig.Config) > 0 {
		if err := json.Unmarshal(factoryConfig.Config, &stored); err != nil {
			return nil, fmt.Errorf("decode LXD provider config: %w", err)
		}
	}
	timeout := time.Duration(stored.OperationTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	config := Config{Endpoint: factoryConfig.Endpoint, Project: stored.Project, ImageServer: stored.ImageServer, OperationTimeout: timeout}
	if strings.HasPrefix(factoryConfig.Endpoint, "https://") {
		if factoryConfig.CredentialRef == nil {
			return nil, errors.New("LXD credential_ref is required for HTTPS")
		}
		client, err := tlsClient(*factoryConfig.CredentialRef, timeout)
		if err != nil {
			return nil, err
		}
		config.Client = client
	}
	return New(config)
}

func New(config Config) (*Adapter, error) {
	baseURL, err := url.Parse(strings.TrimRight(config.Endpoint, "/"))
	if err != nil || baseURL.Host == "" {
		return nil, errors.New("LXD endpoint is invalid")
	}
	if baseURL.Scheme != "https" && !(baseURL.Scheme == "http" && config.AllowInsecureHTTP) {
		return nil, errors.New("LXD endpoint must use HTTPS")
	}
	if config.Project == "" {
		config.Project = "default"
	}
	if config.ImageServer == "" {
		config.ImageServer = "https://cloud-images.ubuntu.com/releases"
	}
	if config.OperationTimeout <= 0 {
		config.OperationTimeout = 30 * time.Second
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: config.OperationTimeout + 5*time.Second}
	}
	return &Adapter{baseURL: baseURL, project: config.Project, imageServer: config.ImageServer, operationTimeout: config.OperationTimeout, client: config.Client, actions: make(map[string]providercontract.Operation), actionTargets: make(map[string]string)}, nil
}

func tlsClient(reference string, timeout time.Duration) (*http.Client, error) {
	for _, character := range reference {
		if !(character == '_' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9') {
			return nil, errors.New("LXD credential_ref must contain only A-Z, 0-9, and underscore")
		}
	}
	certificatePEM, keyPEM, caPEM := os.Getenv(reference+"_CLIENT_CERT_PEM"), os.Getenv(reference+"_CLIENT_KEY_PEM"), os.Getenv(reference+"_SERVER_CA_PEM")
	if certificatePEM == "" || keyPEM == "" || caPEM == "" {
		return nil, errors.New("LXD mTLS credential environment is incomplete")
	}
	certificate, err := tls.X509KeyPair([]byte(certificatePEM), []byte(keyPEM))
	if err != nil {
		return nil, fmt.Errorf("load LXD client certificate: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(caPEM)) {
		return nil, errors.New("load LXD server CA: no certificate found")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}, RootCAs: roots}}
	return &http.Client{Transport: transport, Timeout: timeout + 5*time.Second}, nil
}

func (a *Adapter) Name() string { return "lxdapi" }

func (a *Adapter) Health(ctx context.Context) (*providercontract.Health, error) {
	var metadata struct {
		Environment struct {
			ServerVersion string `json:"server_version"`
		} `json:"environment"`
	}
	if _, err := a.request(ctx, http.MethodGet, "/1.0", nil, nil, &metadata); err != nil {
		return nil, err
	}
	return &providercontract.Health{Status: "healthy", Version: metadata.Environment.ServerVersion, CheckedAt: time.Now().UTC(), Details: map[string]any{"project": a.project}}, nil
}

func (a *Adapter) Capabilities(context.Context) (*providercontract.Capabilities, error) {
	return &providercontract.Capabilities{CreateInstance: true, DeleteInstance: true, Start: true, Stop: true, Restart: true, Reinstall: true, ResetPassword: false, Traffic: true, Metrics: true, NAT: false, SharedIPv4: false, PortForward: false, TrafficMeter: true, IPv4: true, IPv6: true, Snapshot: false, Console: false, Firewall: false, SupportedRuntimes: []string{"kvm", "lxc"}}, nil
}

func (a *Adapter) ListImages(ctx context.Context, _ string) ([]providercontract.Image, error) {
	var images []struct {
		Fingerprint string            `json:"fingerprint"`
		Properties  map[string]string `json:"properties"`
		Aliases     []struct {
			Name string `json:"name"`
		} `json:"aliases"`
	}
	if _, err := a.request(ctx, http.MethodGet, "/1.0/images", url.Values{"recursion": {"1"}}, nil, &images); err != nil {
		return nil, err
	}
	result := make([]providercontract.Image, 0, len(images))
	for _, image := range images {
		id := image.Fingerprint
		if len(image.Aliases) > 0 {
			id = image.Aliases[0].Name
		}
		result = append(result, providercontract.Image{ID: id, Name: image.Properties["description"], OS: image.Properties["os"], Version: image.Properties["release"], Arch: image.Properties["architecture"], Description: image.Properties["description"]})
	}
	return result, nil
}

func (a *Adapter) CreateInstance(ctx context.Context, request providercontract.CreateInstanceRequest) (*providercontract.Operation, error) {
	if request.OperationID == "" || request.IdempotencyKey == "" || request.InstanceID == "" || request.NodeID == "" || request.Image == "" {
		return nil, normalized(providercontract.ErrorUnknown, false, 0, "invalid create request", nil)
	}
	cacheKey := "create:" + request.IdempotencyKey
	if operation, err, found := a.cachedOperation(cacheKey, request.InstanceID); found {
		return operation, err
	}
	name := instanceName(request.InstanceID)
	existing, err := a.instanceDefinition(ctx, name)
	if err == nil {
		if existing.Config["user.vps_billing.instance_id"] != request.InstanceID || existing.Config["user.vps_billing.idempotency_key"] != request.IdempotencyKey {
			return nil, normalized(providercontract.ErrorInstanceAlreadyExists, false, http.StatusConflict, "LXD instance identity conflict", nil)
		}
		result := providercontract.Operation{ProviderOperationID: "existing:" + name, Status: "succeeded", Accepted: true, Metadata: map[string]any{"idempotent_replay": true}}
		a.storeOperation(cacheKey, request.InstanceID, result)
		return &result, nil
	}
	if !isCode(err, providercontract.ErrorInstanceNotFound) {
		return nil, err
	}
	instanceType := "container"
	if request.Virtualization == "kvm" {
		instanceType = "virtual-machine"
	}
	body := map[string]any{
		"name": name, "type": instanceType, "start": true,
		"source": map[string]any{"type": "image", "alias": request.Image, "mode": "pull", "protocol": "simplestreams", "server": a.imageServer},
		"config": map[string]string{
			"limits.cpu": strconv.FormatFloat(request.CPUCores, 'f', -1, 64), "limits.memory": fmt.Sprintf("%dMiB", request.MemoryMB),
			"user.vps_billing.instance_id": request.InstanceID, "user.vps_billing.operation_id": request.OperationID, "user.vps_billing.idempotency_key": request.IdempotencyKey,
		},
		"devices": map[string]map[string]string{"root": {"type": "disk", "path": "/", "size": fmt.Sprintf("%dGiB", request.DiskGB)}},
	}
	query := url.Values{"target": {request.NodeID}}
	env, err := a.request(ctx, http.MethodPost, "/1.0/instances", query, body, nil)
	if err != nil {
		return nil, err
	}
	if err := a.wait(ctx, env.Operation); err != nil {
		return nil, err
	}
	result := providercontract.Operation{ProviderOperationID: operationID(env.Operation), Status: "succeeded", Accepted: true}
	a.storeOperation(cacheKey, request.InstanceID, result)
	return &result, nil
}

type instanceDefinition struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Config    map[string]string `json:"config"`
	CreatedAt time.Time         `json:"created_at"`
}

type instanceState struct {
	Status string `json:"status"`
	Memory struct {
		Usage int64 `json:"usage"`
	} `json:"memory"`
	Network map[string]struct {
		Addresses []struct {
			Family  string `json:"family"`
			Address string `json:"address"`
			Scope   string `json:"scope"`
		} `json:"addresses"`
		Counters struct {
			BytesReceived int64 `json:"bytes_received"`
			BytesSent     int64 `json:"bytes_sent"`
		} `json:"counters"`
	} `json:"network"`
}

func (a *Adapter) GetInstance(ctx context.Context, request providercontract.GetInstanceRequest) (*providercontract.Instance, error) {
	name := request.ProviderInstanceID
	if name == "" {
		name = instanceName(request.PlatformInstanceID)
	}
	definition, err := a.instanceDefinition(ctx, name)
	if err != nil {
		return nil, err
	}
	state, err := a.state(ctx, name)
	if err != nil {
		return nil, err
	}
	result := &providercontract.Instance{ProviderInstanceID: name, State: mapState(state.Status), CreatedAt: definition.CreatedAt, Metadata: map[string]any{"lxd_status": state.Status}}
	result.CPUCores, _ = strconv.ParseFloat(definition.Config["limits.cpu"], 64)
	result.MemoryMB = parseSize(definition.Config["limits.memory"], "MiB")
	for _, network := range state.Network {
		for _, address := range network.Addresses {
			if address.Scope == "link" || address.Address == "" {
				continue
			}
			if address.Family == "inet" {
				result.IPv4 = append(result.IPv4, address.Address)
			} else if address.Family == "inet6" {
				result.IPv6 = append(result.IPv6, address.Address)
			}
		}
	}
	return result, nil
}

func (a *Adapter) instanceDefinition(ctx context.Context, name string) (instanceDefinition, error) {
	var result instanceDefinition
	_, err := a.request(ctx, http.MethodGet, "/1.0/instances/"+url.PathEscape(name), nil, nil, &result)
	return result, err
}

func (a *Adapter) state(ctx context.Context, name string) (instanceState, error) {
	var result instanceState
	_, err := a.request(ctx, http.MethodGet, "/1.0/instances/"+url.PathEscape(name)+"/state", nil, nil, &result)
	return result, err
}

func (a *Adapter) StartInstance(ctx context.Context, request providercontract.InstanceActionRequest) (*providercontract.Operation, error) {
	return a.stateAction(ctx, request, "start")
}
func (a *Adapter) StopInstance(ctx context.Context, request providercontract.InstanceActionRequest) (*providercontract.Operation, error) {
	return a.stateAction(ctx, request, "stop")
}
func (a *Adapter) RestartInstance(ctx context.Context, request providercontract.InstanceActionRequest) (*providercontract.Operation, error) {
	return a.stateAction(ctx, request, "restart")
}

func (a *Adapter) stateAction(ctx context.Context, request providercontract.InstanceActionRequest, action string) (*providercontract.Operation, error) {
	if request.OperationID == "" || request.IdempotencyKey == "" || request.ProviderInstanceID == "" {
		return nil, normalized(providercontract.ErrorUnknown, false, 0, "invalid instance action request", nil)
	}
	key := action + ":" + request.IdempotencyKey
	if operation, err, found := a.cachedOperation(key, request.ProviderInstanceID); found {
		return operation, err
	}
	env, err := a.request(ctx, http.MethodPut, "/1.0/instances/"+url.PathEscape(request.ProviderInstanceID)+"/state", nil, map[string]any{"action": action, "timeout": int(a.operationTimeout.Seconds()), "force": action == "stop"}, nil)
	if err != nil {
		return nil, err
	}
	if err := a.wait(ctx, env.Operation); err != nil {
		return nil, err
	}
	result := providercontract.Operation{ProviderOperationID: operationID(env.Operation), Status: "succeeded", Accepted: true}
	a.storeOperation(key, request.ProviderInstanceID, result)
	return &result, nil
}

func (a *Adapter) ReinstallInstance(ctx context.Context, request providercontract.ReinstallInstanceRequest) (*providercontract.Operation, error) {
	if request.OperationID == "" || request.IdempotencyKey == "" || request.ProviderInstanceID == "" || request.Image == "" {
		return nil, normalized(providercontract.ErrorUnknown, false, 0, "invalid reinstall request", nil)
	}
	cacheKey := "reinstall:" + request.IdempotencyKey
	if operation, err, found := a.cachedOperation(cacheKey, request.ProviderInstanceID); found {
		return operation, err
	}
	current, err := a.GetInstance(ctx, providercontract.GetInstanceRequest{ProviderInstanceID: request.ProviderInstanceID})
	if err != nil {
		return nil, err
	}
	if current.State == "running" {
		if _, err := a.StopInstance(ctx, request.InstanceActionRequest); err != nil {
			return nil, err
		}
	}
	env, err := a.request(ctx, http.MethodPost, "/1.0/instances/"+url.PathEscape(request.ProviderInstanceID)+"/rebuild", nil, map[string]any{"source": map[string]any{"type": "image", "alias": request.Image, "protocol": "simplestreams", "server": a.imageServer}}, nil)
	if err != nil {
		return nil, err
	}
	if err := a.wait(ctx, env.Operation); err != nil {
		return nil, err
	}
	result, err := a.StartInstance(ctx, request.InstanceActionRequest)
	if err != nil {
		return nil, err
	}
	a.storeOperation(cacheKey, request.ProviderInstanceID, *result)
	return result, nil
}

func (a *Adapter) ResetPassword(context.Context, providercontract.ResetPasswordRequest) (*providercontract.Operation, error) {
	return nil, unsupported("reset_password")
}

func (a *Adapter) DeleteInstance(ctx context.Context, request providercontract.InstanceActionRequest) (*providercontract.Operation, error) {
	if request.OperationID == "" || request.IdempotencyKey == "" || request.ProviderInstanceID == "" {
		return nil, normalized(providercontract.ErrorUnknown, false, 0, "invalid delete request", nil)
	}
	cacheKey := "delete:" + request.IdempotencyKey
	if operation, err, found := a.cachedOperation(cacheKey, request.ProviderInstanceID); found {
		return operation, err
	}
	env, err := a.request(ctx, http.MethodDelete, "/1.0/instances/"+url.PathEscape(request.ProviderInstanceID), url.Values{"force": {"1"}}, nil, nil)
	if isCode(err, providercontract.ErrorInstanceNotFound) {
		result := providercontract.Operation{ProviderOperationID: "not-found", Status: "succeeded", Accepted: true, Metadata: map[string]any{"idempotent_replay": true}}
		a.storeOperation(cacheKey, request.ProviderInstanceID, result)
		return &result, nil
	}
	if err != nil {
		return nil, err
	}
	if err := a.wait(ctx, env.Operation); err != nil {
		return nil, err
	}
	result := providercontract.Operation{ProviderOperationID: operationID(env.Operation), Status: "succeeded", Accepted: true}
	a.storeOperation(cacheKey, request.ProviderInstanceID, result)
	return &result, nil
}

func (a *Adapter) cachedOperation(key, target string) (*providercontract.Operation, error, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	existing, ok := a.actions[key]
	if !ok {
		return nil, nil, false
	}
	if a.actionTargets[key] != target {
		return nil, normalized(providercontract.ErrorInstanceAlreadyExists, false, http.StatusConflict, "idempotency key reused for another instance", nil), true
	}
	copy := existing
	if copy.Metadata == nil {
		copy.Metadata = make(map[string]any)
	}
	copy.Metadata["idempotent_replay"] = true
	return &copy, nil, true
}

func (a *Adapter) storeOperation(key, target string, operation providercontract.Operation) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions[key], a.actionTargets[key] = operation, target
}

func (a *Adapter) GetUsage(ctx context.Context, request providercontract.GetInstanceRequest) (*providercontract.Usage, error) {
	name := request.ProviderInstanceID
	if name == "" {
		name = instanceName(request.PlatformInstanceID)
	}
	state, err := a.state(ctx, name)
	if err != nil {
		return nil, err
	}
	return &providercontract.Usage{MemoryUsedMB: state.Memory.Usage / (1024 * 1024), ObservedAt: time.Now().UTC()}, nil
}

func (a *Adapter) GetTraffic(ctx context.Context, request providercontract.GetTrafficRequest) (*providercontract.Traffic, error) {
	name := request.ProviderInstanceID
	if name == "" {
		name = instanceName(request.PlatformInstanceID)
	}
	state, err := a.state(ctx, name)
	if err != nil {
		return nil, err
	}
	result := &providercontract.Traffic{From: request.From, To: request.To}
	for _, network := range state.Network {
		result.RXBytes += network.Counters.BytesReceived
		result.TXBytes += network.Counters.BytesSent
	}
	return result, nil
}

func (a *Adapter) ListPortForwards(context.Context, providercontract.GetInstanceRequest) ([]providercontract.PortForward, error) {
	return nil, unsupported("nat")
}
func (a *Adapter) AddPortForward(context.Context, providercontract.AddPortForwardRequest) (*providercontract.Operation, error) {
	return nil, unsupported("nat")
}
func (a *Adapter) DeletePortForward(context.Context, providercontract.DeletePortForwardRequest) (*providercontract.Operation, error) {
	return nil, unsupported("nat")
}

func (a *Adapter) wait(ctx context.Context, operationURL string) error {
	parsed, err := url.Parse(operationURL)
	if err != nil || !strings.HasPrefix(parsed.Path, "/1.0/operations/") {
		return normalized(providercontract.ErrorUnknown, false, 0, "LXD returned an invalid operation URL", err)
	}
	query := parsed.Query()
	query.Set("timeout", strconv.Itoa(int(a.operationTimeout.Seconds())))
	var operation struct {
		Status     string `json:"status"`
		StatusCode int    `json:"status_code"`
		Err        string `json:"err"`
	}
	if _, err := a.request(ctx, http.MethodGet, parsed.Path+"/wait", query, nil, &operation); err != nil {
		return err
	}
	if operation.Err != "" || operation.StatusCode >= 400 {
		return normalizeHTTP(operation.StatusCode, operation.Err, nil)
	}
	if operation.StatusCode < 200 {
		return normalized(providercontract.ErrorTimeout, true, operation.StatusCode, "LXD operation did not reach a terminal state", nil)
	}
	return nil
}

func (a *Adapter) request(ctx context.Context, method, path string, query url.Values, body any, metadata any) (envelope, error) {
	requestURL := *a.baseURL
	requestURL.Path = strings.TrimRight(a.baseURL.Path, "/") + path
	if query == nil {
		query = make(url.Values)
	}
	if !strings.HasSuffix(path, "/1.0") && path != "/1.0" {
		query.Set("project", a.project)
	}
	requestURL.RawQuery = query.Encode()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return envelope{}, err
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), reader)
	if err != nil {
		return envelope{}, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := a.client.Do(request)
	if err != nil {
		return envelope{}, normalizeTransport(err)
	}
	defer func() { _ = response.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return envelope{}, normalizeTransport(err)
	}
	if len(payload) > maxResponseBytes {
		return envelope{}, normalized(providercontract.ErrorUnknown, false, response.StatusCode, "LXD response exceeded limit", nil)
	}
	var result envelope
	if err := json.Unmarshal(payload, &result); err != nil {
		return envelope{}, normalized(providercontract.ErrorUnknown, false, response.StatusCode, "invalid LXD response", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || result.Error != "" {
		return envelope{}, normalizeHTTP(response.StatusCode, result.Error, nil)
	}
	if metadata != nil && len(result.Metadata) > 0 && string(result.Metadata) != "null" {
		if err := json.Unmarshal(result.Metadata, metadata); err != nil {
			return envelope{}, normalized(providercontract.ErrorUnknown, false, response.StatusCode, "invalid LXD metadata", err)
		}
	}
	return result, nil
}

func normalizeTransport(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return normalized(providercontract.ErrorTimeout, true, 0, err.Error(), err)
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return normalized(providercontract.ErrorTimeout, true, 0, err.Error(), err)
	}
	return normalized(providercontract.ErrorUnavailable, true, 0, err.Error(), err)
}

func normalizeHTTP(status int, message string, cause error) error {
	lower := strings.ToLower(message)
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return normalized(providercontract.ErrorAuthFailed, false, status, message, cause)
	case status == http.StatusNotFound && strings.Contains(lower, "image"):
		return normalized(providercontract.ErrorImageNotFound, false, status, message, cause)
	case status == http.StatusNotFound:
		return normalized(providercontract.ErrorInstanceNotFound, false, status, message, cause)
	case status == http.StatusConflict || strings.Contains(lower, "already exists"):
		return normalized(providercontract.ErrorInstanceAlreadyExists, false, status, message, cause)
	case status == http.StatusInsufficientStorage || strings.Contains(lower, "no space"):
		return normalized(providercontract.ErrorResourceExhausted, true, status, message, cause)
	case status == http.StatusGatewayTimeout:
		return normalized(providercontract.ErrorTimeout, true, status, message, cause)
	case status == http.StatusTooManyRequests || status >= 500:
		return normalized(providercontract.ErrorUnavailable, true, status, message, cause)
	default:
		return normalized(providercontract.ErrorUnknown, false, status, message, cause)
	}
}

func normalized(code string, retryable bool, rawCode int, message string, cause error) *providercontract.Error {
	if len(message) > 512 {
		message = message[:512]
	}
	return &providercontract.Error{Code: code, Retryable: retryable, Provider: "lxdapi", RawCode: strconv.Itoa(rawCode), RawMessage: message, Cause: cause}
}
func unsupported(action string) *providercontract.Error {
	return normalized(providercontract.ErrorUnsupportedOperation, false, 0, action+" is unsupported by the LXD adapter", nil)
}
func isCode(err error, code string) bool {
	var target *providercontract.Error
	return errors.As(err, &target) && target.Code == code
}
func instanceName(platformID string) string { return "vps-" + strings.ToLower(platformID) }
func operationID(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 0 {
		return value
	}
	return parts[len(parts)-1]
}
func mapState(value string) string {
	switch strings.ToLower(value) {
	case "running":
		return "running"
	case "stopped":
		return "stopped"
	case "frozen":
		return "suspended"
	case "starting":
		return "provisioning"
	case "stopping":
		return "stopping"
	default:
		return "unknown"
	}
}
func parseSize(value, suffix string) int64 {
	value = strings.TrimSuffix(value, suffix)
	result, _ := strconv.ParseInt(value, 10, 64)
	return result
}
