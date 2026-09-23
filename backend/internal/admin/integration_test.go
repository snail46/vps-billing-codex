package admin

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"vps-billing/backend/internal/migrations"
)

func TestWalletAdjustmentIsBalancedAndAudited(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := migrations.Run(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID, adminID := uuid.New(), uuid.New()
	if _, err = pool.Exec(ctx, `INSERT INTO users(id,email,password_hash,status) VALUES($1,$2,'test','active'); INSERT INTO admins(id,email,password_hash,status) VALUES($3,$4,'test','active')`, userID, "admin-wallet-"+userID.String()+"@example.com", adminID, "admin-wallet-"+adminID.String()+"@example.com"); err != nil {
		t.Fatal(err)
	}
	repository := NewPostgresRepository(pool)
	audit := AuditContext{AdminID: adminID, RequestID: uuid.NewString(), TraceID: uuid.NewString()}
	result, err := repository.AdjustWallet(ctx, userID, "USD", 1500, "service credit", audit)
	if err != nil {
		t.Fatal(err)
	}
	transactionID, err := uuid.Parse(result["transaction_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var entries int
	var debits, credits int64
	if err = pool.QueryRow(ctx, `SELECT count(*),COALESCE(sum(amount_minor) FILTER(WHERE direction='debit'),0),COALESCE(sum(amount_minor) FILTER(WHERE direction='credit'),0) FROM ledger_entries WHERE transaction_id=$1`, transactionID).Scan(&entries, &debits, &credits); err != nil {
		t.Fatal(err)
	}
	if entries != 2 || debits != 1500 || credits != 1500 {
		t.Fatalf("ledger entries=%d debits=%d credits=%d", entries, debits, credits)
	}
	var balance int64
	if err = pool.QueryRow(ctx, `SELECT available_balance_minor FROM wallets WHERE user_id=$1 AND currency='USD'`, userID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 1500 {
		t.Fatalf("balance=%d", balance)
	}
	var audits int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE actor_id=$1 AND action='ledger.adjusted'`, adminID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("audit count=%d", audits)
	}
	if _, err = repository.AdjustWallet(ctx, userID, "USD", -2000, "invalid debit", audit); err != ErrInsufficient {
		t.Fatalf("negative adjustment error=%v", err)
	}
}

func TestSecretSettingIsMaskedAndAudited(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := migrations.Run(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	adminID := uuid.New()
	if _, err = pool.Exec(ctx, `INSERT INTO admins(id,email,password_hash,status) VALUES($1,$2,'test','active')`, adminID, "admin-setting-"+adminID.String()+"@example.com"); err != nil {
		t.Fatal(err)
	}
	repository := NewPostgresRepository(pool)
	key := "test.secret." + uuid.NewString()
	item, err := repository.UpdateSetting(ctx, key, "private", true, AuditContext{AdminID: adminID})
	if err != nil {
		t.Fatal(err)
	}
	if item["value"] != "***" {
		t.Fatalf("secret response=%v", item["value"])
	}
	items, err := repository.List(ctx, "settings")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, candidate := range items {
		if candidate["key"] == key {
			found = true
			if candidate["value"] != "***" {
				t.Fatalf("listed secret=%v", candidate["value"])
			}
		}
	}
	if !found {
		t.Fatal("setting not listed")
	}
}
