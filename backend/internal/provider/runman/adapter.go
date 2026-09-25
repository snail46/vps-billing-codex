package runmanprovider

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"strconv"
	"strings"
	"time"
	providercontract "vps-billing/backend/internal/provider"
	"vps-billing/backend/internal/runman"
)

type Adapter struct {
	store       runman.Store
	broker      *runman.Broker
	defaultNode *uuid.UUID
}

func New(store runman.Store, broker *runman.Broker, defaultNode *uuid.UUID) *Adapter {
	return &Adapter{store: store, broker: broker, defaultNode: defaultNode}
}
func NewFactory(store runman.Store, broker *runman.Broker) providercontract.Factory {
	return func(config providercontract.FactoryConfig) (providercontract.Provider, error) {
		var value struct {
			DefaultNodeID string `json:"default_node_id"`
		}
		_ = json.Unmarshal(config.Config, &value)
		var node *uuid.UUID
		if value.DefaultNodeID != "" {
			parsed, e := uuid.Parse(value.DefaultNodeID)
			if e != nil {
				return nil, e
			}
			node = &parsed
		}
		return New(store, broker, node), nil
	}
}
func (a *Adapter) Name() string { return "runman" }
func (a *Adapter) Health(ctx context.Context) (*providercontract.Health, error) {
	if a.defaultNode == nil {
		return &providercontract.Health{Status: "active", Version: "runman-v1", CheckedAt: time.Now().UTC(), Details: map[string]any{"scope": "per-node"}}, nil
	}
	ok, e := a.store.Healthy(ctx, *a.defaultNode, 90*time.Second)
	if e != nil {
		return nil, e
	}
	status := "unavailable"
	if ok {
		status = "active"
	}
	return &providercontract.Health{Status: status, Version: "runman-v1", CheckedAt: time.Now().UTC()}, nil
}
func (a *Adapter) Capabilities(context.Context) (*providercontract.Capabilities, error) {
	return &providercontract.Capabilities{CreateInstance: true, DeleteInstance: true, Start: true, Stop: true, Restart: true, Reinstall: true, ResetPassword: true, Traffic: true, Metrics: true, NAT: true, SharedIPv4: true, PortForward: true, TrafficMeter: true, IPv4: true, IPv6: true, SupportedRuntimes: []string{"podman", "cloudhv", "incus"}}, nil
}
func (a *Adapter) ListImages(ctx context.Context, node string) ([]providercontract.Image, error) {
	id, e := parse(node)
	if e != nil {
		return nil, e
	}
	items, e := a.store.Images(ctx, id)
	if e != nil {
		return nil, normalize(e)
	}
	out := make([]providercontract.Image, 0, len(items))
	for _, v := range items {
		out = append(out, providercontract.Image{ID: v.ID, Name: v.Name})
	}
	return out, nil
}
func (a *Adapter) CreateInstance(ctx context.Context, r providercontract.CreateInstanceRequest) (*providercontract.Operation, error) {
	node, e := parse(r.NodeID)
	if e != nil {
		return nil, e
	}
	vm, e := parse(r.InstanceID)
	if e != nil {
		return nil, e
	}
	band := int32(0)
	if r.BandwidthMbps != nil {
		band = int32(*r.BandwidthMbps)
	}
	password := ""
	if r.RootPassword != nil {
		password = *r.RootPassword
	}
	op := operation(r.OperationID)
	cmd, e := a.broker.Issue(ctx, node, op, r.IdempotencyKey, "create", map[string]any{"vm_id": vm.String(), "cpu": int32(r.CPUCores), "ram_mb": r.MemoryMB, "disk_gb": r.DiskGB, "bandwidth_mbps": band, "os_image": r.Image, "root_password": password})
	return result(cmd, e)
}
func (a *Adapter) GetInstance(ctx context.Context, r providercontract.GetInstanceRequest) (*providercontract.Instance, error) {
	node, e := parse(r.NodeID)
	if e != nil {
		return nil, e
	}
	vm, e := parse(first(r.PlatformInstanceID, r.ProviderInstanceID))
	if e != nil {
		return nil, e
	}
	v, e := a.store.VM(ctx, node, vm)
	if e != nil {
		return nil, normalize(e)
	}
	ipv4, ipv6 := []string{}, []string{}
	for _, ip := range v.IPs {
		if strings.Contains(ip, ":") {
			ipv6 = append(ipv6, ip)
		} else {
			ipv4 = append(ipv4, ip)
		}
	}
	spec, _ := a.store.InstanceSpec(ctx, vm)
	return &providercontract.Instance{ProviderInstanceID: vm.String(), State: v.Status, CPUCores: spec.CPU, MemoryMB: spec.MemoryMB, DiskGB: spec.DiskGB, IPv4: ipv4, IPv6: ipv6, Metadata: map[string]any{"observed_at": v.ObservedAt}}, nil
}
func (a *Adapter) StartInstance(ctx context.Context, r providercontract.InstanceActionRequest) (*providercontract.Operation, error) {
	return a.action(ctx, r, "start")
}
func (a *Adapter) StopInstance(ctx context.Context, r providercontract.InstanceActionRequest) (*providercontract.Operation, error) {
	return a.action(ctx, r, "stop")
}
func (a *Adapter) RestartInstance(ctx context.Context, r providercontract.InstanceActionRequest) (*providercontract.Operation, error) {
	return a.action(ctx, r, "restart")
}
func (a *Adapter) DeleteInstance(ctx context.Context, r providercontract.InstanceActionRequest) (*providercontract.Operation, error) {
	return a.action(ctx, r, "delete")
}
func (a *Adapter) action(ctx context.Context, r providercontract.InstanceActionRequest, kind string) (*providercontract.Operation, error) {
	node, e := parse(r.NodeID)
	if e != nil {
		return nil, e
	}
	vm, e := parse(r.ProviderInstanceID)
	if e != nil {
		return nil, e
	}
	cmd, e := a.broker.Issue(ctx, node, operation(r.OperationID), r.IdempotencyKey, kind, map[string]any{"vm_id": vm.String()})
	return result(cmd, e)
}
func (a *Adapter) ReinstallInstance(ctx context.Context, r providercontract.ReinstallInstanceRequest) (*providercontract.Operation, error) {
	node, e := parse(r.NodeID)
	if e != nil {
		return nil, e
	}
	vm, e := parse(r.ProviderInstanceID)
	if e != nil {
		return nil, e
	}
	spec, e := a.store.InstanceSpec(ctx, vm)
	if e != nil {
		return nil, normalize(e)
	}
	password := ""
	if r.RootPassword != nil {
		password = *r.RootPassword
	}
	cmd, e := a.broker.Issue(ctx, node, operation(r.OperationID), r.IdempotencyKey, "reinstall", map[string]any{"vm_id": vm.String(), "os_image": r.Image, "root_password": password, "cpu": int32(spec.CPU), "ram_mb": spec.MemoryMB, "disk_gb": spec.DiskGB, "bandwidth_mbps": spec.BandwidthMbps})
	return result(cmd, e)
}
func (a *Adapter) ResetPassword(ctx context.Context, r providercontract.ResetPasswordRequest) (*providercontract.Operation, error) {
	node, e := parse(r.NodeID)
	if e != nil {
		return nil, e
	}
	vm, e := parse(r.ProviderInstanceID)
	if e != nil {
		return nil, e
	}
	cmd, e := a.broker.Issue(ctx, node, operation(r.OperationID), r.IdempotencyKey, "reset_password", map[string]any{"vm_id": vm.String(), "root_password": r.RootPassword})
	return result(cmd, e)
}
func (a *Adapter) GetUsage(ctx context.Context, r providercontract.GetInstanceRequest) (*providercontract.Usage, error) {
	node, e := parse(r.NodeID)
	if e != nil {
		return nil, e
	}
	vm, e := parse(first(r.PlatformInstanceID, r.ProviderInstanceID))
	if e != nil {
		return nil, e
	}
	v, e := a.store.VM(ctx, node, vm)
	if e != nil {
		return nil, normalize(e)
	}
	spec, _ := a.store.InstanceSpec(ctx, vm)
	return &providercontract.Usage{CPUPercent: v.CPUPercent, MemoryUsedMB: v.RAMUsedMB, MemoryTotalMB: spec.MemoryMB, DiskTotalGB: float64(spec.DiskGB), ObservedAt: v.ObservedAt}, nil
}
func (a *Adapter) GetTraffic(ctx context.Context, r providercontract.GetTrafficRequest) (*providercontract.Traffic, error) {
	node, e := parse(r.NodeID)
	if e != nil {
		return nil, e
	}
	vm, e := parse(first(r.PlatformInstanceID, r.ProviderInstanceID))
	if e != nil {
		return nil, e
	}
	v, e := a.store.VM(ctx, node, vm)
	if e != nil {
		return nil, normalize(e)
	}
	return &providercontract.Traffic{RXBytes: v.MonthlyIn, TXBytes: v.MonthlyOut, From: r.From, To: r.To}, nil
}
func (a *Adapter) ListPortForwards(ctx context.Context, r providercontract.GetInstanceRequest) ([]providercontract.PortForward, error) {
	node, e := parse(r.NodeID)
	if e != nil {
		return nil, e
	}
	vm, e := parse(first(r.PlatformInstanceID, r.ProviderInstanceID))
	if e != nil {
		return nil, e
	}
	_, e = a.broker.Issue(ctx, node, nil, "list-ports:"+uuid.NewString(), "list_ports", map[string]any{"vm_id": vm.String()})
	if e != nil {
		return nil, normalize(e)
	}
	items, e := a.store.PortForwards(ctx, node, vm)
	if e != nil {
		return nil, normalize(e)
	}
	out := []providercontract.PortForward{}
	for _, v := range items {
		out = append(out, providercontract.PortForward{ProviderMappingID: v.Protocol + ":" + strconv.Itoa(int(v.HostPort)), Protocol: v.Protocol, PublicPort: int(v.HostPort), GuestPort: int(v.GuestPort), Description: v.Description})
	}
	return out, nil
}
func (a *Adapter) AddPortForward(ctx context.Context, r providercontract.AddPortForwardRequest) (*providercontract.Operation, error) {
	node, e := parse(r.NodeID)
	if e != nil {
		return nil, e
	}
	vm, e := parse(r.ProviderInstanceID)
	if e != nil {
		return nil, e
	}
	cmd, e := a.broker.Issue(ctx, node, operation(r.OperationID), r.IdempotencyKey, "add_port", map[string]any{"vm_id": vm.String(), "protocol": r.Protocol, "host_port": r.PublicPort, "guest_port": r.GuestPort, "description": r.Description})
	return result(cmd, e)
}
func (a *Adapter) DeletePortForward(ctx context.Context, r providercontract.DeletePortForwardRequest) (*providercontract.Operation, error) {
	node, e := parse(r.NodeID)
	if e != nil {
		return nil, e
	}
	vm, e := parse(r.ProviderInstanceID)
	if e != nil {
		return nil, e
	}
	parts := strings.Split(r.ProviderMappingID, ":")
	if len(parts) != 2 {
		return nil, &providercontract.Error{Code: providercontract.ErrorUnknown, Provider: "runman", RawMessage: "invalid mapping id"}
	}
	port, e := strconv.Atoi(parts[1])
	if e != nil {
		return nil, e
	}
	cmd, e := a.broker.Issue(ctx, node, operation(r.OperationID), r.IdempotencyKey, "delete_port", map[string]any{"vm_id": vm.String(), "protocol": parts[0], "host_port": port})
	return result(cmd, e)
}
func parse(v string) (uuid.UUID, error) {
	id, e := uuid.Parse(v)
	if e != nil {
		return uuid.Nil, &providercontract.Error{Code: providercontract.ErrorUnknown, Provider: "runman", RawMessage: "invalid uuid", Cause: e}
	}
	return id, nil
}
func operation(v string) *uuid.UUID {
	id, e := uuid.Parse(v)
	if e != nil {
		return nil
	}
	return &id
}
func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
func result(c runman.Command, e error) (*providercontract.Operation, error) {
	if e != nil {
		return nil, normalizeWith(c, e)
	}
	return &providercontract.Operation{ProviderOperationID: c.ID.String(), Status: c.Status, Accepted: c.Status == "succeeded" || c.Status == "dispatched"}, nil
}
func normalize(e error) error { return normalizeWith(runman.Command{}, e) }
func normalizeWith(c runman.Command, e error) error {
	code, retry := providercontract.ErrorUnknown, false
	switch {
	case errors.Is(e, runman.ErrNodeOffline):
		code, retry = providercontract.ErrorNodeOffline, true
	case errors.Is(e, context.DeadlineExceeded):
		code, retry = providercontract.ErrorTimeout, true
	case errors.Is(e, runman.ErrNotFound):
		code = providercontract.ErrorInstanceNotFound
	case errors.Is(e, runman.ErrCommandFailed):
		code = providercontract.ErrorUnknown
	}
	raw := c.Error
	if raw == "" {
		raw = e.Error()
	}
	return &providercontract.Error{Code: code, Retryable: retry, Provider: "runman", RawCode: c.Status, RawMessage: raw, Cause: e}
}

var _ providercontract.Provider = (*Adapter)(nil)
