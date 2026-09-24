package portal

import (
	"context"
	"errors"
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
