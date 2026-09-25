package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const systemAdjustmentAccount = "00000000-0000-0000-0000-000000000001"

var listQueries = map[string]string{
	"users":         `SELECT to_jsonb(u) - 'password_hash' FROM users u ORDER BY created_at DESC LIMIT 200`,
	"products":      `SELECT jsonb_build_object('id',p.id,'slug',p.slug,'name_i18n',p.name_i18n,'status',p.status,'sort_order',p.sort_order,'plans',COALESCE(jsonb_agg(to_jsonb(pl) ORDER BY pl.price_minor) FILTER (WHERE pl.id IS NOT NULL),'[]'::jsonb)) FROM products p LEFT JOIN plans pl ON pl.product_id=p.id GROUP BY p.id ORDER BY p.sort_order,p.slug LIMIT 200`,
	"orders":        `SELECT to_jsonb(o) || jsonb_build_object('user_email',u.email,'payment_status',pay.status) FROM orders o JOIN users u ON u.id=o.user_id LEFT JOIN payments pay ON pay.order_id=o.id ORDER BY o.created_at DESC LIMIT 200`,
	"payments":      `SELECT to_jsonb(p) - 'gateway_payload' || jsonb_build_object('order_no',o.order_no,'user_email',u.email) FROM payments p JOIN orders o ON o.id=p.order_id JOIN users u ON u.id=o.user_id ORDER BY p.created_at DESC LIMIT 200`,
	"ledger":        `SELECT to_jsonb(t) || jsonb_build_object('entries',COALESCE(jsonb_agg(to_jsonb(e) ORDER BY e.created_at),'[]'::jsonb)) FROM ledger_transactions t LEFT JOIN ledger_entries e ON e.transaction_id=t.id GROUP BY t.id ORDER BY t.created_at DESC LIMIT 200`,
	"subscriptions": `SELECT to_jsonb(s) || jsonb_build_object('user_email',u.email,'plan_slug',p.slug) FROM subscriptions s JOIN users u ON u.id=s.user_id JOIN plans p ON p.id=s.plan_id ORDER BY s.created_at DESC LIMIT 200`,
	"instances":     `SELECT to_jsonb(i) || jsonb_build_object('user_id',s.user_id,'user_email',u.email,'subscription_status',s.status,'node_name',n.name,'provider_name',p.name) FROM instances i JOIN subscriptions s ON s.id=i.subscription_id JOIN users u ON u.id=s.user_id LEFT JOIN nodes n ON n.id=i.node_id LEFT JOIN providers p ON p.id=i.provider_id ORDER BY i.created_at DESC LIMIT 200`,
	"nodes":         `SELECT to_jsonb(n) || jsonb_build_object('provider_name',p.name,'available_cpu',n.cpu_total-n.cpu_allocated-n.cpu_reserved,'available_memory_mb',n.memory_total_mb-n.memory_allocated_mb-n.memory_reserved_mb,'available_disk_gb',n.disk_total_gb-n.disk_allocated_gb-n.disk_reserved_gb) FROM nodes n JOIN providers p ON p.id=n.provider_id ORDER BY n.status,n.name LIMIT 200`,
	"providers":     `SELECT to_jsonb(p) - 'credential_ref' - 'config' FROM providers p ORDER BY p.status,p.name LIMIT 200`,
	"operations":    `SELECT to_jsonb(o) || jsonb_build_object('provider_name',p.name) FROM operations o LEFT JOIN providers p ON p.id=o.provider_id ORDER BY o.created_at DESC LIMIT 200`,
	"tickets":       `SELECT to_jsonb(t) || jsonb_build_object('user_email',u.email,'message_count',(SELECT count(*) FROM ticket_messages m WHERE m.ticket_id=t.id)) FROM tickets t JOIN users u ON u.id=t.user_id ORDER BY t.updated_at DESC LIMIT 200`,
	"audit":         `SELECT to_jsonb(a) || jsonb_build_object('actor_email',ad.email) FROM audit_events a LEFT JOIN admins ad ON ad.id=a.actor_id ORDER BY a.created_at DESC LIMIT 200`,
	"admins":        `SELECT to_jsonb(a) - 'password_hash' || jsonb_build_object('roles',COALESCE(jsonb_agg(r.key ORDER BY r.key) FILTER (WHERE r.id IS NOT NULL),'[]'::jsonb)) FROM admins a LEFT JOIN admin_roles ar ON ar.admin_id=a.id LEFT JOIN roles r ON r.id=ar.role_id GROUP BY a.id ORDER BY a.created_at DESC LIMIT 200`,
	"roles":         `SELECT to_jsonb(r) || jsonb_build_object('permissions',COALESCE(jsonb_agg(p.key ORDER BY p.key) FILTER (WHERE p.id IS NOT NULL),'[]'::jsonb)) FROM roles r LEFT JOIN role_permissions rp ON rp.role_id=r.id LEFT JOIN permissions p ON p.id=rp.permission_id GROUP BY r.id ORDER BY r.key`,
	"settings":      `SELECT jsonb_build_object('key',key,'value',CASE WHEN is_secret THEN to_jsonb('***'::text) ELSE value END,'is_secret',is_secret,'updated_at',updated_at,'updated_by',updated_by) FROM system_settings ORDER BY key`,
	"usage":         `SELECT to_jsonb(b)||jsonb_build_object('user_email',u.email,'instance_name',i.name) FROM usage_billing_periods b JOIN subscriptions s ON s.id=b.subscription_id JOIN users u ON u.id=s.user_id JOIN instances i ON i.id=b.instance_id ORDER BY b.period_end DESC LIMIT 200`,
	"dead_letters":  `SELECT to_jsonb(o)-'payload'-'last_error'||jsonb_build_object('error_code',CASE WHEN o.last_error IS NULL THEN NULL ELSE 'DELIVERY_FAILED' END) FROM outbox_events o WHERE status='dead_letter' ORDER BY dead_lettered_at DESC LIMIT 200`,
}

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) List(ctx context.Context, resource string) ([]Item, error) {
	query, ok := listQueries[resource]
	if !ok {
		return nil, ErrNotFound
	}
	return listJSON(ctx, r.pool, query)
}

func (r *PostgresRepository) Dashboard(ctx context.Context) (Dashboard, error) {
	metrics, err := oneJSON(ctx, r.pool, `SELECT jsonb_build_object(
      'users',(SELECT count(*) FROM users),'active_subscriptions',(SELECT count(*) FROM subscriptions WHERE status='active'),
      'monthly_revenue_minor',(SELECT COALESCE(sum(amount_minor),0) FROM payments WHERE status='succeeded' AND paid_at >= date_trunc('month',now())),
      'failed_operations',(SELECT count(*) FROM operations WHERE status='failed' AND created_at > now()-interval '24 hours'),
	  'dead_letters',(SELECT count(*) FROM outbox_events WHERE status='dead_letter'),
	  'usage_periods_overdue',(SELECT count(*) FROM usage_billing_periods WHERE status='open' AND period_end<now()),
      'open_tickets',(SELECT count(*) FROM tickets WHERE status NOT IN ('resolved','closed')),
      'offline_nodes',(SELECT count(*) FROM nodes WHERE status NOT IN ('online','active')))`)
	if err != nil {
		return Dashboard{}, err
	}
	queries := []struct {
		target *[]Item
		query  string
	}{
		{nil, `SELECT jsonb_build_object('kind','operation','id',id,'severity','critical','status',status,'error_code',error_code,'created_at',created_at) FROM operations WHERE status='failed' ORDER BY created_at DESC LIMIT 10`},
		{nil, `SELECT jsonb_build_object('kind','dead_letter','id',id,'severity','critical','status',status,'error_code','DELIVERY_FAILED','created_at',dead_lettered_at) FROM outbox_events WHERE status='dead_letter' ORDER BY dead_lettered_at DESC LIMIT 10`},
		{nil, `SELECT to_jsonb(o) FROM operations o WHERE status='failed' ORDER BY created_at DESC LIMIT 10`},
		{nil, `SELECT to_jsonb(p)-'credential_ref'-'config' FROM providers p ORDER BY CASE WHEN status='active' THEN 1 ELSE 0 END,last_health_check_at NULLS FIRST LIMIT 20`},
		{nil, `SELECT to_jsonb(n) FROM nodes n ORDER BY CASE WHEN status='online' THEN 1 ELSE 0 END,last_seen_at NULLS FIRST LIMIT 20`},
		{nil, `SELECT jsonb_build_object('id',id,'name',name,'cpu_percent',CASE WHEN cpu_total=0 THEN 0 ELSE round(100*(cpu_allocated+cpu_reserved)/cpu_total,1) END,'memory_percent',CASE WHEN memory_total_mb=0 THEN 0 ELSE round(100.0*(memory_allocated_mb+memory_reserved_mb)/memory_total_mb,1) END,'disk_percent',CASE WHEN disk_total_gb=0 THEN 0 ELSE round(100.0*(disk_allocated_gb+disk_reserved_gb)/disk_total_gb,1) END) FROM nodes WHERE (cpu_total>0 AND (cpu_allocated+cpu_reserved)/cpu_total >= .85) OR (memory_total_mb>0 AND 1.0*(memory_allocated_mb+memory_reserved_mb)/memory_total_mb >= .85) OR (disk_total_gb>0 AND 1.0*(disk_allocated_gb+disk_reserved_gb)/disk_total_gb >= .85) ORDER BY name`},
	}
	values := make([][]Item, len(queries))
	for i, q := range queries {
		values[i], err = listJSON(ctx, r.pool, q.query)
		if err != nil {
			return Dashboard{}, err
		}
	}
	critical := append(values[0], values[1]...)
	return Dashboard{Metrics: metrics, CriticalAlerts: critical, FailedOperations: values[2], ProviderHealth: values[3], NodeHealth: values[4], CapacityWarnings: values[5]}, nil
}

func (r *PostgresRepository) Instance(ctx context.Context, id uuid.UUID, diagnostics bool) (Item, error) {
	item, err := oneJSON(ctx, r.pool, `SELECT to_jsonb(i) || jsonb_build_object('user_id',s.user_id,'user_email',u.email,'subscription_status',s.status,'plan_slug',pl.slug,'node',to_jsonb(n),'provider',to_jsonb(p)-'credential_ref'-'config','networks',COALESCE((SELECT jsonb_agg(to_jsonb(x)) FROM instance_networks x WHERE x.instance_id=i.id),'[]'::jsonb),'traffic',COALESCE((SELECT jsonb_agg(to_jsonb(x) ORDER BY period_start DESC) FROM traffic_usage x WHERE x.instance_id=i.id),'[]'::jsonb),'operations',COALESCE((SELECT jsonb_agg(to_jsonb(x) ORDER BY created_at DESC) FROM operations x WHERE x.resource_type='instance' AND x.resource_id=i.id),'[]'::jsonb)) FROM instances i JOIN subscriptions s ON s.id=i.subscription_id JOIN users u ON u.id=s.user_id JOIN plans pl ON pl.id=s.plan_id LEFT JOIN nodes n ON n.id=i.node_id LEFT JOIN providers p ON p.id=i.provider_id WHERE i.id=$1`, id)
	if err == nil && !diagnostics {
		delete(item, "provider_instance_id")
		delete(item, "metadata")
	}
	return item, err
}
func (r *PostgresRepository) Operation(ctx context.Context, id uuid.UUID, raw bool) (Item, error) {
	item, err := oneJSON(ctx, r.pool, `SELECT to_jsonb(o)||jsonb_build_object('steps',COALESCE((SELECT jsonb_agg(to_jsonb(s) ORDER BY step_order) FROM operation_steps s WHERE s.operation_id=o.id),'[]'::jsonb),'attempts',COALESCE((SELECT jsonb_agg(to_jsonb(a)-'error_message' ORDER BY attempt) FROM operation_attempts a WHERE a.operation_id=o.id),'[]'::jsonb),'scheduler_decisions',COALESCE((SELECT jsonb_agg(to_jsonb(d) ORDER BY created_at) FROM scheduler_decisions d WHERE d.operation_id=o.id),'[]'::jsonb),'provider_name',p.name,'node_name',n.name) FROM operations o LEFT JOIN providers p ON p.id=o.provider_id LEFT JOIN instances i ON o.resource_type='instance' AND i.id=o.resource_id LEFT JOIN nodes n ON n.id=i.node_id WHERE o.id=$1`, id)
	if err == nil && !raw {
		delete(item, "error_message")
		delete(item, "provider_operation_id")
		delete(item, "input")
	}
	return item, err
}
func (r *PostgresRepository) Provider(ctx context.Context, id uuid.UUID, diagnostics bool) (Item, error) {
	item, err := oneJSON(ctx, r.pool, `SELECT to_jsonb(p)-'credential_ref'||jsonb_build_object(
		'health_history',COALESCE((SELECT jsonb_agg(to_jsonb(h)-'details' ORDER BY h.checked_at DESC) FROM (SELECT * FROM provider_health_checks WHERE provider_id=p.id ORDER BY checked_at DESC LIMIT 50) h),'[]'::jsonb),
		'nodes',COALESCE((SELECT jsonb_agg(to_jsonb(n) ORDER BY n.name) FROM nodes n WHERE n.provider_id=p.id),'[]'::jsonb))
		FROM providers p WHERE p.id=$1`, id)
	if err == nil && !diagnostics {
		delete(item, "config")
		delete(item, "endpoint")
	}
	return item, err
}

func (r *PostgresRepository) ReplayOutbox(ctx context.Context, id uuid.UUID, a AuditContext) (Item, error) {
	return mutateJSON(ctx, r.pool, func(tx pgx.Tx) (Item, Item, error) {
		before, err := oneJSON(ctx, tx, `SELECT to_jsonb(o)-'payload'-'last_error' FROM outbox_events o WHERE id=$1 AND status='dead_letter' FOR UPDATE`, id)
		if err != nil {
			return nil, nil, err
		}
		after, err := oneJSON(ctx, tx, `UPDATE outbox_events SET status='pending',attempts=0,next_attempt_at=now(),last_error=NULL,dead_lettered_at=NULL,replayed_at=now(),replayed_by=$2 WHERE id=$1 RETURNING to_jsonb(outbox_events)-'payload'-'last_error'`, id, a.AdminID)
		return before, after, err
	}, "outbox.replayed", "outbox_event", id, a)
}

func (r *PostgresRepository) RefundPayment(ctx context.Context, paymentID uuid.UUID, amount int64, reason, key string, a AuditContext) (Item, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var existingPayment uuid.UUID
	var existingAmount int64
	var existingReason string
	findErr := tx.QueryRow(ctx, `SELECT payment_id,amount_minor,reason FROM refunds WHERE idempotency_key=$1`, key).Scan(&existingPayment, &existingAmount, &existingReason)
	if findErr == nil {
		if existingPayment != paymentID || existingAmount != amount || existingReason != reason {
			return nil, ErrRefundInvalid
		}
		existing, itemErr := oneJSON(ctx, tx, `SELECT to_jsonb(r) FROM refunds r WHERE idempotency_key=$1`, key)
		if itemErr != nil {
			return nil, itemErr
		}
		return existing, tx.Commit(ctx)
	} else if !errors.Is(findErr, pgx.ErrNoRows) {
		return nil, findErr
	}
	var paymentAmount, refunded int64
	var currency, status string
	var invoiceID uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT p.amount_minor,p.refunded_minor,p.currency,p.status,i.id FROM payments p JOIN invoices i ON i.order_id=p.order_id WHERE p.id=$1 FOR UPDATE OF p`, paymentID).Scan(&paymentAmount, &refunded, &currency, &status, &invoiceID); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if !contains([]string{"succeeded", "partially_refunded"}, status) || amount > paymentAmount-refunded {
		return nil, ErrRefundInvalid
	}
	refundID, transactionID := uuid.New(), uuid.New()
	if _, err = tx.Exec(ctx, `INSERT INTO ledger_transactions(id,type,reference_type,reference_id,description) VALUES($1,'payment_refund','refund',$2,$3)`, transactionID, refundID, reason); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ledger_entries(id,transaction_id,account_type,account_id,direction,amount_minor,currency) VALUES($1,$2,'accounts_receivable',$3,'debit',$4,$5),($6,$2,'gateway_cash',$7,'credit',$4,$5)`, uuid.New(), transactionID, invoiceID, amount, currency, uuid.New(), paymentID); err != nil {
		return nil, err
	}
	newStatus := "partially_refunded"
	if refunded+amount == paymentAmount {
		newStatus = "refunded"
	}
	if _, err = tx.Exec(ctx, `UPDATE payments SET refunded_minor=refunded_minor+$2,status=$3,updated_at=now() WHERE id=$1`, paymentID, amount, newStatus); err != nil {
		return nil, err
	}
	item, err := oneJSON(ctx, tx, `INSERT INTO refunds(id,payment_id,amount_minor,currency,reason,status,idempotency_key,ledger_transaction_id,created_by) VALUES($1,$2,$3,$4,$5,'succeeded',$6,$7,$8) RETURNING to_jsonb(refunds)`, refundID, paymentID, amount, currency, reason, key, transactionID, a.AdminID)
	if err != nil {
		return nil, err
	}
	if err = recordAudit(ctx, tx, "payment.refunded", "payment", paymentID, nil, item, a); err != nil {
		return nil, err
	}
	eventID := uuid.New()
	payload, _ := json.Marshal(map[string]any{"event_id": eventID, "event_type": "payment.refunded.v1", "aggregate_type": "payment", "aggregate_id": paymentID, "data": map[string]any{"refund_id": refundID, "amount_minor": amount, "currency": currency}})
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_type,aggregate_id,payload) VALUES($1,'payment.refunded.v1','payment',$2,$3)`, eventID, paymentID, payload); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *PostgresRepository) UpdateUserStatus(ctx context.Context, id uuid.UUID, status string, a AuditContext) (Item, error) {
	return mutateJSON(ctx, r.pool, func(tx pgx.Tx) (Item, Item, error) {
		before, e := oneJSON(ctx, tx, `SELECT to_jsonb(u)-'password_hash' FROM users u WHERE id=$1 FOR UPDATE`, id)
		if e != nil {
			return nil, nil, e
		}
		after, e := oneJSON(ctx, tx, `UPDATE users SET status=$2,updated_at=now() WHERE id=$1 RETURNING to_jsonb(users)-'password_hash'`, id, status)
		return before, after, e
	}, "user.status_updated", "user", id, a)
}

func (r *PostgresRepository) AdjustWallet(ctx context.Context, userID uuid.UUID, currency string, amount int64, description string, a AuditContext) (Item, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	walletID := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO wallets(id,user_id,currency) VALUES($1,$2,$3) ON CONFLICT(user_id,currency) DO NOTHING`, walletID, userID, currency)
	if err != nil {
		return nil, err
	}
	var current int64
	err = tx.QueryRow(ctx, `SELECT id,available_balance_minor FROM wallets WHERE user_id=$1 AND currency=$2 FOR UPDATE`, userID, currency).Scan(&walletID, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if current+amount < 0 {
		return nil, ErrInsufficient
	}
	txID := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO ledger_transactions(id,type,reference_type,reference_id,description) VALUES($1,'admin_adjustment','admin_request',$1,$2)`, txID, description)
	if err != nil {
		return nil, err
	}
	directionWallet, directionSystem := "credit", "debit"
	absolute := amount
	if amount < 0 {
		absolute = -amount
		directionWallet, directionSystem = "debit", "credit"
	}
	_, err = tx.Exec(ctx, `INSERT INTO ledger_entries(id,transaction_id,account_type,account_id,direction,amount_minor,currency) VALUES($1,$2,'wallet',$3,$4,$5,$6),($7,$2,'platform_adjustment',$8,$9,$5,$6)`, uuid.New(), txID, walletID, directionWallet, absolute, currency, uuid.New(), uuid.MustParse(systemAdjustmentAccount), directionSystem)
	if err != nil {
		return nil, err
	}
	var payload []byte
	err = tx.QueryRow(ctx, `UPDATE wallets SET available_balance_minor=available_balance_minor+$2,updated_at=now() WHERE id=$1 RETURNING to_jsonb(wallets)`, walletID, amount).Scan(&payload)
	if err != nil {
		return nil, err
	}
	var after Item
	if err = json.Unmarshal(payload, &after); err != nil {
		return nil, err
	}
	before := Item{"id": walletID, "user_id": userID, "currency": currency, "available_balance_minor": current}
	if err = recordAudit(ctx, tx, "ledger.adjusted", "wallet", walletID, before, after, a); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	after["transaction_id"] = txID.String()
	return after, nil
}

func (r *PostgresRepository) ReplyTicket(ctx context.Context, id uuid.UUID, message string, a AuditContext) (Item, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	before, err := oneJSON(ctx, tx, `SELECT to_jsonb(t) FROM tickets t WHERE id=$1 FOR UPDATE`, id)
	if err != nil {
		return nil, err
	}
	status, _ := before["status"].(string)
	if status == "closed" || status == "resolved" {
		return nil, ErrTicketFinalized
	}
	messageID := uuid.New()
	after, err := oneJSON(ctx, tx, `INSERT INTO ticket_messages(id,ticket_id,sender_type,sender_id,message) VALUES($1,$2,'admin',$3,$4) RETURNING to_jsonb(ticket_messages)`, messageID, id, a.AdminID, message)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE tickets SET status='waiting_user',updated_at=now() WHERE id=$1`, id)
	if err != nil {
		return nil, err
	}
	if err = recordAudit(ctx, tx, "ticket.replied", "ticket", id, nil, Item{"message_id": messageID}, a); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return after, nil
}
func (r *PostgresRepository) UpdateTicketStatus(ctx context.Context, id uuid.UUID, status string, a AuditContext) (Item, error) {
	return mutateJSON(ctx, r.pool, func(tx pgx.Tx) (Item, Item, error) {
		before, e := oneJSON(ctx, tx, `SELECT to_jsonb(t) FROM tickets t WHERE id=$1 FOR UPDATE`, id)
		if e != nil {
			return nil, nil, e
		}
		after, e := oneJSON(ctx, tx, `UPDATE tickets SET status=$2,closed_at=CASE WHEN $2 IN ('resolved','closed') THEN now() ELSE NULL END,updated_at=now() WHERE id=$1 RETURNING to_jsonb(tickets)`, id, status)
		return before, after, e
	}, "ticket.status_updated", "ticket", id, a)
}
func (r *PostgresRepository) UpdateSetting(ctx context.Context, key string, value any, secret bool, a AuditContext) (Item, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, ErrInvalidInput
	}
	resourceID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("setting:"+key))
	return mutateJSON(ctx, r.pool, func(tx pgx.Tx) (Item, Item, error) {
		before, e := optionalJSON(ctx, tx, `SELECT jsonb_build_object('key',key,'is_secret',is_secret) FROM system_settings WHERE key=$1 FOR UPDATE`, key)
		if e != nil {
			return nil, nil, e
		}
		after, e := oneJSON(ctx, tx, `INSERT INTO system_settings(key,value,is_secret,updated_by) VALUES($1,$2::jsonb,$3,$4) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,is_secret=EXCLUDED.is_secret,updated_by=EXCLUDED.updated_by,updated_at=now() RETURNING jsonb_build_object('key',key,'value',CASE WHEN is_secret THEN to_jsonb('***'::text) ELSE value END,'is_secret',is_secret,'updated_at',updated_at,'updated_by',updated_by)`, key, string(encoded), secret, a.AdminID)
		return before, after, e
	}, "setting.updated", "setting", resourceID, a)
}
func (r *PostgresRepository) SetAdminRoles(ctx context.Context, id uuid.UUID, roles []string, a AuditContext) (Item, error) {
	if id == a.AdminID && !contains(roles, "super_admin") {
		return nil, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admins WHERE id=$1)`, id).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	before, err := roleItem(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	var count int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM roles WHERE key=ANY($1::text[])`, roles).Scan(&count)
	if err != nil {
		return nil, err
	}
	if count != len(roles) {
		return nil, ErrInvalidInput
	}
	if _, err = tx.Exec(ctx, `DELETE FROM admin_roles WHERE admin_id=$1`, id); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO admin_roles(admin_id,role_id) SELECT $1,id FROM roles WHERE key=ANY($2::text[])`, id, roles); err != nil {
		return nil, err
	}
	after, err := roleItem(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err = recordAudit(ctx, tx, "admin.roles_updated", "admin", id, before, after, a); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return after, nil
}

func (r *PostgresRepository) CreateProduct(ctx context.Context, input ProductInput, a AuditContext) (Item, error) {
	id := uuid.New()
	name, err := json.Marshal(input.NameI18n)
	if err != nil {
		return nil, ErrInvalidInput
	}
	description, err := json.Marshal(input.Description)
	if err != nil {
		return nil, ErrInvalidInput
	}
	item, err := mutateJSON(ctx, r.pool, func(tx pgx.Tx) (Item, Item, error) {
		after, createErr := oneJSON(ctx, tx, `INSERT INTO products(id,slug,name_i18n,description_i18n,status,sort_order,product_type,featured) VALUES($1,$2,$3::jsonb,$4::jsonb,$5,$6,$7,$8) RETURNING to_jsonb(products)`, id, input.Slug, name, description, input.Status, input.SortOrder, input.ProductType, input.Featured)
		return nil, after, createErr
	}, "product.created", "product", id, a)
	if isConstraintViolation(err) {
		return nil, ErrInvalidInput
	}
	return item, err
}

func (r *PostgresRepository) UpdateProduct(ctx context.Context, id uuid.UUID, input ProductInput, a AuditContext) (Item, error) {
	name, err := json.Marshal(input.NameI18n)
	if err != nil {
		return nil, ErrInvalidInput
	}
	description, err := json.Marshal(input.Description)
	if err != nil {
		return nil, ErrInvalidInput
	}
	item, err := mutateJSON(ctx, r.pool, func(tx pgx.Tx) (Item, Item, error) {
		before, lockErr := oneJSON(ctx, tx, `SELECT to_jsonb(p) FROM products p WHERE id=$1 FOR UPDATE`, id)
		if lockErr != nil {
			return nil, nil, lockErr
		}
		after, updateErr := oneJSON(ctx, tx, `UPDATE products SET slug=$2,name_i18n=$3::jsonb,description_i18n=$4::jsonb,status=$5,sort_order=$6,product_type=$7,featured=$8,updated_at=now() WHERE id=$1 RETURNING to_jsonb(products)`, id, input.Slug, name, description, input.Status, input.SortOrder, input.ProductType, input.Featured)
		return before, after, updateErr
	}, "product.updated", "product", id, a)
	if isConstraintViolation(err) {
		return nil, ErrInvalidInput
	}
	return item, err
}

func (r *PostgresRepository) CreatePlan(ctx context.Context, productID uuid.UUID, input PlanInput, a AuditContext) (Item, error) {
	id := uuid.New()
	name, err := json.Marshal(input.NameI18n)
	if err != nil {
		return nil, ErrInvalidInput
	}
	item, err := mutateJSON(ctx, r.pool, func(tx pgx.Tx) (Item, Item, error) {
		after, createErr := oneJSON(ctx, tx, `INSERT INTO plans(id,product_id,node_group_id,slug,name_i18n,status,cpu_cores,memory_mb,disk_gb,traffic_gb,bandwidth_mbps,ipv4_count,ipv6_count,nat_port_count,virtualization,billing_cycle,price_minor,currency,stock_mode,default_image_id,stock_quantity,setup_fee_minor,traffic_overage_price_minor) VALUES($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23) RETURNING to_jsonb(plans)`, id, productID, input.NodeGroupID, input.Slug, name, input.Status, input.CPUCores, input.MemoryMB, input.DiskGB, input.TrafficGB, input.BandwidthMbps, input.IPv4Count, input.IPv6Count, input.NATPortCount, input.Virtualization, input.BillingCycle, input.PriceMinor, input.Currency, input.StockMode, input.DefaultImageID, input.StockQuantity, input.SetupFeeMinor, input.OverageMinor)
		return nil, after, createErr
	}, "plan.created", "plan", id, a)
	if isConstraintViolation(err) {
		return nil, ErrInvalidInput
	}
	return item, err
}

func (r *PostgresRepository) UpdatePlan(ctx context.Context, id uuid.UUID, input PlanInput, a AuditContext) (Item, error) {
	name, err := json.Marshal(input.NameI18n)
	if err != nil {
		return nil, ErrInvalidInput
	}
	item, err := mutateJSON(ctx, r.pool, func(tx pgx.Tx) (Item, Item, error) {
		before, lockErr := oneJSON(ctx, tx, `SELECT to_jsonb(p) FROM plans p WHERE id=$1 FOR UPDATE`, id)
		if lockErr != nil {
			return nil, nil, lockErr
		}
		after, updateErr := oneJSON(ctx, tx, `UPDATE plans SET node_group_id=$2,slug=$3,name_i18n=$4::jsonb,status=$5,cpu_cores=$6,memory_mb=$7,disk_gb=$8,traffic_gb=$9,bandwidth_mbps=$10,ipv4_count=$11,ipv6_count=$12,nat_port_count=$13,virtualization=$14,billing_cycle=$15,price_minor=$16,currency=$17,stock_mode=$18,default_image_id=$19,stock_quantity=$20,setup_fee_minor=$21,traffic_overage_price_minor=$22,updated_at=now() WHERE id=$1 RETURNING to_jsonb(plans)`, id, input.NodeGroupID, input.Slug, name, input.Status, input.CPUCores, input.MemoryMB, input.DiskGB, input.TrafficGB, input.BandwidthMbps, input.IPv4Count, input.IPv6Count, input.NATPortCount, input.Virtualization, input.BillingCycle, input.PriceMinor, input.Currency, input.StockMode, input.DefaultImageID, input.StockQuantity, input.SetupFeeMinor, input.OverageMinor)
		return before, after, updateErr
	}, "plan.updated", "plan", id, a)
	if isConstraintViolation(err) {
		return nil, ErrInvalidInput
	}
	return item, err
}

func isConstraintViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503" || pgErr.Code == "23514")
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func listJSON(ctx context.Context, q queryer, query string, args ...any) ([]Item, error) {
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item Item
		if err = json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func oneJSON(ctx context.Context, q queryer, query string, args ...any) (Item, error) {
	var raw []byte
	err := q.QueryRow(ctx, query, args...).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var item Item
	if err = json.Unmarshal(raw, &item); err != nil {
		return nil, err
	}
	return item, nil
}
func optionalJSON(ctx context.Context, q queryer, query string, args ...any) (Item, error) {
	item, err := oneJSON(ctx, q, query, args...)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return item, err
}
func mutateJSON(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) (Item, Item, error), action, resource string, id uuid.UUID, a AuditContext) (Item, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	before, after, err := fn(tx)
	if err != nil {
		return nil, err
	}
	if err = recordAudit(ctx, tx, action, resource, id, before, after, a); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return after, nil
}
func recordAudit(ctx context.Context, tx pgx.Tx, action, resource string, id uuid.UUID, before, after any, a AuditContext) error {
	b, e := json.Marshal(before)
	if e != nil {
		return e
	}
	v, e := json.Marshal(after)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO audit_events(id,actor_type,actor_id,action,resource_type,resource_id,before_data,after_data,ip_address,user_agent,request_id,trace_id) VALUES($1,'admin',$2,$3,$4,$5,$6,$7,NULLIF($8,'')::inet,NULLIF($9,''),NULLIF($10,''),NULLIF($11,''))`, uuid.New(), a.AdminID, action, resource, id, b, v, a.IPAddress, a.UserAgent, a.RequestID, a.TraceID)
	return e
}
func roleItem(ctx context.Context, q queryer, id uuid.UUID) (Item, error) {
	return oneJSON(ctx, q, `SELECT jsonb_build_object('id',a.id,'email',a.email,'roles',COALESCE(jsonb_agg(r.key ORDER BY r.key) FILTER(WHERE r.id IS NOT NULL),'[]'::jsonb)) FROM admins a LEFT JOIN admin_roles ar ON ar.admin_id=a.id LEFT JOIN roles r ON r.id=ar.role_id WHERE a.id=$1 GROUP BY a.id`, id)
}
func contains(values []string, wanted string) bool {
	for _, v := range values {
		if v == wanted {
			return true
		}
	}
	return false
}
func init() {
	for key, query := range listQueries {
		if strings.TrimSpace(query) == "" {
			panic(fmt.Sprintf("empty admin query for %s", key))
		}
	}
}
