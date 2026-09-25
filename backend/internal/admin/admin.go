package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrNotFound        = errors.New("resource not found")
	ErrInvalidInput    = errors.New("invalid input")
	ErrInsufficient    = errors.New("insufficient wallet balance")
	ErrTicketFinalized = errors.New("ticket is finalized")
	ErrRefundInvalid   = errors.New("refund is invalid")
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

type ProductInput struct {
	Slug        string
	NameI18n    map[string]string
	Description map[string]string
	Status      string
	SortOrder   int32
	ProductType string
	Featured    bool
}

type PlanInput struct {
	Slug           string
	NameI18n       map[string]string
	Status         string
	NodeGroupID    *uuid.UUID
	CPUCores       float64
	MemoryMB       int32
	DiskGB         int32
	TrafficGB      *int64
	BandwidthMbps  *int32
	IPv4Count      int32
	IPv6Count      int32
	NATPortCount   int32
	Virtualization string
	BillingCycle   string
	PriceMinor     int64
	Currency       string
	StockMode      string
	DefaultImageID string
	StockQuantity  *int32
	SetupFeeMinor  int64
	OverageMinor   int64
}

type Repository interface {
	Dashboard(context.Context) (Dashboard, error)
	List(context.Context, string) ([]Item, error)
	Instance(context.Context, uuid.UUID, bool) (Item, error)
	Operation(context.Context, uuid.UUID, bool) (Item, error)
	Provider(context.Context, uuid.UUID, bool) (Item, error)
	UpdateUserStatus(context.Context, uuid.UUID, string, AuditContext) (Item, error)
	AdjustWallet(context.Context, uuid.UUID, string, int64, string, AuditContext) (Item, error)
	ReplyTicket(context.Context, uuid.UUID, string, AuditContext) (Item, error)
	UpdateTicketStatus(context.Context, uuid.UUID, string, AuditContext) (Item, error)
	UpdateSetting(context.Context, string, any, bool, AuditContext) (Item, error)
	SetAdminRoles(context.Context, uuid.UUID, []string, AuditContext) (Item, error)
	CreateProduct(context.Context, ProductInput, AuditContext) (Item, error)
	UpdateProduct(context.Context, uuid.UUID, ProductInput, AuditContext) (Item, error)
	CreatePlan(context.Context, uuid.UUID, PlanInput, AuditContext) (Item, error)
	UpdatePlan(context.Context, uuid.UUID, PlanInput, AuditContext) (Item, error)
	ReplayOutbox(context.Context, uuid.UUID, AuditContext) (Item, error)
	RefundPayment(context.Context, uuid.UUID, int64, string, string, AuditContext) (Item, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) Dashboard(ctx context.Context) (Dashboard, error) {
	return s.repository.Dashboard(ctx)
}
func (s *Service) List(ctx context.Context, resource string) ([]Item, error) {
	return s.repository.List(ctx, resource)
}
func (s *Service) ListFiltered(ctx context.Context, resource, query, status string, limit, offset int) ([]Item, int, error) {
	items, err := s.repository.List(ctx, resource)
	if err != nil {
		return nil, 0, err
	}
	query, status = strings.ToLower(strings.TrimSpace(query)), strings.ToLower(strings.TrimSpace(status))
	filtered := make([]Item, 0, len(items))
	for _, item := range items {
		if status != "" && strings.ToLower(fmt.Sprint(item["status"])) != status {
			continue
		}
		if query != "" {
			raw, _ := json.Marshal(item)
			if !strings.Contains(strings.ToLower(string(raw)), query) {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := min(offset+limit, total)
	return filtered[offset:end], total, nil
}
func (s *Service) Instance(ctx context.Context, id uuid.UUID, diagnostics bool) (Item, error) {
	return s.repository.Instance(ctx, id, diagnostics)
}
func (s *Service) Operation(ctx context.Context, id uuid.UUID, raw bool) (Item, error) {
	return s.repository.Operation(ctx, id, raw)
}
func (s *Service) Provider(ctx context.Context, id uuid.UUID, diagnostics bool) (Item, error) {
	return s.repository.Provider(ctx, id, diagnostics)
}
func (s *Service) ReplayOutbox(ctx context.Context, id uuid.UUID, audit AuditContext) (Item, error) {
	return s.repository.ReplayOutbox(ctx, id, audit)
}
func (s *Service) RefundPayment(ctx context.Context, id uuid.UUID, amount int64, reason, key string, audit AuditContext) (Item, error) {
	if amount <= 0 || strings.TrimSpace(reason) == "" || len(key) < 8 || len(key) > 255 {
		return nil, ErrInvalidInput
	}
	return s.repository.RefundPayment(ctx, id, amount, strings.TrimSpace(reason), key, audit)
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

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (s *Service) CreateProduct(ctx context.Context, input ProductInput, audit AuditContext) (Item, error) {
	input = normalizeProduct(input)
	if !validProduct(input) {
		return nil, ErrInvalidInput
	}
	return s.repository.CreateProduct(ctx, input, audit)
}

func (s *Service) UpdateProduct(ctx context.Context, id uuid.UUID, input ProductInput, audit AuditContext) (Item, error) {
	input = normalizeProduct(input)
	if id == uuid.Nil || !validProduct(input) {
		return nil, ErrInvalidInput
	}
	return s.repository.UpdateProduct(ctx, id, input, audit)
}

func (s *Service) CreatePlan(ctx context.Context, productID uuid.UUID, input PlanInput, audit AuditContext) (Item, error) {
	input = normalizePlan(input)
	if productID == uuid.Nil || !validPlan(input) {
		return nil, ErrInvalidInput
	}
	return s.repository.CreatePlan(ctx, productID, input, audit)
}

func (s *Service) UpdatePlan(ctx context.Context, id uuid.UUID, input PlanInput, audit AuditContext) (Item, error) {
	input = normalizePlan(input)
	if id == uuid.Nil || !validPlan(input) {
		return nil, ErrInvalidInput
	}
	return s.repository.UpdatePlan(ctx, id, input, audit)
}

func normalizeProduct(input ProductInput) ProductInput {
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.ProductType = strings.ToLower(strings.TrimSpace(input.ProductType))
	for key, value := range input.NameI18n {
		input.NameI18n[key] = strings.TrimSpace(value)
	}
	for key, value := range input.Description {
		input.Description[key] = strings.TrimSpace(value)
	}
	return input
}

func validProduct(input ProductInput) bool {
	return slugPattern.MatchString(input.Slug) && len(input.Slug) <= 255 &&
		(input.Status == "draft" || input.Status == "active" || input.Status == "disabled" || input.Status == "archived") &&
		input.NameI18n["zh-CN"] != "" && input.NameI18n["en-US"] != "" &&
		(input.ProductType == "vps" || input.ProductType == "nat_vps")
}

func normalizePlan(input PlanInput) PlanInput {
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.Virtualization = strings.ToLower(strings.TrimSpace(input.Virtualization))
	input.BillingCycle = strings.ToLower(strings.TrimSpace(input.BillingCycle))
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	input.StockMode = strings.ToLower(strings.TrimSpace(input.StockMode))
	input.DefaultImageID = strings.TrimSpace(input.DefaultImageID)
	for key, value := range input.NameI18n {
		input.NameI18n[key] = strings.TrimSpace(value)
	}
	return input
}

func validPlan(input PlanInput) bool {
	validCycle := input.BillingCycle == "monthly" || input.BillingCycle == "quarterly" || input.BillingCycle == "yearly"
	validStatus := input.Status == "draft" || input.Status == "active" || input.Status == "disabled" || input.Status == "archived"
	return slugPattern.MatchString(input.Slug) && len(input.Slug) <= 255 && validStatus && validCycle &&
		input.NameI18n["zh-CN"] != "" && input.NameI18n["en-US"] != "" && input.CPUCores > 0 &&
		input.MemoryMB > 0 && input.DiskGB > 0 && input.PriceMinor >= 0 && len(input.Currency) == 3 &&
		input.IPv4Count >= 0 && input.IPv6Count >= 0 && input.NATPortCount >= 0 && input.Virtualization != "" &&
		(input.StockMode == "automatic" || input.StockMode == "manual") && input.DefaultImageID != "" &&
		(input.StockQuantity == nil || *input.StockQuantity >= 0) && input.SetupFeeMinor >= 0 && input.OverageMinor >= 0
}
