package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrPlanNotFound         = errors.New("plan not found")
	ErrInvalidQuantity      = errors.New("quantity is invalid")
	ErrInvalidIdempotency   = errors.New("idempotency key is invalid")
	ErrIdempotencyConflict  = errors.New("idempotency key was used for a different request")
	ErrPaymentNotFound      = errors.New("payment not found")
	ErrPaymentMismatch      = errors.New("payment amount or currency does not match")
	ErrPaymentState         = errors.New("payment state does not allow completion")
	ErrSubscriptionNotFound = errors.New("subscription not found")
	ErrSubscriptionState    = errors.New("subscription state does not allow renewal")
	ErrPromotionInvalid     = errors.New("promotion is invalid")
	ErrBillingProfile       = errors.New("billing profile is invalid")
)

type CatalogItem struct {
	ProductID      uuid.UUID       `json:"product_id"`
	ProductSlug    string          `json:"product_slug"`
	ProductName    json.RawMessage `json:"product_name_i18n"`
	Description    json.RawMessage `json:"description_i18n"`
	ProductType    string          `json:"product_type"`
	Featured       bool            `json:"featured"`
	PlanID         uuid.UUID       `json:"plan_id"`
	PlanSlug       string          `json:"plan_slug"`
	PlanName       json.RawMessage `json:"plan_name_i18n"`
	CPUCores       float64         `json:"cpu_cores"`
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
	StockMode      string          `json:"stock_mode"`
	StockQuantity  *int32          `json:"stock_quantity"`
	SetupFeeMinor  int64           `json:"setup_fee_minor"`
	OverageMinor   int64           `json:"traffic_overage_price_minor"`
	Region         string          `json:"region"`
	Available      bool            `json:"available"`
	SharedIPv4     bool            `json:"shared_ipv4"`
	PortForward    bool            `json:"port_forward"`
	TrafficMeter   bool            `json:"traffic_meter"`
}

type Order struct {
	ID             uuid.UUID  `json:"id"`
	OrderNo        string     `json:"order_no"`
	Status         string     `json:"status"`
	TotalMinor     int64      `json:"total_minor"`
	Currency       string     `json:"currency"`
	Kind           string     `json:"kind"`
	PaymentID      *uuid.UUID `json:"payment_id,omitempty"`
	PaymentStatus  string     `json:"payment_status,omitempty"`
	SubscriptionID *uuid.UUID `json:"subscription_id,omitempty"`
}

type Invoice struct {
	ID             uuid.UUID  `json:"id"`
	InvoiceNo      string     `json:"invoice_no"`
	Status         string     `json:"status"`
	AmountMinor    int64      `json:"amount_minor"`
	Currency       string     `json:"currency"`
	DueAt          time.Time  `json:"due_at"`
	SubscriptionID *uuid.UUID `json:"subscription_id,omitempty"`
	OrderID        *uuid.UUID `json:"order_id,omitempty"`
}
type Wallet struct {
	ID                    uuid.UUID `json:"id"`
	Currency              string    `json:"currency"`
	AvailableBalanceMinor int64     `json:"available_balance_minor"`
}
type BillingProfile struct {
	UserID      uuid.UUID       `json:"user_id"`
	LegalName   string          `json:"legal_name"`
	TaxID       string          `json:"tax_id"`
	CountryCode string          `json:"country_code"`
	Address     json.RawMessage `json:"address"`
	UpdatedAt   time.Time       `json:"updated_at"`
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
	CreateOrderWithPromotion(context.Context, uuid.UUID, uuid.UUID, int32, string, string) (Order, error)
	CreateRenewalOrder(context.Context, uuid.UUID, uuid.UUID, string) (Order, error)
	ListOrders(context.Context, uuid.UUID) ([]Order, error)
	ListInvoices(context.Context, uuid.UUID) ([]Invoice, error)
	EnsureWallet(context.Context, uuid.UUID, string) (Wallet, error)
	CompletePayment(context.Context, Webhook, json.RawMessage) (PaymentResult, error)
	BillingProfile(context.Context, uuid.UUID) (BillingProfile, error)
	UpdateBillingProfile(context.Context, uuid.UUID, BillingProfile) (BillingProfile, error)
}

func (s *Service) CreateRenewalOrder(ctx context.Context, userID, subscriptionID uuid.UUID, key string) (Order, error) {
	if len(key) < 8 || len(key) > 255 {
		return Order{}, ErrInvalidIdempotency
	}
	return s.repository.CreateRenewalOrder(ctx, userID, subscriptionID, key)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) ListCatalog(ctx context.Context) ([]CatalogItem, error) {
	return s.repository.ListCatalog(ctx)
}
func (s *Service) CreateOrder(ctx context.Context, userID, planID uuid.UUID, quantity int32, key string) (Order, error) {
	return s.CreateOrderWithPromotion(ctx, userID, planID, quantity, key, "")
}
func (s *Service) CreateOrderWithPromotion(ctx context.Context, userID, planID uuid.UUID, quantity int32, key, promotionCode string) (Order, error) {
	if quantity < 1 || quantity > 100 {
		return Order{}, ErrInvalidQuantity
	}
	if len(key) < 8 || len(key) > 255 {
		return Order{}, ErrInvalidIdempotency
	}
	return s.repository.CreateOrderWithPromotion(ctx, userID, planID, quantity, key, strings.ToUpper(strings.TrimSpace(promotionCode)))
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
func (s *Service) BillingProfile(ctx context.Context, userID uuid.UUID) (BillingProfile, error) {
	return s.repository.BillingProfile(ctx, userID)
}
func (s *Service) UpdateBillingProfile(ctx context.Context, userID uuid.UUID, input BillingProfile) (BillingProfile, error) {
	input.LegalName = strings.TrimSpace(input.LegalName)
	input.TaxID = strings.TrimSpace(input.TaxID)
	input.CountryCode = strings.ToUpper(strings.TrimSpace(input.CountryCode))
	if len(input.LegalName) > 255 || len(input.TaxID) > 128 || (input.CountryCode != "" && len(input.CountryCode) != 2) || len(input.Address) > 8192 {
		return BillingProfile{}, ErrBillingProfile
	}
	return s.repository.UpdateBillingProfile(ctx, userID, input)
}
