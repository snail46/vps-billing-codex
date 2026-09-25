package operation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"vps-billing/backend/internal/migrations"
	"vps-billing/backend/internal/outbox"
)

func TestOperationQueueRetryAndRecovery(t *testing.T) {
	databaseURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL and TEST_REDIS_URL are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := migrations.Run(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(options)
	defer func() { _ = redisClient.Close() }()
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	userID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,email,password_hash,status,locale,timezone) VALUES ($1,$2,'test','active','en-US','UTC')`, userID, "operation-"+userID.String()+"@example.com"); err != nil {
		t.Fatal(err)
	}
	repository := NewPostgresRepository(pool)
	service := NewService(repository)
	deadline := time.Now().UTC().Add(10 * time.Minute)
	request := CreateRequest{Type: "test.retry", ResourceType: "instance", ResourceID: uuid.New(), IdempotencyKey: "operation-test-" + uuid.NewString(), TraceID: uuid.NewString(), UserID: &userID, MaxRetries: 2, Steps: []StepDefinition{{Key: "allocate", Order: 1}}, DeadlineAt: &deadline}

	var wait sync.WaitGroup
	results := make(chan Operation, 2)
	errorsChannel := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			created, createErr := service.Create(ctx, request)
			results <- created
			errorsChannel <- createErr
		}()
	}
	wait.Wait()
	close(results)
	close(errorsChannel)
	for createErr := range errorsChannel {
		if createErr != nil {
			t.Fatalf("Create() error = %v", createErr)
		}
	}
	var operationID uuid.UUID
	for created := range results {
		if operationID == uuid.Nil {
			operationID = created.ID
		} else if created.ID != operationID {
			t.Fatalf("idempotent creates returned %s and %s", operationID, created.ID)
		}
	}
	replay := request
	replayDeadline := deadline.Add(time.Second)
	replay.DeadlineAt = &replayDeadline
	replayed, err := service.Create(ctx, replay)
	if err != nil || replayed.ID != operationID {
		t.Fatalf("server deadline changed idempotent result: id=%s error=%v", replayed.ID, err)
	}

	registry := NewWorkflowRegistry()
	var attempts atomic.Int32
	if err := registry.Register("test.retry", WorkflowFunc(func(workflowContext context.Context, execution *Execution) error {
		attempt := attempts.Add(1)
		if err := execution.Step(workflowContext, "allocate", "running", 25, "", nil, map[string]any{}); err != nil {
			return err
		}
		if attempt == 1 {
			return &WorkflowError{Code: "PROVIDER_TEMPORARY", MessageKey: "operation.retrying", Retryable: true, Cause: errors.New("private provider diagnostic")}
		}
		return execution.Step(workflowContext, "allocate", "succeeded", 100, "", nil, map[string]any{"safe": true})
	})); err != nil {
		t.Fatal(err)
	}
	dispatcher := outbox.NewDispatcher(pool, redisClient)
	consumer := NewQueueConsumer(repository, redisClient, registry, "integration-worker")
	if _, err := dispatcher.ProcessBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if processed, err := consumer.ProcessBatch(ctx); err != nil || processed != 1 {
		t.Fatalf("first ProcessBatch() = %d, %v", processed, err)
	}
	retrying, err := service.GetForUser(ctx, userID, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if retrying.Status != "retrying" || retrying.RetryCount != 1 || retrying.ErrorCode == nil || *retrying.ErrorCode != "PROVIDER_TEMPORARY" {
		var rawCode, rawMessage *string
		if queryErr := pool.QueryRow(ctx, `SELECT error_code,error_message FROM operations WHERE id=$1`, operationID).Scan(&rawCode, &rawMessage); queryErr != nil {
			t.Fatal(queryErr)
		}
		t.Fatalf("unexpected retry state: %+v; raw_code=%v raw_message=%v", retrying, stringValue(rawCode), stringValue(rawMessage))
	}
	if _, err := pool.Exec(ctx, `UPDATE operations SET next_attempt_at=now() WHERE id=$1`, operationID); err != nil {
		t.Fatal(err)
	}
	if processed, err := NewRetryScheduler(repository).ProcessBatch(ctx); err != nil || processed != 1 {
		t.Fatalf("retry scheduler = %d, %v", processed, err)
	}
	if _, err := dispatcher.ProcessBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if processed, err := consumer.ProcessBatch(ctx); err != nil || processed != 1 {
		t.Fatalf("second ProcessBatch() = %d, %v", processed, err)
	}
	completed, err := service.GetForUser(ctx, userID, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "succeeded" || completed.Progress != 100 || len(completed.Steps) != 1 || completed.Steps[0].Status != "succeeded" {
		t.Fatalf("unexpected completed state: %+v", completed)
	}
	if _, err := service.GetForUser(ctx, uuid.New(), operationID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign user GetForUser() error = %v", err)
	}
}

func TestInstanceAllowsOnlyOneActiveAction(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := migrations.Run(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	service := NewService(NewPostgresRepository(pool))
	instanceID := uuid.New()
	base := CreateRequest{Type: "restart", ResourceType: "instance", ResourceID: instanceID, IdempotencyKey: "restart:" + uuid.NewString(), TraceID: uuid.NewString(), MaxRetries: 3, Steps: []StepDefinition{{Key: "validate", Order: 1}}}
	first, err := service.Create(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	conflict := base
	conflict.Type = "stop"
	conflict.IdempotencyKey = "stop:" + uuid.NewString()
	if _, err := service.Create(ctx, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("concurrent action error=%v", err)
	}
	if err := NewPostgresRepository(pool).Succeed(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, conflict); err != nil {
		t.Fatalf("action after completion error=%v", err)
	}
}

func TestQueueReclaimsMessageAfterWorkerCrash(t *testing.T) {
	databaseURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL and TEST_REDIS_URL are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := migrations.Run(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	options.DB = 2
	redisClient := redis.NewClient(options)
	defer func() { _ = redisClient.Close() }()
	if err = redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	repository := NewPostgresRepository(pool)
	created, err := NewService(repository).Create(ctx, CreateRequest{Type: "test.crash", ResourceType: "instance", ResourceID: uuid.New(), IdempotencyKey: "crash-" + uuid.NewString(), TraceID: uuid.NewString(), MaxRetries: 2, Steps: []StepDefinition{{Key: "recover", Order: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"data": map[string]any{"operation_id": created.ID}})
	if _, err = redisClient.XAdd(ctx, &redis.XAddArgs{Stream: operationQueue, Values: map[string]any{"event_type": "operation.queued.v1", "payload": string(payload)}}).Result(); err != nil {
		t.Fatal(err)
	}
	if err = redisClient.XGroupCreateMkStream(ctx, operationQueue, consumerGroup, "0").Err(); err != nil && !stringsContains(err.Error(), "BUSYGROUP") {
		t.Fatal(err)
	}
	streams, err := redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{Group: consumerGroup, Consumer: "crashed-worker", Streams: []string{operationQueue, ">"}, Count: 1}).Result()
	if err != nil || len(streams) != 1 || len(streams[0].Messages) != 1 {
		t.Fatalf("crashed worker read = %#v, %v", streams, err)
	}
	registry := NewWorkflowRegistry()
	if err = registry.Register("test.crash", WorkflowFunc(func(workflowContext context.Context, execution *Execution) error {
		return execution.Step(workflowContext, "recover", "succeeded", 100, "", nil, map[string]any{"recovered": true})
	})); err != nil {
		t.Fatal(err)
	}
	consumer := NewQueueConsumer(repository, redisClient, registry, "replacement-worker")
	consumer.pendingIdle = 0
	if processed, processErr := consumer.ProcessBatch(ctx); processErr != nil || processed != 1 {
		t.Fatalf("replacement ProcessBatch() = %d, %v", processed, processErr)
	}
	current, err := repository.Get(ctx, created.ID)
	if err != nil || current.Status != "succeeded" {
		t.Fatalf("recovered operation = %s, %v", current.Status, err)
	}
}

func TestOutboxSurvivesRedisConnectionFailure(t *testing.T) {
	databaseURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL and TEST_REDIS_URL are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := migrations.Run(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	created, err := NewService(NewPostgresRepository(pool)).Create(ctx, CreateRequest{Type: "test.redis", ResourceType: "instance", ResourceID: uuid.New(), IdempotencyKey: "redis-" + uuid.NewString(), TraceID: uuid.NewString(), MaxRetries: 1, Steps: []StepDefinition{{Key: "test", Order: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	badRedis := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 20 * time.Millisecond, ReadTimeout: 20 * time.Millisecond, WriteTimeout: 20 * time.Millisecond, MaxRetries: 0})
	if _, err = outbox.NewDispatcher(pool, badRedis).ProcessBatch(ctx); err == nil {
		t.Fatal("dispatcher unexpectedly published while Redis was unavailable")
	}
	_ = badRedis.Close()
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM outbox_events WHERE aggregate_id=$1 AND event_type='operation.queued.v1'`, created.ID).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("outbox after outage = %q, %v", status, err)
	}
	options, _ := redis.ParseURL(redisURL)
	options.DB = 3
	goodRedis := redis.NewClient(options)
	defer func() { _ = goodRedis.Close() }()
	if err = goodRedis.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE outbox_events SET next_attempt_at=now() WHERE aggregate_id=$1`, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = outbox.NewDispatcher(pool, goodRedis).ProcessBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if length, lengthErr := goodRedis.XLen(ctx, operationQueue).Result(); lengthErr != nil || length < 1 {
		t.Fatalf("recovered queue length = %d, %v", length, lengthErr)
	}
}

func stringValue(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}
