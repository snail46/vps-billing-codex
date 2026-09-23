package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrPlanNotFound       = errors.New("plan not found")
	ErrInvalidQuantity    = errors.New("quantity is invalid")
	ErrInvalidIdempotency = errors.New("idempotency key is invalid")
	ErrPaymentNotFound    = errors.New("payment not found")
	ErrPaymentMismatch    = errors.New("payment amount or currency does not match")
	ErrPaymentState       = errors.New("payment state does not allow completion")
)

type CatalogItem struct {
	ProductID      uuid.UUID       `json:"product_id"`
	ProductSlug    string          `json:"product_slug"`
	ProductName    json.RawMessage `json:"product_name_i18n"`
	Description    json.RawMessage `json:"description_i18n"`
	PlanID         uuid.UUID       `json:"plan_id"`
	PlanSlug       string          `json:"plan_slug"`
	PlanName       json.RawMessage `json:"plan_name_i18n"`
	MemoryMB       int32           `json:"memory_mb"`
	DiskGB         int32           `json:"disk_gb"`
	TrafficGB      *int64          `json:"traffic_gb"`
	BandwidthMbps  *int32          `json:"bandwidth_mbps"`
	IPv4Count      int32           `json:"ipv4_count"`
	IPv6Count      int32           `json:"ipv6_count"`
	NATPortCount   int32           `json:"nat_port_count"`
	Virtualization string          `json:"virtualization"`
	BillingCycle   string          `json:"billing_cycle"`
	PriceMinor     int64           `json:"price_minor"`
	Currency       string          `json:"currency"`
}

type Order struct {
	ID            uuid.UUID  `json:"id"`
	OrderNo       string     `json:"order_no"`
	Status        string     `json:"status"`
	TotalMinor    int64      `json:"total_minor"`
	Currency      string     `json:"currency"`
	PaymentID     *uuid.UUID `json:"payment_id,omitempty"`
	PaymentStatus string     `json:"payment_status,omitempty"`
}

type Invoice struct {
	ID          uuid.UUID `json:"id"`
	InvoiceNo   string    `json:"invoice_no"`
	Status      string    `json:"status"`
	AmountMinor int64     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	DueAt       time.Time `json:"due_at"`
}
type Wallet struct {
	ID                    uuid.UUID `json:"id"`
	Currency              string    `json:"currency"`
	AvailableBalanceMinor int64     `json:"available_balance_minor"`
}

type Webhook struct {
	EventID           string    `json:"event_id"`
	PaymentID         uuid.UUID `json:"payment_id"`
	ExternalPaymentID string    `json:"external_payment_id"`
	Status            string    `json:"status"`
	AmountMinor       int64     `json:"amount_minor"`
	Currency          string    `json:"currency"`
}

type PaymentResult struct {
	PaymentID uuid.UUID `json:"payment_id"`
	OrderID   uuid.UUID `json:"order_id"`
	Duplicate bool      `json:"duplicate"`
}

type Repository interface {
	ListCatalog(context.Context) ([]CatalogItem, error)
	CreateOrder(context.Context, uuid.UUID, uuid.UUID, int32, string) (Order, error)
	ListOrders(context.Context, uuid.UUID) ([]Order, error)
	ListInvoices(context.Context, uuid.UUID) ([]Invoice, error)
	EnsureWallet(context.Context, uuid.UUID, string) (Wallet, error)
	CompletePayment(context.Context, Webhook, json.RawMessage) (PaymentResult, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) ListCatalog(ctx context.Context) ([]CatalogItem, error) {
	return s.repository.ListCatalog(ctx)
}
func (s *Service) CreateOrder(ctx context.Context, userID, planID uuid.UUID, quantity int32, key string) (Order, error) {
	if quantity < 1 || quantity > 100 {
		return Order{}, ErrInvalidQuantity
	}
	if len(key) < 8 || len(key) > 255 {
		return Order{}, ErrInvalidIdempotency
	}
	return s.repository.CreateOrder(ctx, userID, planID, quantity, key)
}
func (s *Service) ListOrders(ctx context.Context, userID uuid.UUID) ([]Order, error) {
	return s.repository.ListOrders(ctx, userID)
}
func (s *Service) ListInvoices(ctx context.Context, userID uuid.UUID) ([]Invoice, error) {
	return s.repository.ListInvoices(ctx, userID)
}
func (s *Service) Wallet(ctx context.Context, userID uuid.UUID, currency string) (Wallet, error) {
	return s.repository.EnsureWallet(ctx, userID, currency)
}
func (s *Service) CompletePayment(ctx context.Context, event Webhook, payload json.RawMessage) (PaymentResult, error) {
	if event.EventID == "" || event.ExternalPaymentID == "" || event.Status != "succeeded" || event.AmountMinor < 0 || len(event.Currency) != 3 {
		return PaymentResult{}, ErrPaymentMismatch
	}
	return s.repository.CompletePayment(ctx, event, payload)
}
