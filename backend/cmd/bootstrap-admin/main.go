package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"vps-billing/backend/internal/config"
	"vps-billing/backend/internal/security/password"
	db "vps-billing/backend/internal/store/sqlc"
)

func main() {
	email := flag.String("email", os.Getenv("ADMIN_BOOTSTRAP_EMAIL"), "administrator email")
	displayName := flag.String("display-name", os.Getenv("ADMIN_BOOTSTRAP_DISPLAY_NAME"), "administrator display name")
	flag.Parse()
	normalizedEmail := strings.ToLower(strings.TrimSpace(*email))
	rawPassword := os.Getenv("ADMIN_BOOTSTRAP_PASSWORD")
	if normalizedEmail == "" && rawPassword == "" {
		fmt.Println("administrator bootstrap not configured; skipping")
		return
	}
	if normalizedEmail == "" || rawPassword == "" {
		fmt.Fprintln(os.Stderr, "ADMIN_BOOTSTRAP_EMAIL (or --email) and ADMIN_BOOTSTRAP_PASSWORD must be configured together")
		os.Exit(2)
	}
	settings, err := config.Load()
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, settings.DatabaseURL)
	if err != nil {
		fail(err)
	}
	defer pool.Close()
	created, err := bootstrap(ctx, pool, normalizedEmail, strings.TrimSpace(*displayName), rawPassword)
	if err != nil {
		fail(err)
	}
	if created {
		fmt.Println("administrator created")
		return
	}
	fmt.Println("administrator already exists; bootstrap skipped")
}

func bootstrap(ctx context.Context, pool *pgxpool.Pool, email, displayName, rawPassword string) (bool, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	if _, err := queries.GetAdminByEmail(ctx, email); err == nil {
		return false, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	hash, err := password.Hash(rawPassword)
	if err != nil {
		return false, err
	}
	adminID, err := uuid.NewV7()
	if err != nil {
		return false, err
	}
	admin, err := queries.CreateAdmin(ctx, db.CreateAdminParams{ID: adminID, Email: email, PasswordHash: hash, DisplayName: pgtype.Text{String: displayName, Valid: displayName != ""}})
	if err != nil {
		return false, err
	}
	if err := queries.AssignAdminRole(ctx, db.AssignAdminRoleParams{AdminID: admin.ID, Key: "super_admin"}); err != nil {
		return false, err
	}
	after, _ := json.Marshal(map[string]any{"email": email, "role": "super_admin"})
	if err := queries.CreateAuditEvent(ctx, db.CreateAuditEventParams{ID: uuid.New(), ActorType: "system", Action: "admin.bootstrap", ResourceType: "admin", ResourceID: &admin.ID, BeforeData: []byte("null"), AfterData: after, IpAddress: address("127.0.0.1"), UserAgent: pgtype.Text{String: "bootstrap-admin", Valid: true}}); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func address(value string) *netip.Addr { parsed, _ := netip.ParseAddr(value); return &parsed }
func fail(err error) {
	if !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(1)
}
