package identity

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "vps-billing/backend/internal/store/sqlc"
)

type PostgresRepository struct{ queries *db.Queries }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{queries: db.New(pool)}
}

func (r *PostgresRepository) CreateUser(ctx context.Context, id uuid.UUID, email, hash, locale, timezone string) (User, error) {
	row, err := r.queries.CreateUser(ctx, db.CreateUserParams{ID: id, Email: email, PasswordHash: hash, Locale: locale, Timezone: timezone})
	if isUniqueViolation(err) {
		return User{}, ErrEmailInUse
	}
	return User{ID: row.ID, Email: row.Email, Status: row.Status, Locale: row.Locale, Timezone: row.Timezone}, err
}

func (r *PostgresRepository) UserCredentialsByEmail(ctx context.Context, email string) (Credentials, error) {
	row, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{ID: row.ID, Email: row.Email, PasswordHash: row.PasswordHash, Status: row.Status, Locale: row.Locale, Timezone: row.Timezone}, nil
}

func (r *PostgresRepository) CreateUserSession(ctx context.Context, id, userID uuid.UUID, digest []byte, expires time.Time) error {
	_, err := r.queries.CreateUserSession(ctx, db.CreateUserSessionParams{ID: id, UserID: userID, TokenHash: digest, ExpiresAt: timestamp(expires)})
	return err
}

func (r *PostgresRepository) UserBySession(ctx context.Context, digest []byte) (User, uuid.UUID, error) {
	row, err := r.queries.GetActiveUserSession(ctx, digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, uuid.Nil, ErrUnauthenticated
	}
	return User{ID: row.UserID, Email: row.Email, Status: row.UserStatus, Locale: row.Locale, Timezone: row.Timezone}, row.ID, err
}

func (r *PostgresRepository) TouchUserSession(ctx context.Context, id uuid.UUID) error {
	return r.queries.TouchUserSession(ctx, id)
}
func (r *PostgresRepository) RevokeUserSession(ctx context.Context, digest []byte) error {
	return r.queries.RevokeUserSession(ctx, digest)
}

func (r *PostgresRepository) AdminCredentialsByEmail(ctx context.Context, email string) (Credentials, error) {
	row, err := r.queries.GetAdminByEmail(ctx, email)
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{ID: row.ID, Email: row.Email, PasswordHash: row.PasswordHash, Status: row.Status, DisplayName: row.DisplayName.String, TwoFactor: row.TwoFactorEnabled}, nil
}

func (r *PostgresRepository) CreateAdminSession(ctx context.Context, id, adminID uuid.UUID, digest []byte, expires time.Time) error {
	_, err := r.queries.CreateAdminSession(ctx, db.CreateAdminSessionParams{ID: id, AdminID: adminID, TokenHash: digest, ExpiresAt: timestamp(expires)})
	return err
}

func (r *PostgresRepository) AdminBySession(ctx context.Context, digest []byte) (Admin, uuid.UUID, error) {
	row, err := r.queries.GetActiveAdminSession(ctx, digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admin{}, uuid.Nil, ErrUnauthenticated
	}
	if err != nil {
		return Admin{}, uuid.Nil, err
	}
	permissions, err := r.queries.ListAdminPermissions(ctx, row.AdminID)
	return Admin{ID: row.AdminID, Email: row.Email, Status: row.AdminStatus, DisplayName: row.DisplayName.String, TwoFactorEnabled: row.TwoFactorEnabled, Permissions: permissions}, row.ID, err
}

func (r *PostgresRepository) TouchAdminSession(ctx context.Context, id uuid.UUID) error {
	return r.queries.TouchAdminSession(ctx, id)
}
func (r *PostgresRepository) RevokeAdminSession(ctx context.Context, digest []byte) error {
	return r.queries.RevokeAdminSession(ctx, digest)
}

func (r *PostgresRepository) SaveAdminTOTPSecret(ctx context.Context, adminID uuid.UUID, ciphertext, nonce []byte) error {
	return r.queries.UpsertAdminTOTPSecret(ctx, db.UpsertAdminTOTPSecretParams{AdminID: adminID, Ciphertext: ciphertext, Nonce: nonce})
}

func (r *PostgresRepository) AdminTOTPSecret(ctx context.Context, adminID uuid.UUID) ([]byte, []byte, error) {
	row, err := r.queries.GetAdminTOTPSecret(ctx, adminID)
	return row.Ciphertext, row.Nonce, err
}

func (r *PostgresRepository) EnableAdminTOTP(ctx context.Context, adminID uuid.UUID) error {
	return r.queries.EnableAdminTOTP(ctx, adminID)
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
