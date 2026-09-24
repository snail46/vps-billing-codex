package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "vps-billing/backend/internal/store/sqlc"
)

type Event struct {
	ActorType    string
	ActorID      *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	Before       any
	After        any
	IPAddress    string
	UserAgent    string
	RequestID    string
	TraceID      string
}

type Recorder interface {
	Record(context.Context, Event) error
}

type PostgresRecorder struct{ queries *db.Queries }

func NewPostgresRecorder(pool *pgxpool.Pool) *PostgresRecorder {
	return &PostgresRecorder{queries: db.New(pool)}
}

func (r *PostgresRecorder) Record(ctx context.Context, event Event) error {
	before, err := marshal(event.Before)
	if err != nil {
		return fmt.Errorf("marshal audit before data: %w", err)
	}
	after, err := marshal(event.After)
	if err != nil {
		return fmt.Errorf("marshal audit after data: %w", err)
	}
	var ip *netip.Addr
	if parsed, parseErr := netip.ParseAddr(event.IPAddress); parseErr == nil {
		ip = &parsed
	}
	return r.queries.CreateAuditEvent(ctx, db.CreateAuditEventParams{
		ID: uuid.New(), ActorType: event.ActorType, ActorID: event.ActorID, Action: event.Action,
		ResourceType: event.ResourceType, ResourceID: event.ResourceID, BeforeData: before, AfterData: after,
		IpAddress: ip, UserAgent: text(event.UserAgent), RequestID: text(event.RequestID), TraceID: text(event.TraceID),
	})
}

func marshal(value any) ([]byte, error) {
	if value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(value)
}

func text(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }
