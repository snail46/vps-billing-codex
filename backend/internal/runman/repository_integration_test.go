package runman

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"vps-billing/backend/internal/migrations"
)

func TestPostgresStoreTokenHeartbeatAndIdempotentCommand(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required")
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
	providerID, nodeID := uuid.New(), uuid.New()
	if _, err = pool.Exec(ctx, `INSERT INTO providers(id,name,provider_type,status) VALUES($1,$2,'runman','active')`, providerID, "runman-test-"+providerID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO nodes(id,provider_id,name,region,status) VALUES($1,$2,$3,'test','offline')`, nodeID, providerID, "runman-node-"+nodeID.String()); err != nil {
		t.Fatal(err)
	}
	store := NewPostgresStore(pool)
	token, err := IssueToken(ctx, pool, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	authenticated, err := store.Authenticate(ctx, []byte(token))
	if err != nil || authenticated != nodeID {
		t.Fatalf("Authenticate() = %v, %v", authenticated, err)
	}
	if _, err = store.Authenticate(ctx, []byte("wrong-token")); err != ErrNotFound {
		t.Fatalf("wrong token error = %v", err)
	}
	connectionID := uuid.New()
	if err = store.Connected(ctx, nodeID, connectionID, "127.0.0.1:1234"); err != nil {
		t.Fatal(err)
	}
	if err = store.Heartbeat(ctx, nodeID, Heartbeat{Timestamp: time.Now().UTC(), CPUs: 8, RAMTotalMB: 16384, DiskTotalGB: 400, VirtType: "incus"}); err != nil {
		t.Fatal(err)
	}
	if healthy, healthErr := store.Healthy(ctx, nodeID, 90*time.Second); healthErr != nil || !healthy {
		t.Fatalf("Healthy() = %v, %v", healthy, healthErr)
	}

	first, err := store.CreateCommand(ctx, nodeID, nil, "same-key", "start", map[string]any{"vm_id": uuid.NewString(), "force": false})
	if err != nil {
		t.Fatal(err)
	}
	// PostgreSQL jsonb normalizes object representation; semantic comparison
	// must still recognize this as the same request.
	var payload map[string]any
	if err = json.Unmarshal(first.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateCommand(ctx, nodeID, nil, "same-key", "start", payload)
	if err != nil || first.ID != second.ID {
		t.Fatalf("duplicate CreateCommand() = %v, %v", second.ID, err)
	}
	if _, err = store.CreateCommand(ctx, nodeID, nil, "same-key", "stop", payload); err == nil {
		t.Fatal("idempotency key accepted a different command")
	}
}
