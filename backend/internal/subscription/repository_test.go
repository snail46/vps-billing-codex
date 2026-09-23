package subscription

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "vps-billing/backend/internal/store/sqlc"
)

func TestDecideLifecycleTransition(t *testing.T) {
	now := time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC)
	periodEnd := now.Add(-time.Hour)
	tests := []struct {
		name       string
		current    db.Subscription
		wantStatus string
		wantEvent  string
		want       bool
	}{
		{name: "due enters grace", current: db.Subscription{Status: "active", CurrentPeriodEnd: pgtype.Timestamptz{Time: periodEnd, Valid: true}}, wantStatus: "past_due", wantEvent: "subscription.past_due.v1", want: true},
		{name: "scheduled cancellation wins at period end", current: db.Subscription{Status: "active", CancelAtPeriodEnd: true, CurrentPeriodEnd: pgtype.Timestamptz{Time: periodEnd, Valid: true}}, wantStatus: "cancelled", wantEvent: "subscription.cancelled.v1", want: true},
		{name: "grace expiry suspends", current: db.Subscription{Status: "past_due", GraceUntil: pgtype.Timestamptz{Time: now, Valid: true}}, wantStatus: "suspended", wantEvent: "subscription.suspended.v1", want: true},
		{name: "future period is unchanged", current: db.Subscription{Status: "active", CurrentPeriodEnd: pgtype.Timestamptz{Time: now.Add(time.Hour), Valid: true}}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, ok := decideLifecycleTransition(test.current, now, 72*time.Hour)
			if ok != test.want || decision.status != test.wantStatus || decision.eventType != test.wantEvent {
				t.Fatalf("decision=%#v ok=%v", decision, ok)
			}
			if test.wantStatus == "past_due" && !decision.graceUntil.Time.Equal(periodEnd.Add(72*time.Hour)) {
				t.Fatalf("grace_until=%s", decision.graceUntil.Time)
			}
		})
	}
}
