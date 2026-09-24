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
	email := flag.String("email", "", "administrator email")
	displayName := flag.String("display-name", "", "administrator display name")
	flag.Parse()
	rawPassword := os.Getenv("ADMIN_BOOTSTRAP_PASSWORD")
	if strings.TrimSpace(*email) == "" || rawPassword == "" {
		fmt.Fprintln(os.Stderr, "email and ADMIN_BOOTSTRAP_PASSWORD are required")
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
	if err := bootstrap(ctx, pool, strings.ToLower(strings.TrimSpace(*email)), *displayName, rawPassword); err != nil {
		fail(err)
	}
	fmt.Println("administrator created")
}

func bootstrap(ctx context.Context, pool *pgxpool.Pool, email, displayName, rawPassword string) error {
	hash, err := password.Hash(rawPassword)
	if err != nil {
		return err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	adminID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	admin, err := queries.CreateAdmin(ctx, db.CreateAdminParams{ID: adminID, Email: email, PasswordHash: hash, DisplayName: pgtype.Text{String: displayName, Valid: displayName != ""}})
	if err != nil {
		return err
	}
	if err := queries.AssignAdminRole(ctx, db.AssignAdminRoleParams{AdminID: admin.ID, Key: "super_admin"}); err != nil {
		return err
	}
	after, _ := json.Marshal(map[string]any{"email": email, "role": "super_admin"})
	if err := queries.CreateAuditEvent(ctx, db.CreateAuditEventParams{ID: uuid.New(), ActorType: "system", Action: "admin.bootstrap", ResourceType: "admin", ResourceID: &admin.ID, BeforeData: []byte("null"), AfterData: after, IpAddress: address("127.0.0.1"), UserAgent: pgtype.Text{String: "bootstrap-admin", Valid: true}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func address(value string) *netip.Addr { parsed, _ := netip.ParseAddr(value); return &parsed }
func fail(err error) {
	if !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(1)
}
