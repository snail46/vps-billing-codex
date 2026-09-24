package observability

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Snapshot struct {
	OutboxPending    int64
	OperationsActive int64
	OperationsFailed int64
	NodesOffline     int64
	AgentsStale      int64
	Payments24h      int64
	CPUAvailable     float64
	MemoryAvailable  float64
	DiskAvailable    float64
}

type PostgresMetrics struct {
	pool *pgxpool.Pool
}

func NewPostgresMetrics(pool *pgxpool.Pool) *PostgresMetrics {
	return &PostgresMetrics{pool: pool}
}

func (m *PostgresMetrics) Snapshot(ctx context.Context) (Snapshot, error) {
	var snapshot Snapshot
	err := m.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM outbox_events WHERE status='pending'),
		(SELECT count(*) FROM operations WHERE status IN ('queued','running','waiting_provider','waiting_resource','verifying','retrying')),
		(SELECT count(*) FROM operations WHERE status='failed' AND created_at>now()-interval '24 hours'),
		(SELECT count(*) FROM nodes WHERE status<>'online'),
		(SELECT count(*) FROM agent_connections WHERE status='connected' AND last_heartbeat_at<now()-interval '60 seconds'),
		(SELECT count(*) FROM payments WHERE status='succeeded' AND paid_at>now()-interval '24 hours'),
		(SELECT COALESCE(sum(cpu_total-cpu_allocated-cpu_reserved),0) FROM nodes WHERE status='online'),
		(SELECT COALESCE(sum(memory_total_mb-memory_allocated_mb-memory_reserved_mb),0) FROM nodes WHERE status='online'),
		(SELECT COALESCE(sum(disk_total_gb-disk_allocated_gb-disk_reserved_gb),0) FROM nodes WHERE status='online')`).Scan(
		&snapshot.OutboxPending, &snapshot.OperationsActive, &snapshot.OperationsFailed,
		&snapshot.NodesOffline, &snapshot.AgentsStale, &snapshot.Payments24h,
		&snapshot.CPUAvailable, &snapshot.MemoryAvailable, &snapshot.DiskAvailable,
	)
	return snapshot, err
}
