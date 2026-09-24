package provision

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"vps-billing/backend/internal/commerce"
	"vps-billing/backend/internal/infrastructure"
	"vps-billing/backend/internal/migrations"
	"vps-billing/backend/internal/operation"
	providercontract "vps-billing/backend/internal/provider"
	providermock "vps-billing/backend/internal/provider/mock"
)

func TestPaidOrderProvisionsRunningInstance(t *testing.T) {
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
	options.DB = 1
	redisClient := redis.NewClient(options)
	defer func() { _ = redisClient.Close() }()
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	userID, productID, planID := uuid.New(), uuid.New(), uuid.New()
	providerID, groupID, nodeID := uuid.New(), uuid.New(), uuid.New()
	fixtures := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users (id,email,password_hash,status,locale,timezone) VALUES ($1,$2,'test','active','en-US','UTC')`, []any{userID, "provision-" + userID.String() + "@example.com"}},
		{`INSERT INTO node_groups (id,name,region,status) VALUES ($1,$2,'test-region','active')`, []any{groupID, "group-" + groupID.String()}},
		{`INSERT INTO providers (id,name,provider_type,status,capabilities) VALUES ($1,$2,'mock','active','{"create_instance":true}')`, []any{providerID, "provider-" + providerID.String()}},
		{`INSERT INTO nodes (id,provider_id,node_group_id,provider_node_id,name,region,status,cpu_total,memory_total_mb,disk_total_gb,ipv4_total,capabilities,last_seen_at) VALUES ($1,$2,$3,$4,$5,'test-region','online',8,16384,500,8,'{"virtualization":"kvm"}',now())`, []any{nodeID, providerID, groupID, "node-1", "node-" + nodeID.String()}},
		{`INSERT INTO products (id,slug,name_i18n,status) VALUES ($1,$2,'{"en-US":"Provision"}','active')`, []any{productID, "product-" + productID.String()}},
		{`INSERT INTO plans (id,product_id,node_group_id,slug,name_i18n,status,cpu_cores,memory_mb,disk_gb,ipv4_count,virtualization,billing_cycle,price_minor,currency,default_image_id) VALUES ($1,$2,$3,$4,'{"en-US":"Provision Plan"}','active',2,2048,30,1,'kvm','monthly',1299,'USD','ubuntu-24.04')`, []any{planID, productID, groupID, "plan-" + planID.String()}},
	}
	for _, fixture := range fixtures {
		if _, err := pool.Exec(ctx, fixture.query, fixture.args...); err != nil {
			t.Fatal(err)
		}
	}

	commerceRepository := commerce.NewPostgresRepository(pool)
	order, err := commerceRepository.CreateOrder(ctx, userID, planID, 1, "provision-order-"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	webhook := commerce.Webhook{EventID: "provision-event-" + order.ID.String(), PaymentID: *order.PaymentID, ExternalPaymentID: "external-" + order.ID.String(), Status: "succeeded", AmountMinor: 1299, Currency: "USD"}
	paymentPayload, _ := json.Marshal(webhook)
	if _, err := commerceRepository.CompletePayment(ctx, webhook, paymentPayload); err != nil {
		t.Fatal(err)
	}

	eventPayload, _ := json.Marshal(map[string]any{"data": map[string]any{"order_id": order.ID, "user_id": userID}})
	if _, err := redisClient.XAdd(ctx, &redis.XAddArgs{Stream: domainEventsStream, Values: map[string]any{"event_type": "payment.succeeded.v1", "payload": string(eventPayload)}}).Result(); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(pool)
	trigger := NewTriggerConsumer(repository, redisClient, "provision-integration")
	if processed, err := trigger.ProcessBatch(ctx); err != nil || processed != 1 {
		t.Fatalf("trigger ProcessBatch() = %d, %v", processed, err)
	}
	var operationID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM operations WHERE idempotency_key=$1`, "provision:"+order.ID.String()+":1").Scan(&operationID); err != nil {
		t.Fatal(err)
	}
	operationPayload, _ := json.Marshal(map[string]any{"data": map[string]any{"operation_id": operationID}})
	if _, err := redisClient.XAdd(ctx, &redis.XAddArgs{Stream: "operation-queue", Values: map[string]any{"event_type": "operation.queued.v1", "payload": string(operationPayload)}}).Result(); err != nil {
		t.Fatal(err)
	}
	providerRegistry := providercontract.NewDynamicRegistry(pool)
	if err := providerRegistry.RegisterFactory("mock", func(providercontract.FactoryConfig) (providercontract.Provider, error) {
		return providermock.New(), nil
	}); err != nil {
		t.Fatal(err)
	}
	workflowRegistry := operation.NewWorkflowRegistry()
	infrastructureRepository := infrastructure.NewPostgresRepository(pool)
	if err := workflowRegistry.Register("provision", NewWorkflow(repository, infrastructure.NewScheduler(infrastructureRepository), providerRegistry)); err != nil {
		t.Fatal(err)
	}
	consumer := operation.NewQueueConsumer(operation.NewPostgresRepository(pool), redisClient, workflowRegistry, "provision-integration")
	if processed, err := consumer.ProcessBatch(ctx); err != nil || processed != 1 {
		t.Fatalf("operation ProcessBatch() = %d, %v", processed, err)
	}

	var operationStatus, subscriptionStatus, instanceStatus, orderStatus, reservationStatus string
	var providerInstanceID string
	if err := pool.QueryRow(ctx, `SELECT o.status,s.status,i.observed_state,ord.status,r.status,i.provider_instance_id FROM operations o JOIN instances i ON i.id=o.resource_id JOIN subscriptions s ON s.id=i.subscription_id JOIN orders ord ON ord.id=s.source_order_id JOIN resource_reservations r ON r.operation_id=o.id WHERE o.id=$1`, operationID).Scan(&operationStatus, &subscriptionStatus, &instanceStatus, &orderStatus, &reservationStatus, &providerInstanceID); err != nil {
		t.Fatal(err)
	}
	if operationStatus != "succeeded" || subscriptionStatus != "active" || instanceStatus != "running" || orderStatus != "fulfilled" || reservationStatus != "committed" || providerInstanceID == "" {
		t.Fatalf("unexpected result operation=%s subscription=%s instance=%s order=%s reservation=%s provider_instance=%s", operationStatus, subscriptionStatus, instanceStatus, orderStatus, reservationStatus, providerInstanceID)
	}
	var notifications int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id=$1 AND type='instance_ready'`, userID).Scan(&notifications); err != nil || notifications != 1 {
		t.Fatalf("notification count=%d error=%v", notifications, err)
	}
	if _, err := repository.EnsureForPaidOrder(ctx, order.ID, "duplicate"); err != nil {
		t.Fatalf("duplicate trigger error=%v", err)
	}
	var operationCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM operations WHERE idempotency_key=$1`, "provision:"+order.ID.String()+":1").Scan(&operationCount); err != nil || operationCount != 1 {
		t.Fatalf("operation count=%d error=%v", operationCount, err)
	}
}
