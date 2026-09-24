package reconcile

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"vps-billing/backend/internal/migrations"
	"vps-billing/backend/internal/operation"
	providercontract "vps-billing/backend/internal/provider"
	providermock "vps-billing/backend/internal/provider/mock"
)

func TestProcessorRecoversDurableStateAndSchedulesDriftOperation(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required")
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

	userID, productID, planID := uuid.New(), uuid.New(), uuid.New()
	providerID, groupID, nodeID, offlineNodeID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	subscriptionID, offlineSubscriptionID := uuid.New(), uuid.New()
	instanceID, offlineInstanceID := uuid.New(), uuid.New()
	stuckOperationID, reservationOperationID, reservationID := uuid.New(), uuid.New(), uuid.New()
	fixtures := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users(id,email,password_hash,status,locale,timezone) VALUES($1,$2,'test','active','en-US','UTC')`, []any{userID, "reconcile-" + userID.String() + "@example.com"}},
		{`INSERT INTO node_groups(id,name,region,status) VALUES($1,$2,'test','active')`, []any{groupID, "reconcile-group-" + groupID.String()}},
		{`INSERT INTO products(id,slug,name_i18n,status) VALUES($1,$2,'{"en-US":"Reconcile"}','active')`, []any{productID, "reconcile-product-" + productID.String()}},
		{`INSERT INTO plans(id,product_id,node_group_id,slug,name_i18n,status,cpu_cores,memory_mb,disk_gb,virtualization,billing_cycle,price_minor,currency) VALUES($1,$2,$3,$4,'{"en-US":"Reconcile"}','active',1,1024,20,'kvm','monthly',100,'USD')`, []any{planID, productID, groupID, "reconcile-plan-" + planID.String()}},
		{`INSERT INTO providers(id,name,provider_type,status) VALUES($1,$2,'mock','active')`, []any{providerID, "reconcile-provider-" + providerID.String()}},
		{`INSERT INTO nodes(id,provider_id,node_group_id,provider_node_id,name,region,status,cpu_total,memory_total_mb,disk_total_gb,cpu_reserved,memory_reserved_mb,disk_reserved_gb,capabilities,last_seen_at) VALUES($1,$2,$3,$4,$5,'test','online',8,8192,200,1,1024,20,'{"virtualization":"kvm"}',now())`, []any{nodeID, providerID, groupID, nodeID.String(), "reconcile-node-" + nodeID.String()}},
		{`INSERT INTO nodes(id,provider_id,node_group_id,provider_node_id,name,region,status,cpu_total,memory_total_mb,disk_total_gb,capabilities,last_seen_at) VALUES($1,$2,$3,$4,$5,'test','online',8,8192,200,'{"virtualization":"kvm"}',now()-interval '10 minutes')`, []any{offlineNodeID, providerID, groupID, offlineNodeID.String(), "offline-node-" + offlineNodeID.String()}},
		{`INSERT INTO subscriptions(id,user_id,plan_id,status,billing_cycle,price_minor,currency) VALUES($1,$2,$3,'suspended','monthly',100,'USD'),($4,$2,$3,'active','monthly',100,'USD')`, []any{subscriptionID, userID, planID, offlineSubscriptionID}},
		{`INSERT INTO instances(id,subscription_id,node_id,provider_id,provider_instance_id,name,desired_state,observed_state,cpu_cores,memory_mb,disk_gb,image_id,last_synced_at) VALUES($1,$2,$3,$4,$5,$6,'running','running',1,1024,20,'ubuntu-24.04',now()-interval '10 minutes'),($7,$8,$9,$4,$10,$11,'running','running',1,1024,20,'ubuntu-24.04',now())`, []any{instanceID, subscriptionID, nodeID, providerID, "mock-" + instanceID.String(), "instance-" + instanceID.String(), offlineInstanceID, offlineSubscriptionID, offlineNodeID, "mock-" + offlineInstanceID.String(), "instance-" + offlineInstanceID.String()}},
		{`INSERT INTO operations(id,type,resource_type,resource_id,status,phase,progress,idempotency_key,max_retries,retry_count,trace_id,heartbeat_at) VALUES($1,'restart','instance',$2,'running','provider',40,$3,3,0,'stuck-test',now()-interval '10 minutes'),($4,'provision','instance',$5,'failed','failed',100,$6,3,3,'reservation-test',now()-interval '10 minutes')`, []any{stuckOperationID, uuid.New(), "stuck-" + stuckOperationID.String(), reservationOperationID, uuid.New(), "reservation-" + reservationOperationID.String()}},
		{`INSERT INTO resource_reservations(id,node_id,operation_id,cpu_cores,memory_mb,disk_gb,status,expires_at) VALUES($1,$2,$3,1,1024,20,'reserved',now()-interval '1 minute')`, []any{reservationID, nodeID, reservationOperationID}},
		{`INSERT INTO agent_connections(node_id,connection_id,status,connected_at,last_heartbeat_at) VALUES($1,$2,'connected',now()-interval '10 minutes',now()-interval '10 minutes')`, []any{offlineNodeID, uuid.New()}},
	}
	for _, fixture := range fixtures {
		if _, err = pool.Exec(ctx, fixture.query, fixture.args...); err != nil {
			t.Fatal(err)
		}
	}

	mockProvider := providermock.New()
	if _, err = mockProvider.CreateInstance(ctx, providercontract.CreateInstanceRequest{OperationID: uuid.NewString(), IdempotencyKey: "fixture-create-" + instanceID.String(), NodeID: nodeID.String(), InstanceID: instanceID.String(), CPUCores: 1, MemoryMB: 1024, DiskGB: 20, Image: "ubuntu-24.04"}); err != nil {
		t.Fatal(err)
	}
	registry := providercontract.NewRegistry()
	if err = registry.Register(providerID, mockProvider); err != nil {
		t.Fatal(err)
	}
	operationRepository := operation.NewPostgresRepository(pool)
	processor := New(pool, registry, operation.NewService(operationRepository), operationRepository)
	processor.batchSize = 500
	processor.now = func() time.Time { return time.Now().UTC() }
	if _, err = processor.ProcessBatch(ctx); err != nil {
		t.Fatal(err)
	}

	assertValue(t, pool, `SELECT status||':'||retry_count::text FROM operations WHERE id=$1`, stuckOperationID, "queued:1")
	assertValue(t, pool, `SELECT status FROM resource_reservations WHERE id=$1`, reservationID, "expired")
	assertValue(t, pool, `SELECT cpu_reserved::text||':'||memory_reserved_mb::text||':'||disk_reserved_gb::text FROM nodes WHERE id=$1`, nodeID, "0.00:0:0")
	assertValue(t, pool, `SELECT status FROM nodes WHERE id=$1`, offlineNodeID, "offline")
	assertValue(t, pool, `SELECT observed_state FROM instances WHERE id=$1`, offlineInstanceID, "unknown")
	assertValue(t, pool, `SELECT desired_state FROM instances WHERE id=$1`, instanceID, "suspended")
	assertValue(t, pool, `SELECT type||':'||status FROM operations WHERE resource_id=$1 AND type='suspend'`, instanceID, "suspend:queued")
	assertValue(t, pool, `SELECT count(*)::text FROM outbox_events WHERE aggregate_id=$1 AND event_type='operation.queued.v1'`, stuckOperationID, "1")
}

func assertValue(t *testing.T, pool *pgxpool.Pool, query string, id uuid.UUID, expected string) {
	t.Helper()
	var actual string
	if err := pool.QueryRow(context.Background(), query, id).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("query value = %q, want %q", actual, expected)
	}
}
