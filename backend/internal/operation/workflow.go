package operation

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/google/uuid"
)

type Workflow interface {
	Execute(context.Context, *Execution) error
}

type WorkflowFunc func(context.Context, *Execution) error

func (f WorkflowFunc) Execute(ctx context.Context, execution *Execution) error {
	return f(ctx, execution)
}

type WorkflowRegistry struct {
	mu       sync.RWMutex
	handlers map[string]Workflow
}

func NewWorkflowRegistry() *WorkflowRegistry {
	return &WorkflowRegistry{handlers: make(map[string]Workflow)}
}

func (r *WorkflowRegistry) Register(operationType string, workflow Workflow) error {
	if operationType == "" || workflow == nil {
		return ErrWorkflowMissing
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.handlers[operationType]; exists {
		return errors.New("workflow already registered")
	}
	r.handlers[operationType] = workflow
	return nil
}

func (r *WorkflowRegistry) Get(operationType string) (Workflow, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	workflow, ok := r.handlers[operationType]
	return workflow, ok
}

type Execution struct {
	operationID uuid.UUID
	attempt     int32
	repository  *PostgresRepository
}

func (e *Execution) OperationID() uuid.UUID { return e.operationID }
func (e *Execution) Attempt() int32         { return e.attempt }
func (e *Execution) Progress(ctx context.Context, status, phase string, progress int32, messageKey string) error {
	return e.repository.UpdateProgress(ctx, e.operationID, status, phase, progress, messageKey)
}
func (e *Execution) Step(ctx context.Context, key, status string, progress int32, errorCode string, err error, output any) error {
	rawError := ""
	if err != nil {
		rawError = err.Error()
	}
	rawOutput, marshalErr := json.Marshal(output)
	if marshalErr != nil {
		return marshalErr
	}
	return e.repository.UpdateStep(ctx, e.operationID, key, status, progress, e.attempt, errorCode, rawError, rawOutput)
}
