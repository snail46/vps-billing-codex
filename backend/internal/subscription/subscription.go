package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound     = errors.New("subscription not found")
	ErrInvalidState = errors.New("subscription state does not allow this action")
)

type Subscription struct {
	ID                 uuid.UUID       `json:"id"`
	PlanID             uuid.UUID       `json:"plan_id"`
	PlanSlug           string          `json:"plan_slug"`
	PlanName           json.RawMessage `json:"plan_name_i18n"`
	Status             string          `json:"status"`
	BillingCycle       string          `json:"billing_cycle"`
	PriceMinor         int64           `json:"price_minor"`
	Currency           string          `json:"currency"`
	StartedAt          *time.Time      `json:"started_at"`
	CurrentPeriodStart *time.Time      `json:"current_period_start"`
	CurrentPeriodEnd   *time.Time      `json:"current_period_end"`
	NextDueAt          *time.Time      `json:"next_due_at"`
	GraceUntil         *time.Time      `json:"grace_until"`
	CancelAtPeriodEnd  bool            `json:"cancel_at_period_end"`
	EndedAt            *time.Time      `json:"ended_at"`
	Version            int64           `json:"version"`
}

type Repository interface {
	List(context.Context, uuid.UUID) ([]Subscription, error)
	SetCancelAtPeriodEnd(context.Context, uuid.UUID, uuid.UUID, bool) (Subscription, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]Subscription, error) {
	return s.repository.List(ctx, userID)
}

func (s *Service) SetCancelAtPeriodEnd(ctx context.Context, userID, subscriptionID uuid.UUID, cancel bool) (Subscription, error) {
	return s.repository.SetCancelAtPeriodEnd(ctx, userID, subscriptionID, cancel)
}
