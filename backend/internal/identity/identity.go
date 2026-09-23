package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"vps-billing/backend/internal/security/password"
	"vps-billing/backend/internal/security/session"
	"vps-billing/backend/internal/security/totp"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailInUse         = errors.New("email is already registered")
	ErrUnauthenticated    = errors.New("authentication required")
	ErrInactive           = errors.New("account is inactive")
	ErrTOTPRequired       = errors.New("two-factor authentication code is required")
	ErrTOTPInvalid        = errors.New("two-factor authentication code is invalid")
)

type User struct {
	ID       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	Status   string    `json:"status"`
	Locale   string    `json:"locale"`
	Timezone string    `json:"timezone"`
}

type Admin struct {
	ID               uuid.UUID `json:"id"`
	Email            string    `json:"email"`
	Status           string    `json:"status"`
	DisplayName      string    `json:"display_name"`
	TwoFactorEnabled bool      `json:"two_factor_enabled"`
	Permissions      []string  `json:"permissions"`
}

type Credentials struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	Status       string
	Locale       string
	Timezone     string
	DisplayName  string
	TwoFactor    bool
}

type Repository interface {
	CreateUser(context.Context, uuid.UUID, string, string, string, string) (User, error)
	UserCredentialsByEmail(context.Context, string) (Credentials, error)
	CreateUserSession(context.Context, uuid.UUID, uuid.UUID, []byte, time.Time) error
	UserBySession(context.Context, []byte) (User, uuid.UUID, error)
	TouchUserSession(context.Context, uuid.UUID) error
	RevokeUserSession(context.Context, []byte) error
	AdminCredentialsByEmail(context.Context, string) (Credentials, error)
	CreateAdminSession(context.Context, uuid.UUID, uuid.UUID, []byte, time.Time) error
	AdminBySession(context.Context, []byte) (Admin, uuid.UUID, error)
	TouchAdminSession(context.Context, uuid.UUID) error
	RevokeAdminSession(context.Context, []byte) error
	SaveAdminTOTPSecret(context.Context, uuid.UUID, []byte, []byte) error
	AdminTOTPSecret(context.Context, uuid.UUID) ([]byte, []byte, error)
	EnableAdminTOTP(context.Context, uuid.UUID) error
}

type Service struct {
	repository         Repository
	userSessionSecret  string
	adminSessionSecret string
	userCSRFSecret     string
	adminCSRFSecret    string
	ttl                time.Duration
	dummyPasswordHash  string
	totp               *totp.Manager
}

type AuthResult[T any] struct {
	Principal T      `json:"principal"`
	CSRFToken string `json:"csrf_token"`
	Token     string `json:"-"`
}

func NewService(repository Repository, userSessionSecret, adminSessionSecret, userCSRFSecret, adminCSRFSecret, totpEncryptionKey string, ttl time.Duration) *Service {
	dummyHash, _ := password.Hash("not-a-real-account-password")
	totpManager, _ := totp.NewManager(totpEncryptionKey)
	return &Service{repository: repository, userSessionSecret: userSessionSecret, adminSessionSecret: adminSessionSecret, userCSRFSecret: userCSRFSecret, adminCSRFSecret: adminCSRFSecret, ttl: ttl, dummyPasswordHash: dummyHash, totp: totpManager}
}

func (s *Service) Register(ctx context.Context, email, rawPassword, locale, timezone string) (AuthResult[User], error) {
	hash, err := password.Hash(rawPassword)
	if err != nil {
		return AuthResult[User]{}, err
	}
	if locale != "en-US" && locale != "zh-CN" {
		locale = "zh-CN"
	}
	if strings.TrimSpace(timezone) == "" {
		timezone = "UTC"
	}
	user, err := s.repository.CreateUser(ctx, newID(), normalizeEmail(email), hash, locale, timezone)
	if err != nil {
		return AuthResult[User]{}, err
	}
	return s.newUserSession(ctx, user)
}

func (s *Service) LoginUser(ctx context.Context, email, rawPassword string) (AuthResult[User], error) {
	credentials, err := s.repository.UserCredentialsByEmail(ctx, normalizeEmail(email))
	hash := credentials.PasswordHash
	if err != nil {
		hash = s.dummyPasswordHash
	}
	if !password.Verify(hash, rawPassword) || err != nil {
		return AuthResult[User]{}, ErrInvalidCredentials
	}
	if credentials.Status != "active" {
		return AuthResult[User]{}, ErrInactive
	}
	return s.newUserSession(ctx, User{ID: credentials.ID, Email: credentials.Email, Status: credentials.Status, Locale: credentials.Locale, Timezone: credentials.Timezone})
}

func (s *Service) AuthenticateUser(ctx context.Context, token string) (AuthResult[User], error) {
	if token == "" {
		return AuthResult[User]{}, ErrUnauthenticated
	}
	user, sessionID, err := s.repository.UserBySession(ctx, session.Digest(s.userSessionSecret, token))
	if err != nil || user.Status != "active" {
		return AuthResult[User]{}, ErrUnauthenticated
	}
	_ = s.repository.TouchUserSession(ctx, sessionID)
	return AuthResult[User]{Principal: user, CSRFToken: session.CSRFToken(s.userCSRFSecret, token), Token: token}, nil
}

func (s *Service) LogoutUser(ctx context.Context, token string) error {
	return s.repository.RevokeUserSession(ctx, session.Digest(s.userSessionSecret, token))
}

func (s *Service) LoginAdmin(ctx context.Context, email, rawPassword, code string) (AuthResult[Admin], error) {
	credentials, err := s.repository.AdminCredentialsByEmail(ctx, normalizeEmail(email))
	hash := credentials.PasswordHash
	if err != nil {
		hash = s.dummyPasswordHash
	}
	if !password.Verify(hash, rawPassword) || err != nil {
		return AuthResult[Admin]{}, ErrInvalidCredentials
	}
	if credentials.Status != "active" {
		return AuthResult[Admin]{}, ErrInactive
	}
	if credentials.TwoFactor {
		if code == "" {
			return AuthResult[Admin]{}, ErrTOTPRequired
		}
		ciphertext, nonce, secretErr := s.repository.AdminTOTPSecret(ctx, credentials.ID)
		if secretErr != nil {
			return AuthResult[Admin]{}, ErrInvalidCredentials
		}
		secret, decryptErr := s.totp.Decrypt(ciphertext, nonce)
		if decryptErr != nil || !totp.Validate(secret, code, time.Now()) {
			return AuthResult[Admin]{}, ErrTOTPInvalid
		}
	}
	return s.newAdminSession(ctx, credentials.ID)
}

type TOTPSetup struct {
	Secret string `json:"secret"`
	URI    string `json:"otpauth_uri"`
}

func (s *Service) BeginAdminTOTP(ctx context.Context, admin Admin) (TOTPSetup, error) {
	secret, err := totp.GenerateSecret()
	if err != nil {
		return TOTPSetup{}, err
	}
	ciphertext, nonce, err := s.totp.Encrypt(secret)
	if err != nil {
		return TOTPSetup{}, err
	}
	if err := s.repository.SaveAdminTOTPSecret(ctx, admin.ID, ciphertext, nonce); err != nil {
		return TOTPSetup{}, err
	}
	return TOTPSetup{Secret: secret, URI: totp.URI("VPS Billing", admin.Email, secret)}, nil
}

func (s *Service) EnableAdminTOTP(ctx context.Context, adminID uuid.UUID, code string) error {
	ciphertext, nonce, err := s.repository.AdminTOTPSecret(ctx, adminID)
	if err != nil {
		return ErrTOTPInvalid
	}
	secret, err := s.totp.Decrypt(ciphertext, nonce)
	if err != nil || !totp.Validate(secret, code, time.Now()) {
		return ErrTOTPInvalid
	}
	return s.repository.EnableAdminTOTP(ctx, adminID)
}

func (s *Service) AuthenticateAdmin(ctx context.Context, token string) (AuthResult[Admin], error) {
	if token == "" {
		return AuthResult[Admin]{}, ErrUnauthenticated
	}
	admin, sessionID, err := s.repository.AdminBySession(ctx, session.Digest(s.adminSessionSecret, token))
	if err != nil || admin.Status != "active" {
		return AuthResult[Admin]{}, ErrUnauthenticated
	}
	_ = s.repository.TouchAdminSession(ctx, sessionID)
	return AuthResult[Admin]{Principal: admin, CSRFToken: session.CSRFToken(s.adminCSRFSecret, token), Token: token}, nil
}

func (s *Service) LogoutAdmin(ctx context.Context, token string) error {
	return s.repository.RevokeAdminSession(ctx, session.Digest(s.adminSessionSecret, token))
}

func (s *Service) ValidUserCSRF(token, candidate string) bool {
	return session.ValidCSRF(s.userCSRFSecret, token, candidate)
}
func (s *Service) ValidAdminCSRF(token, candidate string) bool {
	return session.ValidCSRF(s.adminCSRFSecret, token, candidate)
}

func (s *Service) newUserSession(ctx context.Context, user User) (AuthResult[User], error) {
	token, err := session.NewToken()
	if err != nil {
		return AuthResult[User]{}, err
	}
	if err := s.repository.CreateUserSession(ctx, newID(), user.ID, session.Digest(s.userSessionSecret, token), time.Now().Add(s.ttl)); err != nil {
		return AuthResult[User]{}, err
	}
	return AuthResult[User]{Principal: user, CSRFToken: session.CSRFToken(s.userCSRFSecret, token), Token: token}, nil
}

func (s *Service) newAdminSession(ctx context.Context, adminID uuid.UUID) (AuthResult[Admin], error) {
	token, err := session.NewToken()
	if err != nil {
		return AuthResult[Admin]{}, err
	}
	if err := s.repository.CreateAdminSession(ctx, newID(), adminID, session.Digest(s.adminSessionSecret, token), time.Now().Add(s.ttl)); err != nil {
		return AuthResult[Admin]{}, err
	}
	admin, _, err := s.repository.AdminBySession(ctx, session.Digest(s.adminSessionSecret, token))
	if err != nil {
		return AuthResult[Admin]{}, err
	}
	return AuthResult[Admin]{Principal: admin, CSRFToken: session.CSRFToken(s.adminCSRFSecret, token), Token: token}, nil
}

func newID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New()
	}
	return id
}

func normalizeEmail(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
