package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"vps-billing/backend/internal/migrations"
)

func TestDeterministicSchedulerReservationLifecycle(t *testing.T) {
	pool, ctx := infrastructureIntegrationPool(t)
	repository := NewPostgresRepository(pool)
	providerID, groupID := seedInfrastructure(t, ctx, pool, repository)
	capabilities := json.RawMessage(`{"create_instance":true,"virtualization":["kvm"]}`)
	lowWeightID, highWeightID := uuid.New(), uuid.New()
	for _, node := range []Node{
		{ID: lowWeightID, ProviderID: providerID, NodeGroupID: &groupID, Name: "low-" + lowWeightID.String(), Region: "test-1", Status: "online", Total: Capacity{CPUCores: 8, MemoryMB: 8192, DiskGB: 100, IPv4Count: 4, NATPortCount: 100}, Weight: 100, Capabilities: capabilities},
		{ID: highWeightID, ProviderID: providerID, NodeGroupID: &groupID, Name: "high-" + highWeightID.String(), Region: "test-1", Status: "online", Total: Capacity{CPUCores: 8, MemoryMB: 8192, DiskGB: 100, IPv4Count: 4, NATPortCount: 100}, Weight: 200, Capabilities: capabilities},
	} {
		if _, err := repository.CreateNode(ctx, node); err != nil {
			t.Fatal(err)
		}
	}
	incompatibleID := uuid.New()
	if _, err := repository.CreateNode(ctx, Node{ID: incompatibleID, ProviderID: providerID, NodeGroupID: &groupID, Name: "incompatible-" + incompatibleID.String(), Region: "test-1", Status: "online", Total: Capacity{CPUCores: 8, MemoryMB: 8192, DiskGB: 100, IPv4Count: 4, NATPortCount: 100}, Weight: 1000, Capabilities: json.RawMessage(`{"create_instance":true,"virtualization":["lxc"]}`)}); err != nil {
		t.Fatal(err)
	}
	operationID := seedOperation(t, ctx, pool)
	scheduler := NewScheduler(repository)
	request := ScheduleRequest{NodeGroupID: groupID, Resources: Capacity{CPUCores: 2, MemoryMB: 2048, DiskGB: 20, IPv4Count: 1, NATPortCount: 10}, RequiredCapabilities: map[string]any{"create_instance": true, "virtualization": []string{"kvm"}}, TTL: 5 * time.Minute}
	reservation, err := scheduler.Reserve(ctx, operationID, request)
	if err != nil {
		t.Fatal(err)
	}
	if reservation.NodeID != highWeightID || reservation.Status != "reserved" {
		t.Fatalf("reservation=%#v", reservation)
	}
	repeated, err := scheduler.Reserve(ctx, operationID, request)
	if err != nil || repeated.ID != reservation.ID {
		t.Fatalf("repeated reservation=%#v error=%v", repeated, err)
	}
	committed, err := scheduler.Commit(ctx, reservation.ID)
	if err != nil || committed.Status != "committed" {
		t.Fatalf("Commit()=%#v error=%v", committed, err)
	}
	if _, err := scheduler.Commit(ctx, reservation.ID); err != nil {
		t.Fatalf("idempotent Commit() error=%v", err)
	}
	if _, err := scheduler.Release(ctx, reservation.ID); !errors.Is(err, ErrReservationState) {
		t.Fatalf("release committed error=%v", err)
	}
	releasable, err := scheduler.Reserve(ctx, seedOperation(t, ctx, pool), request)
	if err != nil {
		t.Fatal(err)
	}
	released, err := scheduler.Release(ctx, releasable.ID)
	if err != nil || released.Status != "released" {
		t.Fatalf("Release()=%#v error=%v", released, err)
	}
	if _, err := scheduler.Release(ctx, releasable.ID); err != nil {
		t.Fatalf("idempotent Release() error=%v", err)
	}

	var reservedCPU, allocatedCPU float64
	if err := pool.QueryRow(ctx, `SELECT cpu_reserved,cpu_allocated FROM nodes WHERE id=$1`, highWeightID).Scan(&reservedCPU, &allocatedCPU); err != nil {
		t.Fatal(err)
	}
	if reservedCPU != 0 || allocatedCPU != 2 {
		t.Fatalf("reserved=%v allocated=%v", reservedCPU, allocatedCPU)
	}
	providers, err := repository.ListProviders(ctx)
	if err != nil || len(providers) == 0 {
		t.Fatalf("ListProviders()=%#v error=%v", providers, err)
	}
	nodes, err := repository.ListNodes(ctx)
	if err != nil || len(nodes) < 3 {
		t.Fatalf("ListNodes() len=%d error=%v", len(nodes), err)
	}
}

func TestConcurrentReservationsCannotOversubscribe(t *testing.T) {
	pool, ctx := infrastructureIntegrationPool(t)
	repository := NewPostgresRepository(pool)
	providerID, groupID := seedInfrastructure(t, ctx, pool, repository)
	nodeID := uuid.New()
	if _, err := repository.CreateNode(ctx, Node{ID: nodeID, ProviderID: providerID, NodeGroupID: &groupID, Name: "single-" + nodeID.String(), Region: "test-2", Status: "online", Total: Capacity{CPUCores: 2, MemoryMB: 2048, DiskGB: 20}, Weight: 100, Capabilities: json.RawMessage(`{"create_instance":true}`)}); err != nil {
		t.Fatal(err)
	}
	scheduler := NewScheduler(repository)
	request := ScheduleRequest{NodeGroupID: groupID, Resources: Capacity{CPUCores: 2, MemoryMB: 2048, DiskGB: 20}, RequiredCapabilities: map[string]any{"create_instance": true}, TTL: time.Minute}
	operationIDs := []uuid.UUID{seedOperation(t, ctx, pool), seedOperation(t, ctx, pool)}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, operationID := range operationIDs {
		wait.Add(1)
		go func(id uuid.UUID) {
			defer wait.Done()
			_, reserveErr := scheduler.Reserve(ctx, id, request)
			results <- reserveErr
		}(operationID)
	}
	wait.Wait()
	close(results)
	successes, exhausted := 0, 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, ErrResourceExhausted):
			exhausted++
		default:
			t.Fatalf("Reserve() error=%v", result)
		}
	}
	if successes != 1 || exhausted != 1 {
		t.Fatalf("successes=%d exhausted=%d", successes, exhausted)
	}
	var reservedCPU float64
	if err := pool.QueryRow(ctx, `SELECT cpu_reserved FROM nodes WHERE id=$1`, nodeID).Scan(&reservedCPU); err != nil {
		t.Fatal(err)
	}
	if reservedCPU != 2 {
		t.Fatalf("cpu_reserved=%v", reservedCPU)
	}
}

func infrastructureIntegrationPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	if err := migrations.Run(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, ctx
}

func seedInfrastructure(t *testing.T, ctx context.Context, pool *pgxpool.Pool, repository *PostgresRepository) (uuid.UUID, uuid.UUID) {
	t.Helper()
	providerID, groupID := uuid.New(), uuid.New()
	capabilities := json.RawMessage(`{"create_instance":true,"virtualization":["kvm"]}`)
	if _, err := repository.CreateProvider(ctx, ProviderRecord{ID: providerID, Name: "mock-" + providerID.String(), ProviderType: "mock", Status: "active", Capabilities: capabilities}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateNodeGroup(ctx, NodeGroup{ID: groupID, Name: "group-" + groupID.String(), Region: "test", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	return providerID, groupID
}

func seedOperation(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO operations (id,type,resource_type,resource_id,status,idempotency_key,trace_id) VALUES ($1,'provision','subscription',$2,'queued',$3,$4)`, id, uuid.New(), "operation-"+id.String(), "trace-"+id.String()); err != nil {
		t.Fatal(err)
	}
	return id
}
