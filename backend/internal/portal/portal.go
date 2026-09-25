package portal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound       = errors.New("portal resource not found")
	ErrInvalidRequest = errors.New("portal request is invalid")
	ErrStateConflict  = errors.New("instance state does not allow the action")
	ErrTicketClosed   = errors.New("ticket is closed")
	ErrUnsupported    = errors.New("capability is not supported")
	ErrQuotaExceeded  = errors.New("quota is exceeded")
)

type Instance struct {
	ID                 uuid.UUID       `json:"id"`
	Name               string          `json:"name"`
	DesiredState       string          `json:"desired_state"`
	ObservedState      string          `json:"observed_state"`
	CPUCores           float64         `json:"cpu_cores"`
	MemoryMB           int32           `json:"memory_mb"`
	DiskGB             int32           `json:"disk_gb"`
	TrafficLimitGB     *int64          `json:"traffic_limit_gb"`
	BandwidthMbps      *int32          `json:"bandwidth_mbps"`
	ImageID            *string         `json:"image_id"`
	PrimaryIPv4        string          `json:"primary_ipv4"`
	PrimaryIPv6        string          `json:"primary_ipv6"`
	LastSyncedAt       *time.Time      `json:"last_synced_at"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	SubscriptionID     uuid.UUID       `json:"subscription_id"`
	SubscriptionStatus string          `json:"subscription_status"`
	CurrentPeriodEnd   *time.Time      `json:"current_period_end"`
	PlanSlug           string          `json:"plan_slug"`
	PlanName           json.RawMessage `json:"plan_name_i18n"`
}

type Network struct {
	ID        uuid.UUID `json:"id"`
	Type      string    `json:"type"`
	Address   string    `json:"address"`
	Gateway   string    `json:"gateway"`
	Prefix    *int32    `json:"prefix"`
	CreatedAt time.Time `json:"created_at"`
}

type Traffic struct {
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	RXBytes     int64     `json:"rx_bytes"`
	TXBytes     int64     `json:"tx_bytes"`
	Source      string    `json:"source"`
}
type UsageSummary struct {
	PeriodStart    time.Time `json:"period_start"`
	PeriodEnd      time.Time `json:"period_end"`
	IncludedBytes  int64     `json:"included_bytes"`
	UsedBytes      int64     `json:"used_bytes"`
	OverageBytes   int64     `json:"overage_bytes"`
	EstimatedMinor int64     `json:"estimated_minor"`
	Currency       string    `json:"currency"`
}

type PortForward struct {
	ID                uuid.UUID  `json:"id"`
	Protocol          string     `json:"protocol"`
	PublicIP          string     `json:"public_ip"`
	PublicPort        int32      `json:"public_port"`
	GuestPort         int32      `json:"guest_port"`
	Description       string     `json:"description"`
	Status            string     `json:"status"`
	ProviderMappingID *string    `json:"provider_mapping_id,omitempty"`
	OperationID       *uuid.UUID `json:"operation_id,omitempty"`
	ErrorCode         *string    `json:"error_code,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type PortForwardContext struct {
	InstanceID uuid.UUID
	Quota      int32
	Active     int32
	Supported  bool
}

type Notification struct {
	ID         uuid.UUID       `json:"id"`
	Type       string          `json:"type"`
	TitleKey   string          `json:"title_key"`
	MessageKey string          `json:"message_key"`
	Parameters json.RawMessage `json:"parameters"`
	Severity   string          `json:"severity"`
	ReadAt     *time.Time      `json:"read_at"`
	CreatedAt  time.Time       `json:"created_at"`
}

type Ticket struct {
	ID        uuid.UUID       `json:"id"`
	TicketNo  string          `json:"ticket_no"`
	Subject   string          `json:"subject"`
	Status    string          `json:"status"`
	Priority  string          `json:"priority"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	ClosedAt  *time.Time      `json:"closed_at"`
	Messages  []TicketMessage `json:"messages,omitempty"`
}

type TicketMessage struct {
	ID         uuid.UUID `json:"id"`
	SenderType string    `json:"sender_type"`
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"created_at"`
}

type ActionContext struct {
	InstanceID         uuid.UUID
	ProviderID         uuid.UUID
	ProviderInstanceID string
	NodeID             uuid.UUID
	ProviderNodeID     string
	ImageID            string
	DesiredState       string
	ObservedState      string
}

type Repository interface {
	ListInstances(context.Context, uuid.UUID) ([]Instance, error)
	GetInstance(context.Context, uuid.UUID, uuid.UUID) (Instance, error)
	ListNetworks(context.Context, uuid.UUID, uuid.UUID) ([]Network, error)
	ListTraffic(context.Context, uuid.UUID, uuid.UUID) ([]Traffic, error)
	UsageSummary(context.Context, uuid.UUID, uuid.UUID) (UsageSummary, error)
	ListPortForwards(context.Context, uuid.UUID, uuid.UUID) ([]PortForward, error)
	PortForwardContext(context.Context, uuid.UUID, uuid.UUID) (PortForwardContext, error)
	GetPortForward(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (PortForward, error)
	ActionContext(context.Context, uuid.UUID, uuid.UUID) (ActionContext, error)
	ListNotifications(context.Context, uuid.UUID) ([]Notification, error)
	MarkNotificationRead(context.Context, uuid.UUID, uuid.UUID) error
	ListTickets(context.Context, uuid.UUID) ([]Ticket, error)
	GetTicket(context.Context, uuid.UUID, uuid.UUID) (Ticket, error)
	CreateTicket(context.Context, uuid.UUID, string, string, string) (Ticket, error)
	AddTicketMessage(context.Context, uuid.UUID, uuid.UUID, string) (TicketMessage, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) ListInstances(ctx context.Context, userID uuid.UUID) ([]Instance, error) {
	return s.repository.ListInstances(ctx, userID)
}
func (s *Service) GetInstance(ctx context.Context, userID, id uuid.UUID) (Instance, error) {
	return s.repository.GetInstance(ctx, userID, id)
}
func (s *Service) ListNetworks(ctx context.Context, userID, id uuid.UUID) ([]Network, error) {
	return s.repository.ListNetworks(ctx, userID, id)
}
func (s *Service) ListTraffic(ctx context.Context, userID, id uuid.UUID) ([]Traffic, error) {
	return s.repository.ListTraffic(ctx, userID, id)
}
func (s *Service) UsageSummary(ctx context.Context, userID, id uuid.UUID) (UsageSummary, error) {
	return s.repository.UsageSummary(ctx, userID, id)
}
func (s *Service) ListPortForwards(ctx context.Context, userID, id uuid.UUID) ([]PortForward, error) {
	return s.repository.ListPortForwards(ctx, userID, id)
}
func (s *Service) ValidatePortForwardAdd(ctx context.Context, userID, instanceID uuid.UUID, protocol string, publicPort, guestPort int32, description string) error {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if (protocol != "tcp" && protocol != "udp") || publicPort < 1 || publicPort > 65535 || guestPort < 1 || guestPort > 65535 || len(strings.TrimSpace(description)) > 255 {
		return ErrInvalidRequest
	}
	value, err := s.repository.PortForwardContext(ctx, userID, instanceID)
	if err != nil {
		return err
	}
	if !value.Supported {
		return ErrUnsupported
	}
	if value.Quota <= 0 || value.Active >= value.Quota {
		return ErrQuotaExceeded
	}
	return nil
}
func (s *Service) GetPortForward(ctx context.Context, userID, instanceID, id uuid.UUID) (PortForward, error) {
	return s.repository.GetPortForward(ctx, userID, instanceID, id)
}
func (s *Service) ActionContext(ctx context.Context, userID, id uuid.UUID) (ActionContext, error) {
	return s.repository.ActionContext(ctx, userID, id)
}
func (s *Service) ListNotifications(ctx context.Context, userID uuid.UUID) ([]Notification, error) {
	return s.repository.ListNotifications(ctx, userID)
}
func (s *Service) MarkNotificationRead(ctx context.Context, userID, id uuid.UUID) error {
	return s.repository.MarkNotificationRead(ctx, userID, id)
}
func (s *Service) ListTickets(ctx context.Context, userID uuid.UUID) ([]Ticket, error) {
	return s.repository.ListTickets(ctx, userID)
}
func (s *Service) GetTicket(ctx context.Context, userID, id uuid.UUID) (Ticket, error) {
	return s.repository.GetTicket(ctx, userID, id)
}

func (s *Service) CreateTicket(ctx context.Context, userID uuid.UUID, subject, priority, message string) (Ticket, error) {
	subject, message, priority = strings.TrimSpace(subject), strings.TrimSpace(message), strings.ToLower(strings.TrimSpace(priority))
	if len(subject) < 3 || len(subject) > 255 || len(message) < 1 || len(message) > 10_000 || (priority != "low" && priority != "normal" && priority != "high") {
		return Ticket{}, ErrInvalidRequest
	}
	return s.repository.CreateTicket(ctx, userID, subject, priority, message)
}

func (s *Service) AddTicketMessage(ctx context.Context, userID, ticketID uuid.UUID, message string) (TicketMessage, error) {
	message = strings.TrimSpace(message)
	if len(message) < 1 || len(message) > 10_000 {
		return TicketMessage{}, ErrInvalidRequest
	}
	return s.repository.AddTicketMessage(ctx, userID, ticketID, message)
}
