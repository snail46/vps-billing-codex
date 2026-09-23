package runman

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"reflect"
	"time"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(p *pgxpool.Pool) *PostgresStore { return &PostgresStore{p} }
func (s *PostgresStore) Authenticate(ctx context.Context, token []byte) (uuid.UUID, error) {
	sum := sha256.Sum256(token)
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `UPDATE agent_tokens SET last_used_at=now() WHERE token_hash=$1 AND status='active' RETURNING node_id`, sum[:]).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return id, err
}
func (s *PostgresStore) Connected(ctx context.Context, node, id uuid.UUID, remote string) error {
	_, e := s.pool.Exec(ctx, `INSERT INTO agent_connections(node_id,connection_id,status,remote_address,connected_at) VALUES($1,$2,'connected',$3,now()) ON CONFLICT(node_id) DO UPDATE SET connection_id=$2,status='connected',remote_address=$3,connected_at=now(),disconnected_at=NULL`, node, id, remote)
	return e
}
func (s *PostgresStore) Disconnected(ctx context.Context, node, id uuid.UUID) error {
	_, e := s.pool.Exec(ctx, `UPDATE agent_connections SET status='disconnected',disconnected_at=now() WHERE node_id=$1 AND connection_id=$2`, node, id)
	return e
}
func (s *PostgresStore) ClaimMessage(ctx context.Context, node uuid.UUID, messageID string) (bool, error) {
	result, err := s.pool.Exec(ctx, `INSERT INTO agent_messages(message_id,node_id) VALUES($1,$2) ON CONFLICT(message_id) DO UPDATE SET node_id=EXCLUDED.node_id,received_at=now() WHERE agent_messages.received_at<now()-interval '5 minutes'`, messageID, node)
	return result.RowsAffected() == 1, err
}
func (s *PostgresStore) ReleaseMessage(ctx context.Context, node uuid.UUID, messageID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM agent_messages WHERE message_id=$1 AND node_id=$2`, messageID, node)
	return err
}
func (s *PostgresStore) Heartbeat(ctx context.Context, node uuid.UUID, h Heartbeat) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, e = tx.Exec(ctx, `UPDATE agent_connections SET last_heartbeat_at=$2,status='connected' WHERE node_id=$1`, node, h.Timestamp)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE nodes SET status='online',last_seen_at=$2,cpu_total=CASE WHEN $3>0 THEN $3 ELSE cpu_total END,memory_total_mb=CASE WHEN $4>0 THEN $4 ELSE memory_total_mb END,disk_total_gb=CASE WHEN $5>0 THEN $5 ELSE disk_total_gb END,capabilities=capabilities||jsonb_build_object('virtualization',$6,'entry_host',$7,'entry_ipv6',$8),updated_at=now() WHERE id=$1`, node, h.Timestamp, h.CPUs, h.RAMTotalMB, h.DiskTotalGB, h.VirtType, h.EntryHost, h.EntryIPv6)
	if e != nil {
		return e
	}
	for _, v := range h.VMs {
		raw, _ := json.Marshal(v.IPs)
		_, e = tx.Exec(ctx, `INSERT INTO agent_vm_states(node_id,instance_id,status,cpu_percent,ram_used_mb,traffic_in_bytes,traffic_out_bytes,monthly_traffic_in,monthly_traffic_out,ips,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11) ON CONFLICT(node_id,instance_id) DO UPDATE SET status=$3,cpu_percent=$4,ram_used_mb=$5,traffic_in_bytes=$6,traffic_out_bytes=$7,monthly_traffic_in=$8,monthly_traffic_out=$9,ips=$10::jsonb,observed_at=$11`, node, v.InstanceID, v.Status, v.CPUPercent, v.RAMUsedMB, v.TrafficIn, v.TrafficOut, v.MonthlyIn, v.MonthlyOut, string(raw), v.ObservedAt)
		if e != nil {
			return e
		}
	}
	for _, im := range h.Images {
		_, e = tx.Exec(ctx, `INSERT INTO agent_images(node_id,image_id,name) VALUES($1,$2,$3) ON CONFLICT(node_id,image_id) DO UPDATE SET name=$3,updated_at=now()`, node, im.ID, im.Name)
		if e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (s *PostgresStore) CreateCommand(ctx context.Context, node uuid.UUID, op *uuid.UUID, key, kind string, payload any) (Command, error) {
	raw, e := json.Marshal(payload)
	if e != nil {
		return Command{}, e
	}
	id := uuid.New()
	var c Command
	var stored []byte
	e = s.pool.QueryRow(ctx, `INSERT INTO agent_commands(id,node_id,operation_id,idempotency_key,command_type,payload) VALUES($1,$2,$3,$4,$5,$6::jsonb) ON CONFLICT(idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key RETURNING id,node_id,command_type,payload,status,COALESCE(result,'null'),COALESCE(error_message,'')`, id, node, op, key, kind, string(raw)).Scan(&c.ID, &c.NodeID, &c.Type, &stored, &c.Status, &c.Result, &c.Error)
	if e != nil {
		return c, e
	}
	if !equalJSON(stored, raw) || c.NodeID != node || c.Type != kind {
		return Command{}, errors.New("runman idempotency conflict")
	}
	c.Payload = stored
	return c, nil
}
func (s *PostgresStore) Pending(ctx context.Context, node uuid.UUID) ([]Command, error) {
	rows, e := s.pool.Query(ctx, `SELECT id,node_id,command_type,payload,status FROM agent_commands WHERE node_id=$1 AND status IN('queued','dispatched') ORDER BY created_at`, node)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Command{}
	for rows.Next() {
		var c Command
		if e = rows.Scan(&c.ID, &c.NodeID, &c.Type, &c.Payload, &c.Status); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *PostgresStore) MarkDispatched(ctx context.Context, id uuid.UUID) error {
	_, e := s.pool.Exec(ctx, `UPDATE agent_commands SET status='dispatched',dispatched_at=COALESCE(dispatched_at,now()),updated_at=now() WHERE id=$1 AND status IN('queued','dispatched')`, id)
	return e
}
func (s *PostgresStore) Complete(ctx context.Context, id uuid.UUID, success bool, result json.RawMessage, message string) error {
	status := "failed"
	if success {
		status = "succeeded"
	}
	if len(result) == 0 {
		result = []byte(`null`)
	}
	_, e := s.pool.Exec(ctx, `UPDATE agent_commands SET status=$2,result=$3::jsonb,error_message=NULLIF($4,''),payload=CASE WHEN command_type IN('create','reinstall','reset_password') THEN payload-'root_password' ELSE payload END,completed_at=now(),updated_at=now() WHERE id=$1 AND status NOT IN('succeeded','failed')`, id, status, string(result), message)
	return e
}
func (s *PostgresStore) Command(ctx context.Context, id uuid.UUID) (Command, error) {
	var c Command
	e := s.pool.QueryRow(ctx, `SELECT id,node_id,command_type,payload,status,COALESCE(result,'null'),COALESCE(error_message,'') FROM agent_commands WHERE id=$1`, id).Scan(&c.ID, &c.NodeID, &c.Type, &c.Payload, &c.Status, &c.Result, &c.Error)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return c, e
}
func (s *PostgresStore) VM(ctx context.Context, node, id uuid.UUID) (VMState, error) {
	var v VMState
	var raw []byte
	e := s.pool.QueryRow(ctx, `SELECT instance_id,status,cpu_percent,ram_used_mb,traffic_in_bytes,traffic_out_bytes,monthly_traffic_in,monthly_traffic_out,ips,observed_at FROM agent_vm_states WHERE node_id=$1 AND instance_id=$2`, node, id).Scan(&v.InstanceID, &v.Status, &v.CPUPercent, &v.RAMUsedMB, &v.TrafficIn, &v.TrafficOut, &v.MonthlyIn, &v.MonthlyOut, &raw, &v.ObservedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	_ = json.Unmarshal(raw, &v.IPs)
	return v, e
}
func (s *PostgresStore) InstanceSpec(ctx context.Context, id uuid.UUID) (InstanceSpec, error) {
	var v InstanceSpec
	e := s.pool.QueryRow(ctx, `SELECT cpu_cores,memory_mb,disk_gb,COALESCE(bandwidth_mbps,0) FROM instances WHERE id=$1`, id).Scan(&v.CPU, &v.MemoryMB, &v.DiskGB, &v.BandwidthMbps)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return v, e
}
func (s *PostgresStore) Images(ctx context.Context, node uuid.UUID) ([]Image, error) {
	rows, e := s.pool.Query(ctx, `SELECT image_id,name FROM agent_images WHERE node_id=$1 ORDER BY image_id`, node)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Image{}
	for rows.Next() {
		var i Image
		if e = rows.Scan(&i.ID, &i.Name); e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (s *PostgresStore) PortForwards(ctx context.Context, node, vm uuid.UUID) ([]PortForward, error) {
	rows, e := s.pool.Query(ctx, `SELECT protocol,host_port,guest_port,description FROM agent_port_forwards WHERE node_id=$1 AND instance_id=$2 ORDER BY protocol,host_port`, node, vm)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []PortForward{}
	for rows.Next() {
		var p PortForward
		if e = rows.Scan(&p.Protocol, &p.HostPort, &p.GuestPort, &p.Description); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *PostgresStore) SavePortForwards(ctx context.Context, node, vm uuid.UUID, items []PortForward) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, e = tx.Exec(ctx, `DELETE FROM agent_port_forwards WHERE node_id=$1 AND instance_id=$2`, node, vm); e != nil {
		return e
	}
	for _, p := range items {
		if _, e = tx.Exec(ctx, `INSERT INTO agent_port_forwards(node_id,instance_id,protocol,host_port,guest_port,description)VALUES($1,$2,$3,$4,$5,$6)`, node, vm, p.Protocol, p.HostPort, p.GuestPort, p.Description); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (s *PostgresStore) Healthy(ctx context.Context, node uuid.UUID, maxAge time.Duration) (bool, error) {
	var ok bool
	e := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_connections WHERE node_id=$1 AND status='connected' AND last_heartbeat_at>now()-$2::interval)`, node, maxAge.String()).Scan(&ok)
	return ok, e
}
func IssueToken(ctx context.Context, pool *pgxpool.Pool, node uuid.UUID) (string, error) {
	token := uuid.NewString() + uuid.NewString()
	sum := sha256.Sum256([]byte(token))
	_, e := pool.Exec(ctx, `INSERT INTO agent_tokens(id,node_id,token_hash)VALUES($1,$2,$3)`, uuid.New(), node, sum[:])
	return token, e
}

func equalJSON(a, b []byte) bool {
	var left, right any
	if json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}
