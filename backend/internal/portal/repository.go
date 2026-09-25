package portal

import (
	"context"
	"errors"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "vps-billing/backend/internal/store/sqlc"
)

type PostgresRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, queries: db.New(pool)}
}

func (r *PostgresRepository) ListInstances(ctx context.Context, userID uuid.UUID) ([]Instance, error) {
	rows, err := r.queries.ListUserInstances(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]Instance, 0, len(rows))
	for _, row := range rows {
		result = append(result, instanceFromList(row))
	}
	return result, nil
}
func (r *PostgresRepository) GetInstance(ctx context.Context, userID, id uuid.UUID) (Instance, error) {
	row, err := r.queries.GetUserInstance(ctx, db.GetUserInstanceParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Instance{}, ErrNotFound
	}
	if err != nil {
		return Instance{}, err
	}
	return instanceFromGet(row), nil
}
func (r *PostgresRepository) ListNetworks(ctx context.Context, userID, id uuid.UUID) ([]Network, error) {
	if _, err := r.GetInstance(ctx, userID, id); err != nil {
		return nil, err
	}
	rows, err := r.queries.ListUserInstanceNetworks(ctx, db.ListUserInstanceNetworksParams{InstanceID: id, UserID: userID})
	if err != nil {
		return nil, err
	}
	result := make([]Network, 0, len(rows))
	for _, row := range rows {
		var prefix *int32
		if row.Prefix.Valid {
			value := row.Prefix.Int32
			prefix = &value
		}
		result = append(result, Network{ID: row.ID, Type: row.Type, Address: row.Address, Gateway: row.Gateway, Prefix: prefix, CreatedAt: row.CreatedAt.Time.UTC()})
	}
	return result, nil
}
func (r *PostgresRepository) ListTraffic(ctx context.Context, userID, id uuid.UUID) ([]Traffic, error) {
	if _, err := r.GetInstance(ctx, userID, id); err != nil {
		return nil, err
	}
	rows, err := r.queries.ListUserInstanceTraffic(ctx, db.ListUserInstanceTrafficParams{InstanceID: id, UserID: userID})
	if err != nil {
		return nil, err
	}
	result := make([]Traffic, 0, len(rows))
	for _, row := range rows {
		result = append(result, Traffic{PeriodStart: row.PeriodStart.Time.UTC(), PeriodEnd: row.PeriodEnd.Time.UTC(), RXBytes: row.RxBytes, TXBytes: row.TxBytes, Source: row.Source})
	}
	return result, nil
}
func (r *PostgresRepository) UsageSummary(ctx context.Context, userID, id uuid.UUID) (UsageSummary, error) {
	var value UsageSummary
	err := r.pool.QueryRow(ctx, `SELECT s.current_period_start,s.current_period_end,
		CASE WHEN pl.traffic_gb IS NULL THEN 9223372036854775807 ELSE pl.traffic_gb*1073741824 END,
		COALESCE((SELECT sum(us.rx_bytes::numeric+us.tx_bytes::numeric) FROM usage_samples us WHERE us.instance_id=i.id AND us.period_start>=s.current_period_start AND us.period_end<=s.current_period_end),0),
		pl.traffic_overage_price_minor,pl.currency
		FROM instances i JOIN subscriptions s ON s.id=i.subscription_id JOIN plans pl ON pl.id=s.plan_id
		WHERE i.id=$1 AND s.user_id=$2 AND s.current_period_start IS NOT NULL AND s.current_period_end IS NOT NULL`, id, userID).Scan(&value.PeriodStart, &value.PeriodEnd, &value.IncludedBytes, &value.UsedBytes, &value.EstimatedMinor, &value.Currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return UsageSummary{}, ErrNotFound
	}
	if err != nil {
		return UsageSummary{}, err
	}
	value.OverageBytes = max(value.UsedBytes-value.IncludedBytes, 0)
	if value.OverageBytes == 0 {
		value.EstimatedMinor = 0
	} else {
		numerator := new(big.Int).Mul(big.NewInt(value.OverageBytes), big.NewInt(value.EstimatedMinor))
		numerator.Add(numerator, big.NewInt((1<<30)-1)).Div(numerator, big.NewInt(1<<30))
		if numerator.IsInt64() {
			value.EstimatedMinor = numerator.Int64()
		} else {
			value.EstimatedMinor = math.MaxInt64
		}
	}
	value.PeriodStart, value.PeriodEnd = value.PeriodStart.UTC(), value.PeriodEnd.UTC()
	return value, nil
}
func (r *PostgresRepository) ListPortForwards(ctx context.Context, userID, id uuid.UUID) ([]PortForward, error) {
	rows, err := r.pool.Query(ctx, `SELECT pf.id,pf.protocol,host(pf.public_ip),pf.public_port,pf.guest_port,COALESCE(pf.description,''),pf.status,pf.provider_mapping_id,pf.operation_id,pf.error_code,pf.created_at,pf.updated_at
		FROM port_forwards pf JOIN instances i ON i.id=pf.instance_id JOIN subscriptions s ON s.id=i.subscription_id
		WHERE pf.instance_id=$1 AND s.user_id=$2 AND pf.status<>'deleted' ORDER BY pf.created_at`, id, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PortForward{}
	for rows.Next() {
		var item PortForward
		var mapping, errorCode pgtype.Text
		if err = rows.Scan(&item.ID, &item.Protocol, &item.PublicIP, &item.PublicPort, &item.GuestPort, &item.Description, &item.Status, &mapping, &item.OperationID, &errorCode, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.ProviderMappingID, item.ErrorCode = textPtr(mapping), textPtr(errorCode)
		item.CreatedAt, item.UpdatedAt = item.CreatedAt.UTC(), item.UpdatedAt.UTC()
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) PortForwardContext(ctx context.Context, userID, id uuid.UUID) (PortForwardContext, error) {
	var value PortForwardContext
	err := r.pool.QueryRow(ctx, `SELECT i.id,pl.nat_port_count,
		(SELECT count(*) FROM port_forwards pf WHERE pf.instance_id=i.id AND pf.status<>'deleted'),
		COALESCE((p.capabilities->>'port_forward')::boolean,(p.capabilities->>'nat')::boolean,false)
		FROM instances i JOIN subscriptions s ON s.id=i.subscription_id JOIN plans pl ON pl.id=s.plan_id
		JOIN providers p ON p.id=i.provider_id WHERE i.id=$1 AND s.user_id=$2 AND i.deleted_at IS NULL`, id, userID).Scan(&value.InstanceID, &value.Quota, &value.Active, &value.Supported)
	if errors.Is(err, pgx.ErrNoRows) {
		return PortForwardContext{}, ErrNotFound
	}
	return value, err
}

func (r *PostgresRepository) GetPortForward(ctx context.Context, userID, instanceID, id uuid.UUID) (PortForward, error) {
	var item PortForward
	var mapping, errorCode pgtype.Text
	err := r.pool.QueryRow(ctx, `SELECT pf.id,pf.protocol,host(pf.public_ip),pf.public_port,pf.guest_port,COALESCE(pf.description,''),pf.status,pf.provider_mapping_id,pf.operation_id,pf.error_code,pf.created_at,pf.updated_at
		FROM port_forwards pf JOIN instances i ON i.id=pf.instance_id JOIN subscriptions s ON s.id=i.subscription_id
		WHERE pf.id=$1 AND pf.instance_id=$2 AND s.user_id=$3 AND pf.status<>'deleted'`, id, instanceID, userID).Scan(&item.ID, &item.Protocol, &item.PublicIP, &item.PublicPort, &item.GuestPort, &item.Description, &item.Status, &mapping, &item.OperationID, &errorCode, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PortForward{}, ErrNotFound
	}
	if err != nil {
		return PortForward{}, err
	}
	item.ProviderMappingID, item.ErrorCode = textPtr(mapping), textPtr(errorCode)
	item.CreatedAt, item.UpdatedAt = item.CreatedAt.UTC(), item.UpdatedAt.UTC()
	return item, nil
}
func (r *PostgresRepository) ActionContext(ctx context.Context, userID, id uuid.UUID) (ActionContext, error) {
	row, err := r.queries.GetUserInstanceActionContext(ctx, db.GetUserInstanceActionContextParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ActionContext{}, ErrNotFound
	}
	if err != nil {
		return ActionContext{}, err
	}
	if row.ProviderID == nil || row.NodeID == nil || !row.ProviderInstanceID.Valid {
		return ActionContext{}, ErrStateConflict
	}
	return ActionContext{InstanceID: row.ID, ProviderID: *row.ProviderID, ProviderInstanceID: row.ProviderInstanceID.String, NodeID: *row.NodeID, ProviderNodeID: row.ProviderNodeID.String, ImageID: row.ImageID.String, DesiredState: row.DesiredState, ObservedState: row.ObservedState}, nil
}
func (r *PostgresRepository) ListNotifications(ctx context.Context, userID uuid.UUID) ([]Notification, error) {
	rows, err := r.queries.ListUserNotifications(ctx, &userID)
	if err != nil {
		return nil, err
	}
	result := make([]Notification, 0, len(rows))
	for _, row := range rows {
		result = append(result, Notification{ID: row.ID, Type: row.Type, TitleKey: row.TitleKey, MessageKey: row.MessageKey, Parameters: row.Parameters, Severity: row.Severity, ReadAt: timePtr(row.ReadAt), CreatedAt: row.CreatedAt.Time.UTC()})
	}
	return result, nil
}
func (r *PostgresRepository) MarkNotificationRead(ctx context.Context, userID, id uuid.UUID) error {
	count, err := r.queries.MarkUserNotificationRead(ctx, db.MarkUserNotificationReadParams{ID: id, UserID: &userID})
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
func (r *PostgresRepository) ListTickets(ctx context.Context, userID uuid.UUID) ([]Ticket, error) {
	rows, err := r.queries.ListUserTickets(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]Ticket, 0, len(rows))
	for _, row := range rows {
		result = append(result, ticket(row.ID, row.TicketNo, row.Subject, row.Status, row.Priority, row.CreatedAt.Time, row.UpdatedAt.Time, row.ClosedAt))
	}
	return result, nil
}
func (r *PostgresRepository) GetTicket(ctx context.Context, userID, id uuid.UUID) (Ticket, error) {
	row, err := r.queries.GetUserTicket(ctx, db.GetUserTicketParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}
	if err != nil {
		return Ticket{}, err
	}
	result := ticket(row.ID, row.TicketNo, row.Subject, row.Status, row.Priority, row.CreatedAt.Time, row.UpdatedAt.Time, row.ClosedAt)
	messages, err := r.queries.ListUserTicketMessages(ctx, db.ListUserTicketMessagesParams{TicketID: id, UserID: userID})
	if err != nil {
		return Ticket{}, err
	}
	result.Messages = make([]TicketMessage, 0, len(messages))
	for _, message := range messages {
		result.Messages = append(result.Messages, TicketMessage{ID: message.ID, SenderType: message.SenderType, Message: message.Message, CreatedAt: message.CreatedAt.Time.UTC()})
	}
	return result, nil
}
func (r *PostgresRepository) CreateTicket(ctx context.Context, userID uuid.UUID, subject, priority, message string) (Ticket, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Ticket{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	id := newID()
	row, err := q.CreateUserTicket(ctx, db.CreateUserTicketParams{ID: id, TicketNo: number("TKT", id), UserID: userID, Subject: subject, Priority: priority})
	if err != nil {
		return Ticket{}, err
	}
	if _, err = q.CreateUserTicketMessage(ctx, db.CreateUserTicketMessageParams{ID: newID(), TicketID: id, SenderID: &userID, Message: message}); err != nil {
		return Ticket{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Ticket{}, err
	}
	return ticket(row.ID, row.TicketNo, row.Subject, row.Status, row.Priority, row.CreatedAt.Time, row.UpdatedAt.Time, row.ClosedAt), nil
}
func (r *PostgresRepository) AddTicketMessage(ctx context.Context, userID, ticketID uuid.UUID, message string) (TicketMessage, error) {
	target, err := r.queries.GetUserTicket(ctx, db.GetUserTicketParams{ID: ticketID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return TicketMessage{}, ErrNotFound
	} else if err != nil {
		return TicketMessage{}, err
	}
	if target.Status == "closed" || target.Status == "resolved" {
		return TicketMessage{}, ErrTicketClosed
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return TicketMessage{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	row, err := q.CreateUserTicketMessage(ctx, db.CreateUserTicketMessageParams{ID: newID(), TicketID: ticketID, SenderID: &userID, Message: message})
	if err != nil {
		return TicketMessage{}, err
	}
	if err = q.TouchUserTicket(ctx, db.TouchUserTicketParams{ID: ticketID, UserID: userID}); err != nil {
		return TicketMessage{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return TicketMessage{}, err
	}
	return TicketMessage{ID: row.ID, SenderType: row.SenderType, Message: row.Message, CreatedAt: row.CreatedAt.Time.UTC()}, nil
}

func instanceFromList(row db.ListUserInstancesRow) Instance {
	return instance(row.ID, row.Name, row.DesiredState, row.ObservedState, row.CpuCores, row.MemoryMb, row.DiskGb, int64Ptr(row.TrafficLimitGb), int32Ptr(row.BandwidthMbps), textPtr(row.ImageID), row.PrimaryIpv4, row.PrimaryIpv6, timePtr(row.LastSyncedAt), row.CreatedAt.Time, row.UpdatedAt.Time, row.SubscriptionID, row.SubscriptionStatus, timePtr(row.CurrentPeriodEnd), row.PlanSlug, row.PlanNameI18n)
}
func instanceFromGet(row db.GetUserInstanceRow) Instance {
	return instance(row.ID, row.Name, row.DesiredState, row.ObservedState, row.CpuCores, row.MemoryMb, row.DiskGb, int64Ptr(row.TrafficLimitGb), int32Ptr(row.BandwidthMbps), textPtr(row.ImageID), row.PrimaryIpv4, row.PrimaryIpv6, timePtr(row.LastSyncedAt), row.CreatedAt.Time, row.UpdatedAt.Time, row.SubscriptionID, row.SubscriptionStatus, timePtr(row.CurrentPeriodEnd), row.PlanSlug, row.PlanNameI18n)
}
func instance(id uuid.UUID, name, desired, observed string, cpu float64, memory, disk int32, traffic *int64, bandwidth *int32, image *string, ipv4, ipv6 string, synced *time.Time, created, updated time.Time, subscriptionID uuid.UUID, subscriptionStatus string, periodEnd *time.Time, planSlug string, planName []byte) Instance {
	return Instance{ID: id, Name: name, DesiredState: desired, ObservedState: observed, CPUCores: cpu, MemoryMB: memory, DiskGB: disk, TrafficLimitGB: traffic, BandwidthMbps: bandwidth, ImageID: image, PrimaryIPv4: ipv4, PrimaryIPv6: ipv6, LastSyncedAt: synced, CreatedAt: created.UTC(), UpdatedAt: updated.UTC(), SubscriptionID: subscriptionID, SubscriptionStatus: subscriptionStatus, CurrentPeriodEnd: periodEnd, PlanSlug: planSlug, PlanName: planName}
}
func ticket(id uuid.UUID, no, subject, status, priority string, created, updated time.Time, closed pgtype.Timestamptz) Ticket {
	return Ticket{ID: id, TicketNo: no, Subject: subject, Status: status, Priority: priority, CreatedAt: created.UTC(), UpdatedAt: updated.UTC(), ClosedAt: timePtr(closed)}
}
func timePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}
func int64Ptr(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
func int32Ptr(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	result := value.Int32
	return &result
}
func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
func newID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New()
	}
	return id
}
func number(prefix string, id uuid.UUID) string {
	return prefix + "-" + strings.ToUpper(strings.ReplaceAll(id.String(), "-", ""))[:20]
}
