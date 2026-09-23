package operation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	db "vps-billing/backend/internal/store/sqlc"
)

const (
	operationQueue = "operation-queue"
	consumerGroup  = "operation-workers"
)

type QueueConsumer struct {
	repository *PostgresRepository
	redis      *redis.Client
	registry   *WorkflowRegistry
	consumer   string
}

func NewQueueConsumer(repository *PostgresRepository, redisClient *redis.Client, registry *WorkflowRegistry, consumer string) *QueueConsumer {
	return &QueueConsumer{repository: repository, redis: redisClient, registry: registry, consumer: consumer}
}

func (c *QueueConsumer) ProcessBatch(ctx context.Context) (int, error) {
	if err := c.redis.XGroupCreateMkStream(ctx, operationQueue, consumerGroup, "0").Err(); err != nil && !stringsContains(err.Error(), "BUSYGROUP") {
		return 0, err
	}
	streams, err := c.redis.XReadGroup(ctx, &redis.XReadGroupArgs{Group: consumerGroup, Consumer: c.consumer, Streams: []string{operationQueue, ">"}, Count: 10, Block: 10 * time.Millisecond}).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, stream := range streams {
		for _, message := range stream.Messages {
			if err := c.processMessage(ctx, message); err != nil {
				return processed, err
			}
			if err := c.redis.XAck(ctx, operationQueue, consumerGroup, message.ID).Err(); err != nil {
				return processed, err
			}
			processed++
		}
	}
	return processed, nil
}

func (c *QueueConsumer) processMessage(ctx context.Context, message redis.XMessage) error {
	payloadValue, ok := message.Values["payload"]
	if !ok {
		return fmt.Errorf("operation queue message %s has no payload", message.ID)
	}
	var envelope struct {
		Data struct {
			OperationID uuid.UUID `json:"operation_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(fmt.Sprint(payloadValue)), &envelope); err != nil {
		return err
	}
	if envelope.Data.OperationID == uuid.Nil {
		return ErrInvalidRequest
	}
	current, err := c.repository.Claim(ctx, envelope.Data.OperationID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	workflow, ok := c.registry.Get(current.Type)
	if !ok {
		return c.repository.Fail(ctx, current.ID, &WorkflowError{Code: "WORKFLOW_NOT_REGISTERED", MessageKey: "operation.errors.workflowMissing", Cause: ErrWorkflowMissing})
	}
	execution := &Execution{operationID: current.ID, attempt: current.RetryCount + 1, repository: c.repository}
	executeErr := workflow.Execute(ctx, execution)
	if executeErr == nil {
		return c.repository.Succeed(ctx, current.ID)
	}
	var workflowErr *WorkflowError
	if !errors.As(executeErr, &workflowErr) {
		workflowErr = &WorkflowError{Code: "WORKFLOW_FAILED", MessageKey: "operation.failed", Cause: executeErr}
	}
	if workflowErr.Retryable && current.RetryCount < current.MaxRetries {
		backoff := time.Duration(1<<min(current.RetryCount, 8)) * time.Second
		return c.repository.ScheduleRetry(ctx, current, workflowErr, time.Now().UTC().Add(backoff))
	}
	return c.repository.Fail(ctx, current.ID, workflowErr)
}

type RetryScheduler struct {
	repository *PostgresRepository
	batchSize  int32
}

func NewRetryScheduler(repository *PostgresRepository) *RetryScheduler {
	return &RetryScheduler{repository: repository, batchSize: 50}
}

func (s *RetryScheduler) ProcessBatch(ctx context.Context) (int, error) {
	tx, err := s.repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	rows, err := queries.ClaimDueOperationRetries(ctx, s.batchSize)
	if err != nil {
		return 0, err
	}
	for _, current := range rows {
		updated, updateErr := queries.RequeueOperation(ctx, current.ID)
		if updateErr != nil {
			return 0, updateErr
		}
		if eventErr := createEvent(ctx, queries, "operation.queued.v1", updated, nil); eventErr != nil {
			return 0, eventErr
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func stringsContains(value, target string) bool {
	for index := 0; index+len(target) <= len(value); index++ {
		if value[index:index+len(target)] == target {
			return true
		}
	}
	return false
}
