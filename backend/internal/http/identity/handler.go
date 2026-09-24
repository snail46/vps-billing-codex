package identityhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"vps-billing/backend/internal/audit"
	"vps-billing/backend/internal/http/middleware"
	"vps-billing/backend/internal/http/response"
	"vps-billing/backend/internal/identity"
	"vps-billing/backend/internal/security/password"
	"vps-billing/backend/internal/security/ratelimit"
)

type principalKey string

const userPrincipalKey principalKey = "user_principal"
const adminPrincipalKey principalKey = "admin_principal"

const maxBodyBytes = 16 << 10

type Handler struct {
	service     *identity.Service
	limiter     *ratelimit.Limiter
	secure      bool
	userCookie  string
	adminCookie string
	audit       audit.Recorder
}

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Locale   string `json:"locale"`
	Timezone string `json:"timezone"`
	TOTPCode string `json:"totp_code"`
}

func New(service *identity.Service, limiter *ratelimit.Limiter, recorder audit.Recorder, secure bool) *Handler {
	userCookie, adminCookie := "vps_user_session", "vps_admin_session"
	if secure {
		userCookie, adminCookie = "__Host-vps-user-session", "__Host-vps-admin-session"
	}
	return &Handler{service: service, limiter: limiter, audit: recorder, secure: secure, userCookie: userCookie, adminCookie: adminCookie}
}

func (h *Handler) Register(router chi.Router) {
	router.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/register", h.registerUser)
		r.Post("/login", h.loginUser)
		r.Get("/me", h.userMe)
		r.Post("/logout", h.logoutUser)
	})
	router.Route("/api/v1/admin/auth", func(r chi.Router) {
		r.Post("/login", h.loginAdmin)
		r.Get("/me", h.adminMe)
		r.Post("/logout", h.logoutAdmin)
		r.Post("/totp/setup", h.setupAdminTOTP)
		r.Post("/totp/enable", h.enableAdminTOTP)
	})
}

func (h *Handler) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := h.service.AuthenticateUser(r.Context(), h.cookie(r, h.userCookie))
		if err != nil {
			h.identityError(w, r, err)
			return
		}
		if isUnsafe(r.Method) && !h.service.ValidUserCSRF(result.Token, r.Header.Get("X-CSRF-Token")) {
			response.Error(w, r, http.StatusForbidden, "CSRF_INVALID", "errors.csrfInvalid")
			return
		}
		ctx := context.WithValue(r.Context(), userPrincipalKey, result.Principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) RequireAdmin(permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := h.service.AuthenticateAdmin(r.Context(), h.cookie(r, h.adminCookie))
		if err != nil {
			h.identityError(w, r, err)
			return
		}
		if permission != "" && !contains(result.Principal.Permissions, permission) {
			response.Error(w, r, http.StatusForbidden, "PERMISSION_DENIED", "errors.permissionDenied")
			return
		}
		if isUnsafe(r.Method) && !h.service.ValidAdminCSRF(result.Token, r.Header.Get("X-CSRF-Token")) {
			response.Error(w, r, http.StatusForbidden, "CSRF_INVALID", "errors.csrfInvalid")
			return
		}
		ctx := context.WithValue(r.Context(), adminPrincipalKey, result.Principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func UserFromContext(ctx context.Context) (identity.User, bool) {
	value, ok := ctx.Value(userPrincipalKey).(identity.User)
	return value, ok
}
func AdminFromContext(ctx context.Context) (identity.Admin, bool) {
	value, ok := ctx.Value(adminPrincipalKey).(identity.Admin)
	return value, ok
}

func (h *Handler) registerUser(w http.ResponseWriter, r *http.Request) {
	var input credentialsRequest
	if !h.decodeAndLimit(w, r, &input, "register:"+strings.ToLower(input.Email), 5) {
		return
	}
	result, err := h.service.Register(r.Context(), input.Email, input.Password, input.Locale, input.Timezone)
	if err != nil {
		h.identityError(w, r, err)
		return
	}
	h.setCookie(w, h.userCookie, result.Token)
	response.JSON(w, r, http.StatusCreated, result)
}

func (h *Handler) loginUser(w http.ResponseWriter, r *http.Request) {
	var input credentialsRequest
	if !h.decodeAndLimit(w, r, &input, "user-login", 10) {
		return
	}
	result, err := h.service.LoginUser(r.Context(), input.Email, input.Password)
	if err != nil {
		h.identityError(w, r, err)
		return
	}
	h.setCookie(w, h.userCookie, result.Token)
	response.JSON(w, r, http.StatusOK, result)
}

func (h *Handler) userMe(w http.ResponseWriter, r *http.Request) {
	token := h.cookie(r, h.userCookie)
	result, err := h.service.AuthenticateUser(r.Context(), token)
	if err != nil {
		h.identityError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, result)
}

func (h *Handler) logoutUser(w http.ResponseWriter, r *http.Request) {
	token := h.cookie(r, h.userCookie)
	if _, err := h.service.AuthenticateUser(r.Context(), token); err != nil {
		h.identityError(w, r, err)
		return
	}
	if !h.service.ValidUserCSRF(token, r.Header.Get("X-CSRF-Token")) {
		response.Error(w, r, http.StatusForbidden, "CSRF_INVALID", "errors.csrfInvalid")
		return
	}
	if err := h.service.LogoutUser(r.Context(), token); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "errors.internal")
		return
	}
	h.clearCookie(w, h.userCookie)
	response.JSON(w, r, http.StatusOK, map[string]bool{"logged_out": true})
}

func (h *Handler) loginAdmin(w http.ResponseWriter, r *http.Request) {
	var input credentialsRequest
	if !h.decodeAndLimit(w, r, &input, "admin-login", 5) {
		return
	}
	result, err := h.service.LoginAdmin(r.Context(), input.Email, input.Password, input.TOTPCode)
	if err != nil {
		h.recordAdminAudit(r, audit.Event{ActorType: "anonymous", Action: "admin.login_failed", ResourceType: "admin", After: map[string]any{"email": strings.ToLower(strings.TrimSpace(input.Email))}})
		h.identityError(w, r, err)
		return
	}
	adminID := result.Principal.ID
	if err := h.recordAdminAudit(r, audit.Event{ActorType: "admin", ActorID: &adminID, Action: "admin.login", ResourceType: "admin", ResourceID: &adminID}); err != nil {
		_ = h.service.LogoutAdmin(r.Context(), result.Token)
		response.Error(w, r, http.StatusInternalServerError, "AUDIT_FAILED", "errors.internal")
		return
	}
	h.setCookie(w, h.adminCookie, result.Token)
	response.JSON(w, r, http.StatusOK, result)
}

func (h *Handler) setupAdminTOTP(w http.ResponseWriter, r *http.Request) {
	result, ok := h.authenticatedAdmin(w, r)
	if !ok {
		return
	}
	adminID := result.Principal.ID
	if err := h.recordAdminAudit(r, audit.Event{ActorType: "admin", ActorID: &adminID, Action: "admin.totp_setup_requested", ResourceType: "admin", ResourceID: &adminID}); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "AUDIT_FAILED", "errors.internal")
		return
	}
	setup, err := h.service.BeginAdminTOTP(r.Context(), result.Principal)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "errors.internal")
		return
	}
	response.JSON(w, r, http.StatusOK, setup)
}

func (h *Handler) enableAdminTOTP(w http.ResponseWriter, r *http.Request) {
	result, ok := h.authenticatedAdmin(w, r)
	if !ok {
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		response.Error(w, r, http.StatusBadRequest, "REQUEST_INVALID", "errors.requestInvalid")
		return
	}
	adminID := result.Principal.ID
	if err := h.recordAdminAudit(r, audit.Event{ActorType: "admin", ActorID: &adminID, Action: "admin.totp_enable_requested", ResourceType: "admin", ResourceID: &adminID}); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "AUDIT_FAILED", "errors.internal")
		return
	}
	if err := h.service.EnableAdminTOTP(r.Context(), result.Principal.ID, input.Code); err != nil {
		h.identityError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"two_factor_enabled": true})
}

func (h *Handler) authenticatedAdmin(w http.ResponseWriter, r *http.Request) (identity.AuthResult[identity.Admin], bool) {
	token := h.cookie(r, h.adminCookie)
	result, err := h.service.AuthenticateAdmin(r.Context(), token)
	if err != nil {
		h.identityError(w, r, err)
		return identity.AuthResult[identity.Admin]{}, false
	}
	if !h.service.ValidAdminCSRF(token, r.Header.Get("X-CSRF-Token")) {
		response.Error(w, r, http.StatusForbidden, "CSRF_INVALID", "errors.csrfInvalid")
		return identity.AuthResult[identity.Admin]{}, false
	}
	return result, true
}

func (h *Handler) adminMe(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.AuthenticateAdmin(r.Context(), h.cookie(r, h.adminCookie))
	if err != nil {
		h.identityError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, result)
}

func (h *Handler) logoutAdmin(w http.ResponseWriter, r *http.Request) {
	token := h.cookie(r, h.adminCookie)
	principal, err := h.service.AuthenticateAdmin(r.Context(), token)
	if err != nil {
		h.identityError(w, r, err)
		return
	}
	if !h.service.ValidAdminCSRF(token, r.Header.Get("X-CSRF-Token")) {
		response.Error(w, r, http.StatusForbidden, "CSRF_INVALID", "errors.csrfInvalid")
		return
	}
	adminID := principal.Principal.ID
	if err := h.recordAdminAudit(r, audit.Event{ActorType: "admin", ActorID: &adminID, Action: "admin.logout_requested", ResourceType: "admin", ResourceID: &adminID}); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "AUDIT_FAILED", "errors.internal")
		return
	}
	if err := h.service.LogoutAdmin(r.Context(), token); err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "errors.internal")
		return
	}
	h.clearCookie(w, h.adminCookie)
	response.JSON(w, r, http.StatusOK, map[string]bool{"logged_out": true})
}

func (h *Handler) decodeAndLimit(w http.ResponseWriter, r *http.Request, target *credentialsRequest, action string, limit int64) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		response.Error(w, r, http.StatusUnsupportedMediaType, "CONTENT_TYPE_INVALID", "errors.contentTypeInvalid")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		response.Error(w, r, http.StatusBadRequest, "REQUEST_INVALID", "errors.requestInvalid")
		return false
	}
	parsedEmail, err := mail.ParseAddress(strings.TrimSpace(target.Email))
	if err != nil || !strings.EqualFold(parsedEmail.Address, strings.TrimSpace(target.Email)) || len(parsedEmail.Address) > 320 {
		response.Error(w, r, http.StatusUnprocessableEntity, "EMAIL_INVALID", "errors.emailInvalid")
		return false
	}
	key := action + ":" + clientIP(r) + ":" + strings.ToLower(strings.TrimSpace(target.Email))
	allowed, err := h.limiter.Allow(r.Context(), key, limit, time.Minute)
	if err != nil {
		response.Error(w, r, http.StatusServiceUnavailable, "RATE_LIMIT_UNAVAILABLE", "errors.authUnavailable")
		return false
	}
	if !allowed {
		response.Error(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "errors.rateLimited")
		return false
	}
	return true
}

func (h *Handler) identityError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrInvalidCredentials):
		response.Error(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "errors.invalidCredentials")
	case errors.Is(err, identity.ErrUnauthenticated):
		response.Error(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "errors.unauthenticated")
	case errors.Is(err, identity.ErrInactive):
		response.Error(w, r, http.StatusForbidden, "ACCOUNT_INACTIVE", "errors.accountInactive")
	case errors.Is(err, identity.ErrEmailInUse):
		response.Error(w, r, http.StatusConflict, "EMAIL_IN_USE", "errors.emailInUse")
	case errors.Is(err, password.ErrInvalidPassword):
		response.Error(w, r, http.StatusUnprocessableEntity, "PASSWORD_INVALID", "errors.passwordInvalid")
	case errors.Is(err, identity.ErrTOTPRequired):
		response.Error(w, r, http.StatusUnauthorized, "TOTP_REQUIRED", "errors.totpRequired")
	case errors.Is(err, identity.ErrTOTPInvalid):
		response.Error(w, r, http.StatusUnauthorized, "TOTP_INVALID", "errors.totpInvalid")
	default:
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "errors.internal")
	}
}

func (h *Handler) setCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteStrictMode, MaxAge: 86400})
}
func (h *Handler) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}
func (h *Handler) cookie(r *http.Request, name string) string {
	value, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return value.Value
}
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func isUnsafe(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func (h *Handler) recordAdminAudit(r *http.Request, event audit.Event) error {
	if h.audit == nil {
		return nil
	}
	event.IPAddress = clientIP(r)
	event.UserAgent = r.UserAgent()
	event.RequestID = middleware.RequestID(r.Context())
	event.TraceID = middleware.TraceID(r.Context())
	return h.audit.Record(r.Context(), event)
}
