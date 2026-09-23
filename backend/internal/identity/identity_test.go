package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"vps-billing/backend/internal/security/password"
)

type fakeRepository struct {
	userCredentials  Credentials
	adminCredentials Credentials
	userDigest       []byte
	adminDigest      []byte
	user             User
	admin            Admin
	totpCiphertext   []byte
	totpNonce        []byte
	totpEnabled      bool
}

func (f *fakeRepository) CreateUser(_ context.Context, id uuid.UUID, email, hash, locale, timezone string) (User, error) {
	f.userCredentials = Credentials{ID: id, Email: email, PasswordHash: hash, Status: "active", Locale: locale, Timezone: timezone}
	f.user = User{ID: id, Email: email, Status: "active", Locale: locale, Timezone: timezone}
	return f.user, nil
}
func (f *fakeRepository) UserCredentialsByEmail(context.Context, string) (Credentials, error) {
	if f.userCredentials.ID == uuid.Nil {
		return Credentials{}, errors.New("not found")
	}
	return f.userCredentials, nil
}
func (f *fakeRepository) CreateUserSession(_ context.Context, _, _ uuid.UUID, digest []byte, _ time.Time) error {
	f.userDigest = digest
	return nil
}
func (f *fakeRepository) UserBySession(_ context.Context, digest []byte) (User, uuid.UUID, error) {
	if string(digest) != string(f.userDigest) {
		return User{}, uuid.Nil, ErrUnauthenticated
	}
	return f.user, uuid.New(), nil
}
func (*fakeRepository) TouchUserSession(context.Context, uuid.UUID) error { return nil }
func (*fakeRepository) RevokeUserSession(context.Context, []byte) error   { return nil }
func (f *fakeRepository) AdminCredentialsByEmail(context.Context, string) (Credentials, error) {
	if f.adminCredentials.ID == uuid.Nil {
		return Credentials{}, errors.New("not found")
	}
	return f.adminCredentials, nil
}
func (f *fakeRepository) CreateAdminSession(_ context.Context, _, _ uuid.UUID, digest []byte, _ time.Time) error {
	f.adminDigest = digest
	return nil
}
func (f *fakeRepository) AdminBySession(_ context.Context, digest []byte) (Admin, uuid.UUID, error) {
	if string(digest) != string(f.adminDigest) {
		return Admin{}, uuid.Nil, ErrUnauthenticated
	}
	return f.admin, uuid.New(), nil
}
func (*fakeRepository) TouchAdminSession(context.Context, uuid.UUID) error { return nil }
func (*fakeRepository) RevokeAdminSession(context.Context, []byte) error   { return nil }
func (f *fakeRepository) SaveAdminTOTPSecret(_ context.Context, _ uuid.UUID, ciphertext, nonce []byte) error {
	f.totpCiphertext, f.totpNonce = ciphertext, nonce
	return nil
}
func (f *fakeRepository) AdminTOTPSecret(context.Context, uuid.UUID) ([]byte, []byte, error) {
	if len(f.totpCiphertext) == 0 {
		return nil, nil, errors.New("not configured")
	}
	return f.totpCiphertext, f.totpNonce, nil
}
func (f *fakeRepository) EnableAdminTOTP(context.Context, uuid.UUID) error {
	f.totpEnabled = true
	return nil
}

func TestRegisterAndAuthenticateUser(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository, "user-session", "admin-session", "user-csrf", "admin-csrf", "totp-encryption-key", time.Hour)
	registered, err := service.Register(context.Background(), " Person@Example.COM ", "a sufficiently long password", "en-US", "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if registered.Principal.Email != "person@example.com" || !service.ValidUserCSRF(registered.Token, registered.CSRFToken) {
		t.Fatalf("Register() = %#v", registered)
	}
	authenticated, err := service.AuthenticateUser(context.Background(), registered.Token)
	if err != nil || authenticated.Principal.ID != registered.Principal.ID {
		t.Fatalf("AuthenticateUser() = %#v, %v", authenticated, err)
	}
	if _, err := service.AuthenticateUser(context.Background(), "wrong"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("wrong token error = %v", err)
	}
}

func TestAdminAndUserSessionsAreCryptographicallyIsolated(t *testing.T) {
	hash, err := password.Hash("a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	repository := &fakeRepository{adminCredentials: Credentials{ID: id, Email: "admin@example.com", PasswordHash: hash, Status: "active"}, admin: Admin{ID: id, Email: "admin@example.com", Status: "active", Permissions: []string{"audit.read"}}}
	service := NewService(repository, "user-session", "admin-session", "user-csrf", "admin-csrf", "totp-encryption-key", time.Hour)
	admin, err := service.LoginAdmin(context.Background(), "admin@example.com", "a sufficiently long password", "")
	if err != nil {
		t.Fatal(err)
	}
	if service.ValidUserCSRF(admin.Token, admin.CSRFToken) {
		t.Fatal("admin CSRF token passed user validation")
	}
	if _, err := service.AuthenticateUser(context.Background(), admin.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("admin token authenticated as user: %v", err)
	}
}
