package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrNotFound        = errors.New("resource not found")
	ErrInvalidInput    = errors.New("invalid input")
	ErrInsufficient    = errors.New("insufficient wallet balance")
	ErrTicketFinalized = errors.New("ticket is finalized")
)

type Item = map[string]any

type Dashboard struct {
	Metrics          Item   `json:"metrics"`
	CriticalAlerts   []Item `json:"critical_alerts"`
	FailedOperations []Item `json:"failed_operations"`
	ProviderHealth   []Item `json:"provider_health"`
	NodeHealth       []Item `json:"node_health"`
	CapacityWarnings []Item `json:"capacity_warnings"`
}

type AuditContext struct {
	AdminID   uuid.UUID
	IPAddress string
	UserAgent string
	RequestID string
	TraceID   string
}

type Repository interface {
	Dashboard(context.Context) (Dashboard, error)
	List(context.Context, string) ([]Item, error)
	Instance(context.Context, uuid.UUID, bool) (Item, error)
	Operation(context.Context, uuid.UUID, bool) (Item, error)
	UpdateUserStatus(context.Context, uuid.UUID, string, AuditContext) (Item, error)
	AdjustWallet(context.Context, uuid.UUID, string, int64, string, AuditContext) (Item, error)
	ReplyTicket(context.Context, uuid.UUID, string, AuditContext) (Item, error)
	UpdateTicketStatus(context.Context, uuid.UUID, string, AuditContext) (Item, error)
	UpdateSetting(context.Context, string, any, bool, AuditContext) (Item, error)
	SetAdminRoles(context.Context, uuid.UUID, []string, AuditContext) (Item, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) Dashboard(ctx context.Context) (Dashboard, error) {
	return s.repository.Dashboard(ctx)
}
func (s *Service) List(ctx context.Context, resource string) ([]Item, error) {
	return s.repository.List(ctx, resource)
}
func (s *Service) Instance(ctx context.Context, id uuid.UUID, diagnostics bool) (Item, error) {
	return s.repository.Instance(ctx, id, diagnostics)
}
func (s *Service) Operation(ctx context.Context, id uuid.UUID, raw bool) (Item, error) {
	return s.repository.Operation(ctx, id, raw)
}

func (s *Service) UpdateUserStatus(ctx context.Context, id uuid.UUID, status string, audit AuditContext) (Item, error) {
	if status != "active" && status != "suspended" {
		return nil, ErrInvalidInput
	}
	return s.repository.UpdateUserStatus(ctx, id, status, audit)
}
func (s *Service) AdjustWallet(ctx context.Context, userID uuid.UUID, currency string, amount int64, description string, audit AuditContext) (Item, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 || amount == 0 || strings.TrimSpace(description) == "" {
		return nil, ErrInvalidInput
	}
	return s.repository.AdjustWallet(ctx, userID, currency, amount, strings.TrimSpace(description), audit)
}
func (s *Service) ReplyTicket(ctx context.Context, id uuid.UUID, message string, audit AuditContext) (Item, error) {
	message = strings.TrimSpace(message)
	if message == "" || len(message) > 10000 {
		return nil, ErrInvalidInput
	}
	return s.repository.ReplyTicket(ctx, id, message, audit)
}
func (s *Service) UpdateTicketStatus(ctx context.Context, id uuid.UUID, status string, audit AuditContext) (Item, error) {
	if status != "open" && status != "waiting_user" && status != "waiting_support" && status != "resolved" && status != "closed" {
		return nil, ErrInvalidInput
	}
	return s.repository.UpdateTicketStatus(ctx, id, status, audit)
}
func (s *Service) UpdateSetting(ctx context.Context, key string, value any, secret bool, audit AuditContext) (Item, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 255 || value == nil {
		return nil, ErrInvalidInput
	}
	return s.repository.UpdateSetting(ctx, key, value, secret, audit)
}
func (s *Service) SetAdminRoles(ctx context.Context, id uuid.UUID, roles []string, audit AuditContext) (Item, error) {
	if len(roles) == 0 {
		return nil, ErrInvalidInput
	}
	for _, role := range roles {
		if strings.TrimSpace(role) == "" {
			return nil, ErrInvalidInput
		}
	}
	return s.repository.SetAdminRoles(ctx, id, roles, audit)
}
