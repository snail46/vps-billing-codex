package commerce

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

func TestDuplicateWebhookOneHundredTimesCreditsExactlyOnce(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := migrations.Run(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	userID, productID, planID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,email,password_hash,status,locale,timezone) VALUES ($1,$2,'test','active','en-US','UTC')`, userID, "finance-"+userID.String()+"@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO products (id,slug,name_i18n,status) VALUES ($1,$2,'{"en-US":"Integration"}','active')`, productID, "integration-product-"+productID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO plans (id,product_id,slug,name_i18n,status,cpu_cores,memory_mb,disk_gb,virtualization,billing_cycle,price_minor,currency) VALUES ($1,$2,$3,'{"en-US":"Plan"}','active',1,1024,20,'kvm','monthly',1299,'USD')`, planID, productID, "integration-plan-"+planID.String()); err != nil {
		t.Fatal(err)
	}

	repository := NewPostgresRepository(pool)
	order, err := repository.CreateOrder(ctx, userID, planID, 1, "integration-order-idempotency")
	if err != nil {
		t.Fatal(err)
	}
	event := Webhook{EventID: "duplicate-event-" + order.ID.String(), PaymentID: *order.PaymentID, ExternalPaymentID: "fake-external-" + order.ID.String(), Status: "succeeded", AmountMinor: 1299, Currency: "USD"}
	payload, _ := json.Marshal(event)

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
	conflictingEvent := event
	conflictingEvent.EventID = "conflicting-event-" + order.ID.String()
	conflictingEvent.ExternalPaymentID = "different-external-payment"
	conflictingPayload, _ := json.Marshal(conflictingEvent)
	if _, err := repository.CompletePayment(ctx, conflictingEvent, conflictingPayload); !errors.Is(err, ErrPaymentMismatch) {
		t.Fatalf("conflicting completed payment callback error=%v, want ErrPaymentMismatch", err)
	}

	assertCount(t, pool, `SELECT count(*) FROM ledger_transactions WHERE type='payment_capture' AND reference_id=$1`, event.PaymentID, 1)
	assertCount(t, pool, `SELECT count(*) FROM ledger_entries e JOIN ledger_transactions t ON t.id=e.transaction_id WHERE t.reference_id=$1`, event.PaymentID, 2)
	assertCount(t, pool, `SELECT count(*) FROM outbox_events WHERE event_type='payment.succeeded.v1' AND aggregate_id=$1`, event.PaymentID, 1)
	assertCount(t, pool, `SELECT count(*) FROM payment_webhook_receipts WHERE gateway='fake' AND external_event_id=$1 AND processed_at IS NOT NULL`, event.EventID, 1)
	assertCount(t, pool, `SELECT count(*) FROM payments p JOIN orders o ON o.id=p.order_id JOIN invoices i ON i.order_id=o.id WHERE p.id=$1 AND p.status='succeeded' AND o.status='paid' AND i.status='paid'`, event.PaymentID, 1)

	var debit, credit int64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(amount_minor) FILTER (WHERE direction='debit'),0), COALESCE(sum(amount_minor) FILTER (WHERE direction='credit'),0) FROM ledger_entries e JOIN ledger_transactions t ON t.id=e.transaction_id WHERE t.reference_id=$1`, event.PaymentID).Scan(&debit, &credit); err != nil {
		t.Fatal(err)
	}
	if debit != 1299 || credit != 1299 {
		t.Fatalf("ledger debit=%d credit=%d", debit, credit)
	}

	_, err = pool.Exec(ctx, `UPDATE ledger_entries SET amount_minor=1 WHERE transaction_id IN (SELECT id FROM ledger_transactions WHERE reference_id=$1)`, event.PaymentID)
	if err == nil || !stringsContains(err.Error(), "ledger history is immutable") {
		t.Fatalf("ledger UPDATE error = %v", err)
	}
}

func assertCount(t *testing.T, pool *pgxpool.Pool, query string, argument any, want int64) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(context.Background(), query, argument).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("count = %d, want %d for %s", got, want, query)
	}
}

func stringsContains(value, target string) bool {
	for i := 0; i+len(target) <= len(value); i++ {
		if value[i:i+len(target)] == target {
			return true
		}
	}
	return false
}
