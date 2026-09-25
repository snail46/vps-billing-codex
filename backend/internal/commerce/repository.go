package commerce

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
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

func (r *PostgresRepository) BillingProfile(ctx context.Context, userID uuid.UUID) (BillingProfile, error) {
	var value BillingProfile
	err := r.pool.QueryRow(ctx, `SELECT user_id,legal_name,tax_id,country_code,address,updated_at FROM billing_profiles WHERE user_id=$1`, userID).Scan(&value.UserID, &value.LegalName, &value.TaxID, &value.CountryCode, &value.Address, &value.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return BillingProfile{UserID: userID, Address: json.RawMessage(`{}`)}, nil
	}
	return value, err
}

func (r *PostgresRepository) UpdateBillingProfile(ctx context.Context, userID uuid.UUID, input BillingProfile) (BillingProfile, error) {
	if len(input.Address) == 0 {
		input.Address = json.RawMessage(`{}`)
	}
	var value BillingProfile
	err := r.pool.QueryRow(ctx, `INSERT INTO billing_profiles(user_id,legal_name,tax_id,country_code,address) VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id) DO UPDATE SET legal_name=EXCLUDED.legal_name,tax_id=EXCLUDED.tax_id,country_code=EXCLUDED.country_code,address=EXCLUDED.address,updated_at=now() RETURNING user_id,legal_name,tax_id,country_code,address,updated_at`, userID, input.LegalName, input.TaxID, input.CountryCode, input.Address).Scan(&value.UserID, &value.LegalName, &value.TaxID, &value.CountryCode, &value.Address, &value.UpdatedAt)
	return value, err
}

func (r *PostgresRepository) ListCatalog(ctx context.Context) ([]CatalogItem, error) {
	rows, err := r.queries.ListActiveProductsAndPlans(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]CatalogItem, 0, len(rows))
	for _, row := range rows {
		item := CatalogItem{ProductID: row.ProductID, ProductSlug: row.ProductSlug, ProductName: row.ProductNameI18n, Description: row.DescriptionI18n, ProductType: row.ProductType, Featured: row.Featured, PlanID: row.PlanID, PlanSlug: row.PlanSlug, PlanName: row.PlanNameI18n, CPUCores: row.CpuCores, MemoryMB: row.MemoryMb, DiskGB: row.DiskGb, IPv4Count: row.Ipv4Count, IPv6Count: row.Ipv6Count, NATPortCount: row.NatPortCount, Virtualization: row.Virtualization, BillingCycle: row.BillingCycle, PriceMinor: row.PriceMinor, Currency: row.Currency, StockMode: row.StockMode, SetupFeeMinor: row.SetupFeeMinor, OverageMinor: row.TrafficOveragePriceMinor, Region: row.Region.String, Available: row.Available, SharedIPv4: row.ProductType == "nat_vps", PortForward: row.NatPortCount > 0, TrafficMeter: row.TrafficGb.Valid}
		if row.TrafficGb.Valid {
			value := row.TrafficGb.Int64
			item.TrafficGB = &value
		}
		if row.BandwidthMbps.Valid {
			value := row.BandwidthMbps.Int32
			item.BandwidthMbps = &value
		}
		if row.StockQuantity.Valid {
			value := row.StockQuantity.Int32
			item.StockQuantity = &value
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *PostgresRepository) CreateOrder(ctx context.Context, userID, planID uuid.UUID, quantity int32, idempotencyKey string) (Order, error) {
	return r.CreateOrderWithPromotion(ctx, userID, planID, quantity, idempotencyKey, "")
}

func (r *PostgresRepository) CreateOrderWithPromotion(ctx context.Context, userID, planID uuid.UUID, quantity int32, idempotencyKey, promotionCode string) (Order, error) {
	key := pgtype.Text{String: idempotencyKey, Valid: true}
	if existing, err := r.queries.GetOrderByUserIdempotency(ctx, db.GetOrderByUserIdempotencyParams{UserID: userID, IdempotencyKey: key}); err == nil {
		if existing.Kind != "purchase" || existing.SubscriptionID != nil || !r.orderMatchesPurchase(ctx, existing.ID, planID, quantity, promotionCode) {
			return Order{}, ErrIdempotencyConflict
		}
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
	if plan.SetupFeeMinor > math.MaxInt64-plan.PriceMinor {
		return Order{}, ErrInvalidQuantity
	}
	unitTotal := plan.PriceMinor + plan.SetupFeeMinor
	if unitTotal > 0 && int64(quantity) > math.MaxInt64/unitTotal {
		return Order{}, ErrInvalidQuantity
	}
	total := unitTotal * int64(quantity)
	subtotal, discount := total, int64(0)
	var promotionID *uuid.UUID
	var promotionSnapshot []byte
	if promotionCode != "" {
		var id uuid.UUID
		var discountType string
		var value int64
		var currency *string
		err = tx.QueryRow(ctx, `SELECT id,discount_type,discount_value,currency FROM promotions WHERE code=$1 AND status='active' AND (starts_at IS NULL OR starts_at<=now()) AND (ends_at IS NULL OR ends_at>now()) AND (max_redemptions IS NULL OR redemption_count<max_redemptions) FOR UPDATE`, promotionCode).Scan(&id, &discountType, &value, &currency)
		if errors.Is(err, pgx.ErrNoRows) {
			return Order{}, ErrPromotionInvalid
		}
		if err != nil || (discountType == "fixed" && (currency == nil || *currency != plan.Currency)) {
			if err != nil {
				return Order{}, err
			}
			return Order{}, ErrPromotionInvalid
		}
		discount = promotionDiscount(total, value, discountType)
		promotionID = &id
		promotionSnapshot, err = json.Marshal(map[string]any{"id": id, "code": promotionCode, "discount_type": discountType, "discount_value": value, "discount_minor": discount})
		if err != nil {
			return Order{}, err
		}
		total -= discount
	}
	orderID, invoiceID, paymentID := newID(), newID(), newID()
	order, err := queries.CreateOrder(ctx, db.CreateOrderParams{ID: orderID, OrderNo: number("ORD", orderID), UserID: userID, SubtotalMinor: subtotal, DiscountMinor: discount, TotalMinor: total, Currency: plan.Currency, IdempotencyKey: key, Kind: "purchase"})
	if err != nil {
		if isUniqueViolation(err) {
			_ = tx.Rollback(ctx)
			existing, findErr := r.queries.GetOrderByUserIdempotency(ctx, db.GetOrderByUserIdempotencyParams{UserID: userID, IdempotencyKey: key})
			if findErr != nil {
				return Order{}, findErr
			}
			if existing.Kind != "purchase" || existing.SubscriptionID != nil || !r.orderMatchesPurchase(ctx, existing.ID, planID, quantity, promotionCode) {
				return Order{}, ErrIdempotencyConflict
			}
			return r.orderWithPayment(ctx, existing)
		}
		return Order{}, err
	}
	if promotionID != nil {
		if _, err = tx.Exec(ctx, `UPDATE orders SET promotion_snapshot=$2 WHERE id=$1`, orderID, promotionSnapshot); err != nil {
			return Order{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO promotion_redemptions(id,promotion_id,order_id,user_id,discount_minor,snapshot) VALUES($1,$2,$3,$4,$5,$6)`, newID(), *promotionID, orderID, userID, discount, promotionSnapshot); err != nil {
			return Order{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE promotions SET redemption_count=redemption_count+1,updated_at=now() WHERE id=$1`, *promotionID); err != nil {
			return Order{}, err
		}
	}
	productSnapshot, err := json.Marshal(map[string]any{"id": plan.ProductID, "slug": plan.ProductSlug, "name_i18n": json.RawMessage(plan.ProductNameI18n), "description_i18n": json.RawMessage(plan.ProductDescriptionI18n)})
	if err != nil {
		return Order{}, err
	}
	planSnapshot, err := json.Marshal(map[string]any{"id": plan.ID, "slug": plan.Slug, "name_i18n": json.RawMessage(plan.NameI18n), "cpu_cores": plan.CpuCores, "memory_mb": plan.MemoryMb, "disk_gb": plan.DiskGb, "billing_cycle": plan.BillingCycle, "price_minor": plan.PriceMinor, "setup_fee_minor": plan.SetupFeeMinor, "traffic_overage_price_minor": plan.TrafficOveragePriceMinor, "currency": plan.Currency, "virtualization": plan.Virtualization})
	if err != nil {
		return Order{}, err
	}
	if err := queries.CreateOrderItem(ctx, db.CreateOrderItemParams{ID: newID(), OrderID: orderID, ProductID: plan.ProductID, PlanID: plan.ID, Quantity: quantity, UnitPriceMinor: unitTotal, TotalMinor: total, ProductSnapshot: productSnapshot, PlanSnapshot: planSnapshot}); err != nil {
		return Order{}, err
	}
	invoice, err := queries.CreateInvoice(ctx, db.CreateInvoiceParams{ID: invoiceID, InvoiceNo: number("INV", invoiceID), UserID: userID, OrderID: &orderID, AmountMinor: total, Currency: plan.Currency, DueAt: timestamp(time.Now().Add(30 * time.Minute))})
	if err != nil {
		return Order{}, err
	}
	if err := queries.CreateInvoiceItem(ctx, db.CreateInvoiceItemParams{ID: newID(), InvoiceID: invoice.ID, DescriptionI18n: plan.NameI18n, Quantity: quantity, UnitAmountMinor: unitTotal, TotalMinor: total}); err != nil {
		return Order{}, err
	}
	payment, err := queries.CreatePayment(ctx, db.CreatePaymentParams{ID: paymentID, PaymentNo: number("PAY", paymentID), OrderID: orderID, Gateway: fakeGateway, AmountMinor: total, Currency: plan.Currency, IdempotencyKey: "order:" + orderID.String() + ":fake"})
	if err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, err
	}
	return Order{ID: order.ID, OrderNo: order.OrderNo, Status: order.Status, TotalMinor: order.TotalMinor, Currency: order.Currency, Kind: order.Kind, PaymentID: &payment.ID, PaymentStatus: payment.Status}, nil
}

func (r *PostgresRepository) orderMatchesPurchase(ctx context.Context, orderID, planID uuid.UUID, quantity int32, promotionCode string) bool {
	var storedPlan uuid.UUID
	var storedQuantity int32
	var storedCode string
	err := r.pool.QueryRow(ctx, `SELECT oi.plan_id,oi.quantity,COALESCE(o.promotion_snapshot->>'code','') FROM orders o JOIN order_items oi ON oi.order_id=o.id WHERE o.id=$1 ORDER BY oi.created_at LIMIT 1`, orderID).Scan(&storedPlan, &storedQuantity, &storedCode)
	return err == nil && storedPlan == planID && storedQuantity == quantity && storedCode == promotionCode
}

func promotionDiscount(total, value int64, discountType string) int64 {
	if total <= 0 || value <= 0 {
		return 0
	}
	if discountType == "fixed" {
		return min(total, value)
	}
	result := new(big.Int).Mul(big.NewInt(total), big.NewInt(value))
	result.Div(result, big.NewInt(10000))
	if !result.IsInt64() || result.Int64() > total {
		return total
	}
	return result.Int64()
}

func (r *PostgresRepository) CreateRenewalOrder(ctx context.Context, userID, subscriptionID uuid.UUID, idempotencyKey string) (Order, error) {
	key := pgtype.Text{String: idempotencyKey, Valid: true}
	if existing, err := r.queries.GetOrderByUserIdempotency(ctx, db.GetOrderByUserIdempotencyParams{UserID: userID, IdempotencyKey: key}); err == nil {
		if existing.Kind != "renewal" || existing.SubscriptionID == nil || *existing.SubscriptionID != subscriptionID {
			return Order{}, ErrIdempotencyConflict
		}
		order, paymentErr := r.orderWithPayment(ctx, existing)
		return order, paymentErr
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Order{}, err
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	target, err := queries.GetSubscriptionForRenewal(ctx, db.GetSubscriptionForRenewalParams{ID: subscriptionID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrSubscriptionNotFound
	}
	if err != nil {
		return Order{}, err
	}
	if target.Status != "active" && target.Status != "past_due" && target.Status != "suspended" {
		return Order{}, ErrSubscriptionState
	}
	orderID, invoiceID, paymentID := newID(), newID(), newID()
	order, err := queries.CreateOrder(ctx, db.CreateOrderParams{ID: orderID, OrderNo: number("ORD", orderID), UserID: userID, SubtotalMinor: target.PriceMinor, DiscountMinor: 0, TotalMinor: target.PriceMinor, Currency: target.Currency, IdempotencyKey: key, Kind: "renewal", SubscriptionID: &subscriptionID})
	if err != nil {
		if isUniqueViolation(err) {
			_ = tx.Rollback(ctx)
			existing, findErr := r.queries.GetOrderByUserIdempotency(ctx, db.GetOrderByUserIdempotencyParams{UserID: userID, IdempotencyKey: key})
			if findErr != nil {
				return Order{}, findErr
			}
			if existing.Kind != "renewal" || existing.SubscriptionID == nil || *existing.SubscriptionID != subscriptionID {
				return Order{}, ErrIdempotencyConflict
			}
			result, paymentErr := r.orderWithPayment(ctx, existing)
			return result, paymentErr
		}
		return Order{}, err
	}
	productSnapshot, err := json.Marshal(map[string]any{"id": target.ProductID, "slug": target.ProductSlug, "name_i18n": json.RawMessage(target.ProductNameI18n), "description_i18n": json.RawMessage(target.ProductDescriptionI18n)})
	if err != nil {
		return Order{}, err
	}
	planSnapshot, err := json.Marshal(map[string]any{"id": target.PlanID, "slug": target.PlanSlug, "name_i18n": json.RawMessage(target.PlanNameI18n), "billing_cycle": target.BillingCycle, "price_minor": target.PriceMinor, "currency": target.Currency})
	if err != nil {
		return Order{}, err
	}
	if err := queries.CreateOrderItem(ctx, db.CreateOrderItemParams{ID: newID(), OrderID: orderID, ProductID: target.ProductID, PlanID: target.PlanID, Quantity: 1, UnitPriceMinor: target.PriceMinor, TotalMinor: target.PriceMinor, ProductSnapshot: productSnapshot, PlanSnapshot: planSnapshot}); err != nil {
		return Order{}, err
	}
	invoice, err := queries.CreateInvoice(ctx, db.CreateInvoiceParams{ID: invoiceID, InvoiceNo: number("INV", invoiceID), UserID: userID, SubscriptionID: &subscriptionID, OrderID: &orderID, AmountMinor: target.PriceMinor, Currency: target.Currency, DueAt: timestamp(time.Now().Add(30 * time.Minute))})
	if err != nil {
		return Order{}, err
	}
	if err := queries.CreateInvoiceItem(ctx, db.CreateInvoiceItemParams{ID: newID(), InvoiceID: invoice.ID, DescriptionI18n: target.PlanNameI18n, Quantity: 1, UnitAmountMinor: target.PriceMinor, TotalMinor: target.PriceMinor}); err != nil {
		return Order{}, err
	}
	payment, err := queries.CreatePayment(ctx, db.CreatePaymentParams{ID: paymentID, PaymentNo: number("PAY", paymentID), OrderID: orderID, Gateway: fakeGateway, AmountMinor: target.PriceMinor, Currency: target.Currency, IdempotencyKey: "order:" + orderID.String() + ":fake"})
	if err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, err
	}
	return Order{ID: order.ID, OrderNo: order.OrderNo, Status: order.Status, TotalMinor: order.TotalMinor, Currency: order.Currency, Kind: order.Kind, PaymentID: &payment.ID, PaymentStatus: payment.Status, SubscriptionID: &subscriptionID}, nil
}

func (r *PostgresRepository) orderWithPayment(ctx context.Context, order db.Order) (Order, error) {
	payment, err := r.queries.GetPaymentByOrder(ctx, order.ID)
	if err != nil {
		return Order{}, err
	}
	return Order{ID: order.ID, OrderNo: order.OrderNo, Status: order.Status, TotalMinor: order.TotalMinor, Currency: order.Currency, Kind: order.Kind, PaymentID: &payment.ID, PaymentStatus: payment.Status, SubscriptionID: order.SubscriptionID}, nil
}

func (r *PostgresRepository) ListOrders(ctx context.Context, userID uuid.UUID) ([]Order, error) {
	rows, err := r.queries.ListOrdersByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]Order, 0, len(rows))
	for _, row := range rows {
		result = append(result, Order{ID: row.ID, OrderNo: row.OrderNo, Status: row.Status, TotalMinor: row.TotalMinor, Currency: row.Currency, Kind: row.Kind, PaymentID: row.PaymentID, PaymentStatus: row.PaymentStatus.String, SubscriptionID: row.SubscriptionID})
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
		result = append(result, Invoice{ID: row.ID, InvoiceNo: row.InvoiceNo, Status: row.Status, AmountMinor: row.AmountMinor, Currency: row.Currency, DueAt: row.DueAt.Time, SubscriptionID: row.SubscriptionID, OrderID: row.OrderID})
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
	if locked.PaymentAmountMinor != event.AmountMinor || locked.PaymentCurrency != strings.ToUpper(event.Currency) {
		return PaymentResult{}, ErrPaymentMismatch
	}
	if locked.PaymentStatus == "succeeded" || locked.PaymentStatus == "partially_refunded" || locked.PaymentStatus == "refunded" {
		if !locked.GatewayPaymentID.Valid || locked.GatewayPaymentID.String != event.ExternalPaymentID {
			return PaymentResult{}, ErrPaymentMismatch
		}
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
	if err := queries.MarkPaymentSucceeded(ctx, db.MarkPaymentSucceededParams{ID: locked.PaymentID, GatewayPaymentID: text(event.ExternalPaymentID), GatewayPayload: payload}); err != nil {
		return PaymentResult{}, err
	}
	if err := queries.MarkOrderPaid(ctx, locked.OrderID); err != nil {
		return PaymentResult{}, err
	}
	if err := queries.MarkInvoicePaid(ctx, locked.InvoiceID); err != nil {
		return PaymentResult{}, err
	}
	var renewedSubscription *db.Subscription
	if locked.SubscriptionID != nil {
		subscription, lockErr := queries.LockSubscriptionByID(ctx, *locked.SubscriptionID)
		if errors.Is(lockErr, pgx.ErrNoRows) {
			return PaymentResult{}, ErrSubscriptionNotFound
		}
		if lockErr != nil {
			return PaymentResult{}, lockErr
		}
		if subscription.Status != "active" && subscription.Status != "past_due" && subscription.Status != "suspended" {
			return PaymentResult{}, ErrSubscriptionState
		}
		renewed, renewErr := queries.RenewSubscriptionAfterPayment(ctx, db.RenewSubscriptionAfterPaymentParams{ID: subscription.ID, StartedAt: timestamp(time.Now())})
		if renewErr != nil {
			return PaymentResult{}, renewErr
		}
		renewedSubscription = &renewed
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
	if renewedSubscription != nil {
		renewalEventID := newID()
		renewalEnvelope, marshalErr := json.Marshal(map[string]any{"event_id": renewalEventID, "event_type": "subscription.renewed.v1", "occurred_at": time.Now().UTC(), "aggregate_type": "subscription", "aggregate_id": renewedSubscription.ID, "data": map[string]any{"subscription_id": renewedSubscription.ID, "user_id": renewedSubscription.UserID, "payment_id": locked.PaymentID, "current_period_end": renewedSubscription.CurrentPeriodEnd.Time, "status": renewedSubscription.Status, "version": renewedSubscription.Version}})
		if marshalErr != nil {
			return PaymentResult{}, marshalErr
		}
		if err := queries.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{ID: renewalEventID, EventType: "subscription.renewed.v1", AggregateType: "subscription", AggregateID: renewedSubscription.ID, Payload: renewalEnvelope}); err != nil {
			return PaymentResult{}, err
		}
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
