package infrastructure

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrResourceExhausted = errors.New("no node has sufficient compatible capacity")
	ErrInvalidRequest    = errors.New("infrastructure request is invalid")
	ErrReservationState  = errors.New("reservation state does not allow this action")
	ErrReservationAbsent = errors.New("reservation not found")
)

type ProviderRecord struct {
	ID            uuid.UUID       `json:"id"`
	Name          string          `json:"name"`
	ProviderType  string          `json:"provider_type"`
	Endpoint      *string         `json:"endpoint,omitempty"`
	CredentialRef *string         `json:"credential_ref,omitempty"`
	Status        string          `json:"status"`
	Config        json.RawMessage `json:"config"`
	Capabilities  json.RawMessage `json:"capabilities"`
}

type NodeGroup struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Region string    `json:"region"`
	Status string    `json:"status"`
}

type Capacity struct {
	CPUCores     float64 `json:"cpu_cores"`
	MemoryMB     int64   `json:"memory_mb"`
	DiskGB       int64   `json:"disk_gb"`
	IPv4Count    int32   `json:"ipv4_count"`
	IPv6Count    int32   `json:"ipv6_count"`
	NATPortCount int32   `json:"nat_port_count"`
}

type Node struct {
	ID             uuid.UUID       `json:"id"`
	ProviderID     uuid.UUID       `json:"provider_id"`
	NodeGroupID    *uuid.UUID      `json:"node_group_id,omitempty"`
	ProviderNodeID *string         `json:"provider_node_id,omitempty"`
	Name           string          `json:"name"`
	Region         string          `json:"region"`
	Status         string          `json:"status"`
	Total          Capacity        `json:"total"`
	Allocated      Capacity        `json:"allocated"`
	Reserved       Capacity        `json:"reserved"`
	Weight         int32           `json:"weight"`
	Capabilities   json.RawMessage `json:"capabilities"`
	LastSeenAt     *time.Time      `json:"last_seen_at,omitempty"`
	Version        int64           `json:"version"`
}

type Reservation struct {
	ID          uuid.UUID `json:"id"`
	NodeID      uuid.UUID `json:"node_id"`
	OperationID uuid.UUID `json:"operation_id"`
	Resources   Capacity  `json:"resources"`
	Status      string    `json:"status"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type ScheduleRequest struct {
	NodeGroupID          uuid.UUID
	Resources            Capacity
	RequiredCapabilities map[string]any
	TTL                  time.Duration
}
