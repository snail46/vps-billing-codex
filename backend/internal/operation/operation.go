package operation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound            = errors.New("operation not found")
	ErrInvalidRequest      = errors.New("operation request is invalid")
	ErrIdempotencyConflict = errors.New("operation idempotency key was used for another request")
	ErrWorkflowMissing     = errors.New("operation workflow is not registered")
	ErrStateConflict       = errors.New("operation state does not allow this action")
)

type Operation struct {
	ID           uuid.UUID  `json:"id"`
	Type         string     `json:"type"`
	ResourceType string     `json:"resource_type"`
	ResourceID   uuid.UUID  `json:"resource_id"`
	Status       string     `json:"status"`
	Phase        *string    `json:"phase"`
	Progress     int32      `json:"progress"`
	MessageKey   *string    `json:"message_key"`
	Retryable    bool       `json:"retryable"`
	RetryCount   int32      `json:"retry_count"`
	MaxRetries   int32      `json:"max_retries"`
	ErrorCode    *string    `json:"error_code"`
	TraceID      string     `json:"trace_id"`
	StartedAt    *time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Steps        []Step     `json:"steps"`
	Attempts     []Attempt  `json:"attempts"`
}

type Attempt struct {
	Attempt    int32      `json:"attempt"`
	Status     string     `json:"status"`
	ErrorCode  *string    `json:"error_code"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type Step struct {
	Key        string          `json:"key"`
	Order      int32           `json:"order"`
	Status     string          `json:"status"`
	Progress   int32           `json:"progress"`
	Attempt    int32           `json:"attempt"`
	ErrorCode  *string         `json:"error_code"`
	Output     json.RawMessage `json:"output,omitempty"`
	StartedAt  *time.Time      `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at"`
}

type StepDefinition struct {
	Key   string
	Order int32
}

type CreateRequest struct {
	Type           string
	ResourceType   string
	ResourceID     uuid.UUID
	IdempotencyKey string
	TraceID        string
	UserID         *uuid.UUID
	ActorAdminID   *uuid.UUID
	MaxRetries     int32
	Steps          []StepDefinition
	Input          json.RawMessage
	DeadlineAt     *time.Time
	ParentID       *uuid.UUID
}

type WorkflowError struct {
	Code       string
	MessageKey string
	Retryable  bool
	Cause      error
}

func (e *WorkflowError) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Cause.Error()
	}
	return e.Code
}

func (e *WorkflowError) Unwrap() error { return e.Cause }

type Repository interface {
	Create(context.Context, CreateRequest) (Operation, error)
	GetForUser(context.Context, uuid.UUID, uuid.UUID) (Operation, error)
	CreateAdminRetry(context.Context, uuid.UUID, uuid.UUID, string, string) (Operation, error)
	CancelAdmin(context.Context, uuid.UUID, uuid.UUID, string) (Operation, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Create(ctx context.Context, request CreateRequest) (Operation, error) {
	if request.Type == "" || request.ResourceType == "" || request.ResourceID == uuid.Nil || len(request.IdempotencyKey) < 8 || request.TraceID == "" || request.MaxRetries < 0 || len(request.Steps) == 0 {
		return Operation{}, ErrInvalidRequest
	}
	if len(request.Input) > 0 && !json.Valid(request.Input) {
		return Operation{}, ErrInvalidRequest
	}
	if request.DeadlineAt != nil && !request.DeadlineAt.After(time.Now()) {
		return Operation{}, ErrInvalidRequest
	}
	return s.repository.Create(ctx, request)
}
func (s *Service) GetForUser(ctx context.Context, userID, operationID uuid.UUID) (Operation, error) {
	return s.repository.GetForUser(ctx, userID, operationID)
}
func (s *Service) RetryAdmin(ctx context.Context, operationID, adminID uuid.UUID, idempotencyKey, traceID string) (Operation, error) {
	if operationID == uuid.Nil || adminID == uuid.Nil || len(idempotencyKey) < 8 || traceID == "" {
		return Operation{}, ErrInvalidRequest
	}
	return s.repository.CreateAdminRetry(ctx, operationID, adminID, idempotencyKey, traceID)
}
func (s *Service) CancelAdmin(ctx context.Context, operationID, adminID uuid.UUID, traceID string) (Operation, error) {
	if operationID == uuid.Nil || adminID == uuid.Nil || traceID == "" {
		return Operation{}, ErrInvalidRequest
	}
	return s.repository.CancelAdmin(ctx, operationID, adminID, traceID)
}
