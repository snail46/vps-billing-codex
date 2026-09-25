package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	providercontract "vps-billing/backend/internal/provider"
)

const (
	collectInterval = 5 * time.Minute
	providerTimeout = 5 * time.Second
	gib             = int64(1 << 30)
)

type Processor struct {
	pool      *pgxpool.Pool
	providers providercontract.Resolver
	now       func() time.Time
}

func New(pool *pgxpool.Pool, providers providercontract.Resolver) *Processor {
	return &Processor{pool: pool, providers: providers, now: time.Now}
}

func (p *Processor) ProcessBatch(ctx context.Context) (int, error) {
	collected, collectErr := p.collect(ctx)
	rated, rateErr := p.rate(ctx)
	return collected + rated, errors.Join(collectErr, rateErr)
}

type collectionTarget struct {
	instanceID, providerID, subscriptionID uuid.UUID
	providerInstanceID, providerNodeID     string
	periodStart, periodEnd                 time.Time
	includedGB                             *int64
	overagePrice                           int64
	currency                               string
}

func (p *Processor) collect(ctx context.Context) (int, error) {
	now := p.now().UTC()
	rows, err := p.pool.Query(ctx, `SELECT i.id,i.provider_id,i.provider_instance_id,COALESCE(n.provider_node_id,''),s.id,s.current_period_start,s.current_period_end,pl.traffic_gb,pl.traffic_overage_price_minor,pl.currency
		FROM instances i JOIN subscriptions s ON s.id=i.subscription_id JOIN plans pl ON pl.id=s.plan_id LEFT JOIN nodes n ON n.id=i.node_id
		WHERE i.observed_state IN ('running','stopped','suspended') AND i.deleted_at IS NULL AND i.provider_id IS NOT NULL AND i.provider_instance_id IS NOT NULL
		AND s.current_period_start IS NOT NULL AND s.current_period_end IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM usage_samples us WHERE us.instance_id=i.id AND us.period_end>$1)
		ORDER BY COALESCE((SELECT max(us.period_end) FROM usage_samples us WHERE us.instance_id=i.id),'-infinity') LIMIT 10`, now.Add(-collectInterval))
	if err != nil {
		return 0, err
	}
	targets := []collectionTarget{}
	for rows.Next() {
		var value collectionTarget
		if err = rows.Scan(&value.instanceID, &value.providerID, &value.providerInstanceID, &value.providerNodeID, &value.subscriptionID, &value.periodStart, &value.periodEnd, &value.includedGB, &value.overagePrice, &value.currency); err != nil {
			rows.Close()
			return 0, err
		}
		targets = append(targets, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	var resultErr error
	bucketEnd := now.Truncate(collectInterval)
	if bucketEnd.Equal(now) {
		bucketEnd = bucketEnd.Add(-collectInterval)
	}
	bucketStart := bucketEnd.Add(-collectInterval)
	for _, target := range targets {
		adapter, resolveErr := p.providers.Resolve(ctx, target.providerID)
		if resolveErr != nil {
			resultErr = errors.Join(resultErr, resolveErr)
			continue
		}
		providerCtx, cancel := context.WithTimeout(ctx, providerTimeout)
		traffic, trafficErr := adapter.GetTraffic(providerCtx, providercontract.GetTrafficRequest{GetInstanceRequest: providercontract.GetInstanceRequest{NodeID: target.providerNodeID, ProviderInstanceID: target.providerInstanceID, PlatformInstanceID: target.instanceID.String()}, From: bucketStart, To: bucketEnd})
		cancel()
		if trafficErr != nil {
			resultErr = errors.Join(resultErr, trafficErr)
			continue
		}
		if traffic == nil || traffic.RXBytes < 0 || traffic.TXBytes < 0 {
			resultErr = errors.Join(resultErr, errors.New("provider returned invalid traffic usage"))
			continue
		}
		inserted, persistErr := p.persistSample(ctx, target, bucketStart, bucketEnd, traffic.RXBytes, traffic.TXBytes)
		if persistErr != nil {
			resultErr = errors.Join(resultErr, persistErr)
			continue
		}
		if inserted {
			count++
		}
	}
	return count, resultErr
}

func (p *Processor) persistSample(ctx context.Context, target collectionTarget, from, to time.Time, rx, txBytes int64) (bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	sourceID := target.instanceID.String() + ":" + fmt.Sprint(from.Unix())
	result, err := tx.Exec(ctx, `INSERT INTO usage_samples(id,provider_id,instance_id,source_event_id,period_start,period_end,rx_bytes,tx_bytes) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(provider_id,source_event_id) DO NOTHING`, newID(), target.providerID, target.instanceID, sourceID, from, to, rx, txBytes)
	if err != nil {
		return false, err
	}
	includedBytes := int64(math.MaxInt64)
	if target.includedGB != nil {
		if *target.includedGB < 0 || *target.includedGB > math.MaxInt64/gib {
			return false, errors.New("traffic allowance is invalid")
		}
		includedBytes = *target.includedGB * gib
	}
	_, err = tx.Exec(ctx, `INSERT INTO usage_billing_periods(id,subscription_id,instance_id,period_start,period_end,included_bytes,overage_price_minor_per_gb,currency) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(subscription_id,period_start) DO NOTHING`, newID(), target.subscriptionID, target.instanceID, target.periodStart, target.periodEnd, includedBytes, target.overagePrice, target.currency)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO traffic_usage(id,instance_id,period_start,period_end,rx_bytes,tx_bytes,source) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(instance_id,period_start,source) DO NOTHING`, newID(), target.instanceID, from, to, rx, txBytes, "provider:"+target.providerID.String())
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

type billingPeriod struct {
	id, subscriptionID, instanceID uuid.UUID
	start, end                     time.Time
	included, unitPrice            int64
	currency                       string
}

func (p *Processor) rate(ctx context.Context) (int, error) {
	now := p.now().UTC()
	rows, err := p.pool.Query(ctx, `SELECT id,subscription_id,instance_id,period_start,period_end,included_bytes,overage_price_minor_per_gb,currency FROM usage_billing_periods WHERE status='open' AND period_end<=$1 ORDER BY period_end FOR UPDATE SKIP LOCKED LIMIT 20`, now)
	if err != nil {
		return 0, err
	}
	periods := []billingPeriod{}
	for rows.Next() {
		var value billingPeriod
		if err = rows.Scan(&value.id, &value.subscriptionID, &value.instanceID, &value.start, &value.end, &value.included, &value.unitPrice, &value.currency); err != nil {
			rows.Close()
			return 0, err
		}
		periods = append(periods, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, period := range periods {
		if err = p.ratePeriod(ctx, period); err != nil {
			return 0, err
		}
	}
	return len(periods), nil
}

func (p *Processor) ratePeriod(ctx context.Context, period billingPeriod) error {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM usage_billing_periods WHERE id=$1 FOR UPDATE`, period.id).Scan(&status); err != nil {
		return err
	}
	if status != "open" {
		return tx.Commit(ctx)
	}
	var used int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(rx_bytes+tx_bytes),0) FROM usage_samples WHERE instance_id=$1 AND period_start>=$2 AND period_end<=$3`, period.instanceID, period.start, period.end).Scan(&used); err != nil {
		return err
	}
	overage := max(used-period.included, 0)
	amount := ratedAmount(overage, period.unitPrice)
	if amount == 0 {
		_, err = tx.Exec(ctx, `UPDATE usage_billing_periods SET used_bytes=$2,overage_bytes=$3,amount_minor=0,status='closed',updated_at=now() WHERE id=$1`, period.id, used, overage)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	invoiceID, transactionID := newID(), newID()
	invoiceNo := number("INV-U", invoiceID)
	if _, err = tx.Exec(ctx, `INSERT INTO invoices(id,invoice_no,user_id,subscription_id,status,amount_minor,currency,due_at,billing_profile_snapshot) SELECT $1,$2,s.user_id,$3,'open',$4,$5,now()+interval '7 days',COALESCE((SELECT to_jsonb(bp)-'user_id'-'updated_at' FROM billing_profiles bp WHERE bp.user_id=s.user_id),'{}'::jsonb) FROM subscriptions s WHERE s.id=$3`, invoiceID, invoiceNo, period.subscriptionID, amount, period.currency); err != nil {
		return err
	}
	description, _ := json.Marshal(map[string]string{"zh-CN": "流量超额费用", "en-US": "Traffic overage"})
	if _, err = tx.Exec(ctx, `INSERT INTO invoice_items(id,invoice_id,description_i18n,quantity,unit_amount_minor,total_minor) VALUES($1,$2,$3,1,$4,$4)`, newID(), invoiceID, description, amount); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ledger_transactions(id,type,reference_type,reference_id,description) VALUES($1,'usage_charge','usage_billing_period',$2,'Rated traffic overage')`, transactionID, period.id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ledger_entries(id,transaction_id,account_type,account_id,direction,amount_minor,currency) VALUES($1,$2,'accounts_receivable',$3,'debit',$4,$5),($6,$2,'usage_revenue',$7,'credit',$4,$5)`, newID(), transactionID, invoiceID, amount, period.currency, newID(), period.subscriptionID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO usage_charges(id,usage_billing_period_id,invoice_id,ledger_transaction_id,amount_minor,currency) VALUES($1,$2,$3,$4,$5,$6)`, newID(), period.id, invoiceID, transactionID, amount, period.currency); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE usage_billing_periods SET used_bytes=$2,overage_bytes=$3,amount_minor=$4,status='invoiced',invoice_id=$5,updated_at=now() WHERE id=$1`, period.id, used, overage, amount, invoiceID); err != nil {
		return err
	}
	eventID := newID()
	payload, _ := json.Marshal(map[string]any{"event_id": eventID, "event_type": "usage.rated.v1", "occurred_at": time.Now().UTC(), "aggregate_type": "usage_billing_period", "aggregate_id": period.id, "data": map[string]any{"subscription_id": period.subscriptionID, "invoice_id": invoiceID, "amount_minor": amount, "currency": period.currency}})
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,event_type,aggregate_type,aggregate_id,payload,status,next_attempt_at) VALUES($1,'usage.rated.v1','usage_billing_period',$2,$3,'pending',now())`, eventID, period.id, payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func ratedAmount(overageBytes, priceMinorPerGB int64) int64 {
	if overageBytes <= 0 || priceMinorPerGB <= 0 {
		return 0
	}
	numerator := new(big.Int).Mul(big.NewInt(overageBytes), big.NewInt(priceMinorPerGB))
	numerator.Add(numerator, big.NewInt(gib-1))
	value := numerator.Div(numerator, big.NewInt(gib))
	if !value.IsInt64() {
		return math.MaxInt64
	}
	return value.Int64()
}

func newID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New()
	}
	return id
}

func number(prefix string, id uuid.UUID) string {
	return prefix + "-" + strings.ToUpper(strings.ReplaceAll(id.String(), "-", ""))[:18]
}
