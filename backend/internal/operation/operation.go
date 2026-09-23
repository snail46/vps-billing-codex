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
	return s.repository.Create(ctx, request)
}
func (s *Service) GetForUser(ctx context.Context, userID, operationID uuid.UUID) (Operation, error) {
	return s.repository.GetForUser(ctx, userID, operationID)
}
