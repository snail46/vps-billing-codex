package subscription

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

	"vps-billing/backend/internal/commerce"
	"vps-billing/backend/internal/migrations"
)

func TestRenewalPaymentExtendsSubscriptionExactlyOnce(t *testing.T) {
	pool, ctx := integrationPool(t)
	userID, _, planID := seedPlan(t, ctx, pool)
	periodStart := time.Now().UTC().Add(-20 * 24 * time.Hour).Truncate(time.Second)
	periodEnd := periodStart.AddDate(0, 1, 0)
	subscriptionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO subscriptions (id,user_id,plan_id,status,billing_cycle,price_minor,currency,started_at,current_period_start,current_period_end,next_due_at) VALUES ($1,$2,$3,'active','monthly',1299,'USD',$4,$4,$5,$5)`, subscriptionID, userID, planID, periodStart, periodEnd); err != nil {
		t.Fatal(err)
	}

	repository := commerce.NewPostgresRepository(pool)
	order, err := repository.CreateRenewalOrder(ctx, userID, subscriptionID, "renewal-integration-"+subscriptionID.String())
	if err != nil {
		t.Fatal(err)
	}
	repeatedOrder, err := repository.CreateRenewalOrder(ctx, userID, subscriptionID, "renewal-integration-"+subscriptionID.String())
	if err != nil || repeatedOrder.ID != order.ID || repeatedOrder.Kind != "renewal" {
		t.Fatalf("idempotent renewal order=%#v error=%v", repeatedOrder, err)
	}
	otherSubscriptionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO subscriptions (id,user_id,plan_id,status,billing_cycle,price_minor,currency,started_at,current_period_start,current_period_end,next_due_at) VALUES ($1,$2,$3,'active','monthly',1299,'USD',$4,$4,$5,$5)`, otherSubscriptionID, userID, planID, periodStart, periodEnd); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateRenewalOrder(ctx, userID, otherSubscriptionID, "renewal-integration-"+subscriptionID.String()); !errors.Is(err, commerce.ErrIdempotencyConflict) {
		t.Fatalf("cross-subscription idempotency error=%v", err)
	}
	event := commerce.Webhook{EventID: "renewal-event-" + order.ID.String(), PaymentID: *order.PaymentID, ExternalPaymentID: "renewal-external-" + order.ID.String(), Status: "succeeded", AmountMinor: 1299, Currency: "USD"}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	errorsChannel := make(chan error, 100)
	for range 100 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, completeErr := repository.CompletePayment(ctx, event, payload)
			errorsChannel <- completeErr
		}()
	}
	wait.Wait()
	close(errorsChannel)
	for completeErr := range errorsChannel {
		if completeErr != nil {
			t.Errorf("CompletePayment() error = %v", completeErr)
		}
	}
	if t.Failed() {
		return
	}

	var status string
	var renewedEnd time.Time
	var version int64
	if err := pool.QueryRow(ctx, `SELECT status,current_period_end,version FROM subscriptions WHERE id=$1`, subscriptionID).Scan(&status, &renewedEnd, &version); err != nil {
		t.Fatal(err)
	}
	expectedEnd := periodEnd.AddDate(0, 1, 0)
	if status != "active" || !renewedEnd.Equal(expectedEnd) || version != 2 {
		t.Fatalf("subscription status=%s end=%s version=%d, want active %s version=2", status, renewedEnd, version, expectedEnd)
	}
	assertDatabaseCount(t, pool, `SELECT count(*) FROM ledger_transactions WHERE type='payment_capture' AND reference_id=$1`, event.PaymentID, 1)
	assertDatabaseCount(t, pool, `SELECT count(*) FROM outbox_events WHERE event_type='subscription.renewed.v1' AND aggregate_id=$1`, subscriptionID, 1)
}

func TestLifecycleDueGraceSuspendAndCancel(t *testing.T) {
	pool, ctx := integrationPool(t)
	userID, _, planID := seedPlan(t, ctx, pool)
	now := time.Now().UTC().Truncate(time.Second)
	pastDueID, cancelID := uuid.New(), uuid.New()
	for _, fixture := range []struct {
		id     uuid.UUID
		cancel bool
	}{{pastDueID, false}, {cancelID, false}} {
		if _, err := pool.Exec(ctx, `INSERT INTO subscriptions (id,user_id,plan_id,status,billing_cycle,price_minor,currency,started_at,current_period_start,current_period_end,next_due_at,cancel_at_period_end) VALUES ($1,$2,$3,'active','monthly',1299,'USD',$4,$4,$5,$5,$6)`, fixture.id, userID, planID, now.AddDate(0, -1, 0), now.Add(-time.Minute), fixture.cancel); err != nil {
			t.Fatal(err)
		}
	}
	repository := NewPostgresRepository(pool)
	if _, err := repository.SetCancelAtPeriodEnd(ctx, userID, cancelID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SetCancelAtPeriodEnd(ctx, uuid.New(), cancelID, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ownership error=%v, want ErrNotFound", err)
	}

	processor := NewLifecycleProcessor(pool, 72*time.Hour)
	processor.now = func() time.Time { return now }
	if count, err := processor.ProcessBatch(ctx); err != nil || count < 2 {
		t.Fatalf("first ProcessBatch() count=%d error=%v", count, err)
	}
	assertStatus(t, pool, pastDueID, "past_due")
	assertStatus(t, pool, cancelID, "cancelled")

	processor.now = func() time.Time { return now.Add(73 * time.Hour) }
	if _, err := processor.ProcessBatch(ctx); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, pool, pastDueID, "suspended")
	assertDatabaseCount(t, pool, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND event_type IN ('subscription.past_due.v1','subscription.suspended.v1')`, pastDueID, 2)
	assertDatabaseCount(t, pool, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND event_type='subscription.cancelled.v1'`, cancelID, 1)
	assertDatabaseCount(t, pool, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND event_type='subscription.cancel_scheduled.v1'`, cancelID, 1)
}

func integrationPool(t *testing.T) (*pgxpool.Pool, context.Context) {
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

func seedPlan(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	userID, productID, planID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,email,password_hash,status,locale,timezone) VALUES ($1,$2,'test','active','en-US','UTC')`, userID, "subscription-"+userID.String()+"@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO products (id,slug,name_i18n,status) VALUES ($1,$2,'{"en-US":"Subscription"}','active')`, productID, "subscription-product-"+productID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO plans (id,product_id,slug,name_i18n,status,cpu_cores,memory_mb,disk_gb,virtualization,billing_cycle,price_minor,currency) VALUES ($1,$2,$3,'{"en-US":"Monthly"}','active',1,1024,20,'kvm','monthly',1299,'USD')`, planID, productID, "subscription-plan-"+planID.String()); err != nil {
		t.Fatal(err)
	}
	return userID, productID, planID
}

func assertStatus(t *testing.T, pool *pgxpool.Pool, subscriptionID uuid.UUID, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM subscriptions WHERE id=$1`, subscriptionID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("subscription status=%s, want %s", got, want)
	}
}

func assertDatabaseCount(t *testing.T, pool *pgxpool.Pool, query string, argument any, want int64) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(context.Background(), query, argument).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("count=%d, want %d for %s", got, want, query)
	}
}
