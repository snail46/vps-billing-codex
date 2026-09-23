package commerce

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "vps-billing/backend/internal/store/sqlc"
)

const fakeGateway = "fake"

type PostgresRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, queries: db.New(pool)}
}

func (r *PostgresRepository) ListCatalog(ctx context.Context) ([]CatalogItem, error) {
	rows, err := r.queries.ListActiveProductsAndPlans(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]CatalogItem, 0, len(rows))
	for _, row := range rows {
		item := CatalogItem{ProductID: row.ProductID, ProductSlug: row.ProductSlug, ProductName: row.ProductNameI18n, Description: row.DescriptionI18n, PlanID: row.PlanID, PlanSlug: row.PlanSlug, PlanName: row.PlanNameI18n, MemoryMB: row.MemoryMb, DiskGB: row.DiskGb, IPv4Count: row.Ipv4Count, IPv6Count: row.Ipv6Count, NATPortCount: row.NatPortCount, Virtualization: row.Virtualization, BillingCycle: row.BillingCycle, PriceMinor: row.PriceMinor, Currency: row.Currency}
		if row.TrafficGb.Valid {
			value := row.TrafficGb.Int64
			item.TrafficGB = &value
		}
		if row.BandwidthMbps.Valid {
			value := row.BandwidthMbps.Int32
			item.BandwidthMbps = &value
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *PostgresRepository) CreateOrder(ctx context.Context, userID, planID uuid.UUID, quantity int32, idempotencyKey string) (Order, error) {
	key := pgtype.Text{String: idempotencyKey, Valid: true}
	if existing, err := r.queries.GetOrderByUserIdempotency(ctx, db.GetOrderByUserIdempotencyParams{UserID: userID, IdempotencyKey: key}); err == nil {
		return r.orderWithPayment(ctx, existing)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Order{}, err
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	plan, err := queries.GetPlanForOrder(ctx, planID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrPlanNotFound
	}
	if err != nil {
		return Order{}, err
	}
	if plan.PriceMinor > 0 && int64(quantity) > math.MaxInt64/plan.PriceMinor {
		return Order{}, ErrInvalidQuantity
	}
	total := plan.PriceMinor * int64(quantity)
	orderID, invoiceID, paymentID := newID(), newID(), newID()
	order, err := queries.CreateOrder(ctx, db.CreateOrderParams{ID: orderID, OrderNo: number("ORD", orderID), UserID: userID, SubtotalMinor: total, Currency: plan.Currency, IdempotencyKey: key})
	if err != nil {
		if isUniqueViolation(err) {
			_ = tx.Rollback(ctx)
			existing, findErr := r.queries.GetOrderByUserIdempotency(ctx, db.GetOrderByUserIdempotencyParams{UserID: userID, IdempotencyKey: key})
			if findErr != nil {
				return Order{}, findErr
			}
			return r.orderWithPayment(ctx, existing)
		}
		return Order{}, err
	}
	productSnapshot, err := json.Marshal(map[string]any{"id": plan.ProductID, "slug": plan.ProductSlug, "name_i18n": json.RawMessage(plan.ProductNameI18n), "description_i18n": json.RawMessage(plan.ProductDescriptionI18n)})
	if err != nil {
		return Order{}, err
	}
	planSnapshot, err := json.Marshal(map[string]any{"id": plan.ID, "slug": plan.Slug, "name_i18n": json.RawMessage(plan.NameI18n), "memory_mb": plan.MemoryMb, "disk_gb": plan.DiskGb, "billing_cycle": plan.BillingCycle, "price_minor": plan.PriceMinor, "currency": plan.Currency, "virtualization": plan.Virtualization})
	if err != nil {
		return Order{}, err
	}
	if err := queries.CreateOrderItem(ctx, db.CreateOrderItemParams{ID: newID(), OrderID: orderID, ProductID: plan.ProductID, PlanID: plan.ID, Quantity: quantity, UnitPriceMinor: plan.PriceMinor, TotalMinor: total, ProductSnapshot: productSnapshot, PlanSnapshot: planSnapshot}); err != nil {
		return Order{}, err
	}
	invoice, err := queries.CreateInvoice(ctx, db.CreateInvoiceParams{ID: invoiceID, InvoiceNo: number("INV", invoiceID), UserID: userID, OrderID: &orderID, AmountMinor: total, Currency: plan.Currency, DueAt: timestamp(time.Now().Add(30 * time.Minute))})
	if err != nil {
		return Order{}, err
	}
	if err := queries.CreateInvoiceItem(ctx, db.CreateInvoiceItemParams{ID: newID(), InvoiceID: invoice.ID, DescriptionI18n: plan.NameI18n, Quantity: quantity, UnitAmountMinor: plan.PriceMinor, TotalMinor: total}); err != nil {
		return Order{}, err
	}
	payment, err := queries.CreatePayment(ctx, db.CreatePaymentParams{ID: paymentID, PaymentNo: number("PAY", paymentID), OrderID: orderID, Gateway: fakeGateway, AmountMinor: total, Currency: plan.Currency, IdempotencyKey: "order:" + orderID.String() + ":fake"})
	if err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, err
	}
	return Order{ID: order.ID, OrderNo: order.OrderNo, Status: order.Status, TotalMinor: order.TotalMinor, Currency: order.Currency, PaymentID: &payment.ID, PaymentStatus: payment.Status}, nil
}

func (r *PostgresRepository) orderWithPayment(ctx context.Context, order db.Order) (Order, error) {
	payment, err := r.queries.GetPaymentByOrder(ctx, order.ID)
	if err != nil {
		return Order{}, err
	}
	return Order{ID: order.ID, OrderNo: order.OrderNo, Status: order.Status, TotalMinor: order.TotalMinor, Currency: order.Currency, PaymentID: &payment.ID, PaymentStatus: payment.Status}, nil
}

func (r *PostgresRepository) ListOrders(ctx context.Context, userID uuid.UUID) ([]Order, error) {
	rows, err := r.queries.ListOrdersByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]Order, 0, len(rows))
	for _, row := range rows {
		result = append(result, Order{ID: row.ID, OrderNo: row.OrderNo, Status: row.Status, TotalMinor: row.TotalMinor, Currency: row.Currency, PaymentID: row.PaymentID, PaymentStatus: row.PaymentStatus.String})
	}
	return result, nil
}

func (r *PostgresRepository) ListInvoices(ctx context.Context, userID uuid.UUID) ([]Invoice, error) {
	rows, err := r.queries.ListInvoicesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]Invoice, 0, len(rows))
	for _, row := range rows {
		result = append(result, Invoice{ID: row.ID, InvoiceNo: row.InvoiceNo, Status: row.Status, AmountMinor: row.AmountMinor, Currency: row.Currency, DueAt: row.DueAt.Time})
	}
	return result, nil
}

func (r *PostgresRepository) EnsureWallet(ctx context.Context, userID uuid.UUID, currency string) (Wallet, error) {
	currency = strings.ToUpper(currency)
	if len(currency) != 3 {
		return Wallet{}, ErrPaymentMismatch
	}
	row, err := r.queries.EnsureWallet(ctx, db.EnsureWalletParams{ID: newID(), UserID: userID, Currency: currency})
	return Wallet{ID: row.ID, Currency: row.Currency, AvailableBalanceMinor: row.AvailableBalanceMinor}, err
}

func (r *PostgresRepository) CompletePayment(ctx context.Context, event Webhook, payload json.RawMessage) (PaymentResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PaymentResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	receiptID, err := queries.InsertWebhookReceipt(ctx, db.InsertWebhookReceiptParams{ID: newID(), Gateway: fakeGateway, ExternalEventID: event.EventID, Payload: payload})
	if errors.Is(err, pgx.ErrNoRows) {
		receipt, receiptErr := queries.GetWebhookReceipt(ctx, db.GetWebhookReceiptParams{Gateway: fakeGateway, ExternalEventID: event.EventID})
		if receiptErr != nil {
			return PaymentResult{}, receiptErr
		}
		if !equalJSON(receipt.Payload, payload) {
			return PaymentResult{}, ErrPaymentMismatch
		}
		locked, findErr := queries.LockPaymentOrderInvoice(ctx, db.LockPaymentOrderInvoiceParams{ID: event.PaymentID, Gateway: fakeGateway})
		if errors.Is(findErr, pgx.ErrNoRows) {
			return PaymentResult{}, ErrPaymentNotFound
		}
		if findErr != nil {
			return PaymentResult{}, findErr
		}
		return PaymentResult{PaymentID: locked.PaymentID, OrderID: locked.OrderID, Duplicate: true}, nil
	}
	if err != nil {
		return PaymentResult{}, err
	}
	locked, err := queries.LockPaymentOrderInvoice(ctx, db.LockPaymentOrderInvoiceParams{ID: event.PaymentID, Gateway: fakeGateway})
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentResult{}, ErrPaymentNotFound
	}
	if err != nil {
		return PaymentResult{}, err
	}
	if locked.PaymentStatus == "succeeded" {
		if err := queries.MarkWebhookProcessed(ctx, receiptID); err != nil {
			return PaymentResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return PaymentResult{}, err
		}
		return PaymentResult{PaymentID: locked.PaymentID, OrderID: locked.OrderID, Duplicate: true}, nil
	}
	if locked.PaymentStatus != "pending" && locked.PaymentStatus != "processing" {
		return PaymentResult{}, ErrPaymentState
	}
	if locked.PaymentAmountMinor != event.AmountMinor || locked.PaymentCurrency != strings.ToUpper(event.Currency) {
		return PaymentResult{}, ErrPaymentMismatch
	}
	if err := queries.MarkPaymentSucceeded(ctx, db.MarkPaymentSucceededParams{ID: locked.PaymentID, GatewayPaymentID: text(event.ExternalPaymentID), GatewayPayload: payload}); err != nil {
		return PaymentResult{}, err
	}
	if err := queries.MarkOrderPaid(ctx, locked.OrderID); err != nil {
		return PaymentResult{}, err
	}
	if err := queries.MarkInvoicePaid(ctx, locked.InvoiceID); err != nil {
		return PaymentResult{}, err
	}
	transactionID := newID()
	if err := queries.CreateLedgerTransaction(ctx, db.CreateLedgerTransactionParams{ID: transactionID, Type: "payment_capture", ReferenceType: text("payment"), ReferenceID: &locked.PaymentID, Description: text("External payment captured")}); err != nil {
		return PaymentResult{}, err
	}
	entries := []db.CreateLedgerEntryParams{
		{ID: newID(), TransactionID: transactionID, AccountType: "gateway_cash", AccountID: locked.PaymentID, Direction: "debit", AmountMinor: event.AmountMinor, Currency: locked.PaymentCurrency},
		{ID: newID(), TransactionID: transactionID, AccountType: "accounts_receivable", AccountID: locked.InvoiceID, Direction: "credit", AmountMinor: event.AmountMinor, Currency: locked.PaymentCurrency},
	}
	if err := validateBalanced(entries); err != nil {
		return PaymentResult{}, err
	}
	for _, entry := range entries {
		if err := queries.CreateLedgerEntry(ctx, entry); err != nil {
			return PaymentResult{}, err
		}
	}
	outboxID := newID()
	envelope, err := json.Marshal(map[string]any{"event_id": outboxID, "event_type": "payment.succeeded.v1", "occurred_at": time.Now().UTC(), "aggregate_type": "payment", "aggregate_id": locked.PaymentID, "data": map[string]any{"payment_id": locked.PaymentID, "order_id": locked.OrderID, "user_id": locked.UserID, "amount_minor": event.AmountMinor, "currency": locked.PaymentCurrency}})
	if err != nil {
		return PaymentResult{}, err
	}
	if err := queries.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{ID: outboxID, EventType: "payment.succeeded.v1", AggregateType: "payment", AggregateID: locked.PaymentID, Payload: envelope}); err != nil {
		return PaymentResult{}, err
	}
	if err := queries.MarkWebhookProcessed(ctx, receiptID); err != nil {
		return PaymentResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentResult{}, err
	}
	return PaymentResult{PaymentID: locked.PaymentID, OrderID: locked.OrderID}, nil
}

func equalJSON(left, right []byte) bool {
	leftCanonical, err := canonicalJSON(left)
	if err != nil {
		return false
	}
	rightCanonical, err := canonicalJSON(right)
	if err != nil {
		return false
	}
	return bytes.Equal(leftCanonical, rightCanonical)
}

func canonicalJSON(payload []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return json.Marshal(value)
}

func validateBalanced(entries []db.CreateLedgerEntryParams) error {
	balances := make(map[string]int64)
	for _, entry := range entries {
		if entry.AmountMinor < 0 {
			return errors.New("ledger amount cannot be negative")
		}
		if entry.Direction == "debit" {
			balances[entry.Currency] += entry.AmountMinor
		} else if entry.Direction == "credit" {
			balances[entry.Currency] -= entry.AmountMinor
		} else {
			return errors.New("ledger direction is invalid")
		}
	}
	for currency, balance := range balances {
		if balance != 0 {
			return fmt.Errorf("ledger is unbalanced for %s", currency)
		}
	}
	return nil
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
func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func text(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
