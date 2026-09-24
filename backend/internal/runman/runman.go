package runman

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNodeOffline = errors.New("runman node offline")
var ErrCommandFailed = errors.New("runman command failed")
var ErrNotFound = errors.New("runman resource not found")

type Command struct {
	ID      uuid.UUID
	NodeID  uuid.UUID
	Type    string
	Payload json.RawMessage
	Status  string
	Result  json.RawMessage
	Error   string
}
type VMState struct {
	InstanceID                                              uuid.UUID
	Status                                                  string
	CPUPercent                                              float64
	RAMUsedMB, TrafficIn, TrafficOut, MonthlyIn, MonthlyOut int64
	IPs                                                     []string
	ObservedAt                                              time.Time
}
type Image struct{ ID, Name string }
type PortForward struct {
	Protocol    string `json:"protocol"`
	HostPort    int32  `json:"host_port"`
	GuestPort   int32  `json:"guest_port"`
	Description string `json:"description"`
}
type InstanceSpec struct {
	CPU              float64
	MemoryMB, DiskGB int64
	BandwidthMbps    int32
}
type Heartbeat struct {
	Timestamp                                                           time.Time
	CPUPercent                                                          float64
	RAMUsedMB, DiskUsedGB, NetInBPS, NetOutBPS, RAMTotalMB, DiskTotalGB int64
	CPUs, BandwidthMbps                                                 int32
	VirtType, EntryHost, EntryIPv6                                      string
	VMs                                                                 []VMState
	Images                                                              []Image
}

type Store interface {
	Authenticate(context.Context, []byte) (uuid.UUID, error)
	Connected(context.Context, uuid.UUID, uuid.UUID, string) error
	Disconnected(context.Context, uuid.UUID, uuid.UUID) error
	ClaimMessage(context.Context, uuid.UUID, string) (bool, error)
	ReleaseMessage(context.Context, uuid.UUID, string) error
	Heartbeat(context.Context, uuid.UUID, Heartbeat) error
	CreateCommand(context.Context, uuid.UUID, *uuid.UUID, string, string, any) (Command, error)
	Pending(context.Context, uuid.UUID) ([]Command, error)
	MarkDispatched(context.Context, uuid.UUID) error
	Complete(context.Context, uuid.UUID, bool, json.RawMessage, string) error
	Command(context.Context, uuid.UUID) (Command, error)
	VM(context.Context, uuid.UUID, uuid.UUID) (VMState, error)
	InstanceSpec(context.Context, uuid.UUID) (InstanceSpec, error)
	Images(context.Context, uuid.UUID) ([]Image, error)
	PortForwards(context.Context, uuid.UUID, uuid.UUID) ([]PortForward, error)
	SavePortForwards(context.Context, uuid.UUID, uuid.UUID, []PortForward) error
	Healthy(context.Context, uuid.UUID, time.Duration) (bool, error)
}

type Sender interface {
	Send(context.Context, Command) error
	Online(uuid.UUID) bool
}
type Broker struct {
	store   Store
	sender  Sender
	timeout time.Duration
}

func NewBroker(store Store, sender Sender, timeout time.Duration) *Broker {
	return &Broker{store: store, sender: sender, timeout: timeout}
}
func (b *Broker) Issue(ctx context.Context, nodeID uuid.UUID, operationID *uuid.UUID, key, kind string, payload any) (Command, error) {
	cmd, err := b.store.CreateCommand(ctx, nodeID, operationID, key, kind, payload)
	if err != nil {
		return Command{}, err
	}
	if cmd.Status == "succeeded" {
		return cmd, nil
	}
	if cmd.Status == "failed" {
		return cmd, ErrCommandFailed
	}
	if !b.sender.Online(nodeID) {
		return cmd, ErrNodeOffline
	}
	if cmd.Status == "queued" {
		if err = b.sender.Send(ctx, cmd); err != nil {
			return cmd, ErrNodeOffline
		}
		_ = b.store.MarkDispatched(ctx, cmd.ID)
	}
	deadline := time.NewTimer(b.timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return cmd, ctx.Err()
		case <-deadline.C:
			return cmd, context.DeadlineExceeded
		case <-ticker.C:
			latest, e := b.store.Command(ctx, cmd.ID)
			if e != nil {
				return cmd, e
			}
			if latest.Status == "succeeded" {
				return latest, nil
			}
			if latest.Status == "failed" {
				return latest, ErrCommandFailed
			}
		}
	}
}
