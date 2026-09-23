package adminhttp

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"vps-billing/backend/internal/admin"
	identityhttp "vps-billing/backend/internal/http/identity"
	"vps-billing/backend/internal/http/middleware"
	"vps-billing/backend/internal/http/response"
)

const maxBodyBytes = 64 << 10

type Handler struct {
	service  *admin.Service
	identity *identityhttp.Handler
}

func New(service *admin.Service, identity *identityhttp.Handler) *Handler {
	return &Handler{service: service, identity: identity}
}

func (h *Handler) Register(router chi.Router) {
	router.Route("/api/v1/admin", func(r chi.Router) {
		r.With(h.require("health.read")).Get("/dashboard", h.dashboard)
		for _, route := range []struct{ path, resource, permission string }{
			{"/users", "users", "users.read"}, {"/products", "products", "products.read"}, {"/orders", "orders", "orders.read"},
			{"/payments", "payments", "payments.read"}, {"/ledger", "ledger", "ledger.read"}, {"/subscriptions", "subscriptions", "subscriptions.read"},
			{"/instances", "instances", "instances.read"}, {"/nodes", "nodes", "nodes.read"}, {"/providers", "providers", "providers.read"},
			{"/operations", "operations", "operations.read"}, {"/tickets", "tickets", "tickets.read"}, {"/audit", "audit", "audit.read"},
			{"/admins", "admins", "admins.read"}, {"/roles", "roles", "roles.read"}, {"/settings", "settings", "settings.read"},
		} {
			resource := route.resource
			r.With(h.require(route.permission)).Get(route.path, h.list(resource))
		}
		r.With(h.require("instances.read")).Get("/instances/{id}", h.instance)
		r.With(h.require("operations.read")).Get("/operations/{id}", h.operation)
		r.With(h.require("users.suspend")).Patch("/users/{id}/status", h.updateUserStatus)
		r.With(h.require("ledger.adjust")).Post("/ledger/adjustments", h.adjustWallet)
		r.With(h.require("tickets.reply")).Post("/tickets/{id}/messages", h.replyTicket)
		r.With(h.require("tickets.manage")).Patch("/tickets/{id}/status", h.updateTicketStatus)
		r.With(h.require("settings.manage")).Put("/settings/{key}", h.updateSetting)
		r.With(h.require("admins.manage")).Put("/admins/{id}/roles", h.setAdminRoles)
	})
}

func (h *Handler) require(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return h.identity.RequireAdmin(permission, next) }
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	value, err := h.service.Dashboard(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, value)
}
func (h *Handler) list(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := h.service.List(r.Context(), resource)
		if err != nil {
			h.writeError(w, r, err)
			return
		}
		response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
	}
}
func (h *Handler) instance(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	principal, _ := identityhttp.AdminFromContext(r.Context())
	item, err := h.service.Instance(r.Context(), id, hasPermission(principal.Permissions, "providers.manage"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}
func (h *Handler) operation(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	principal, _ := identityhttp.AdminFromContext(r.Context())
	item, err := h.service.Operation(r.Context(), id, hasPermission(principal.Permissions, "operations.retry"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}
func (h *Handler) updateUserStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &input) {
		return
	}
	item, err := h.service.UpdateUserStatus(r.Context(), id, input.Status, h.audit(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}
func (h *Handler) adjustWallet(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UserID      uuid.UUID `json:"user_id"`
		Currency    string    `json:"currency"`
		AmountMinor int64     `json:"amount_minor"`
		Description string    `json:"description"`
	}
	if !decode(w, r, &input) {
		return
	}
	item, err := h.service.AdjustWallet(r.Context(), input.UserID, input.Currency, input.AmountMinor, input.Description, h.audit(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}
func (h *Handler) replyTicket(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var input struct {
		Message string `json:"message"`
	}
	if !decode(w, r, &input) {
		return
	}
	item, err := h.service.ReplyTicket(r.Context(), id, input.Message, h.audit(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}
func (h *Handler) updateTicketStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &input) {
		return
	}
	item, err := h.service.UpdateTicketStatus(r.Context(), id, input.Status, h.audit(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}
func (h *Handler) updateSetting(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Value    any  `json:"value"`
		IsSecret bool `json:"is_secret"`
	}
	if !decode(w, r, &input) {
		return
	}
	item, err := h.service.UpdateSetting(r.Context(), chi.URLParam(r, "key"), input.Value, input.IsSecret, h.audit(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}
func (h *Handler) setAdminRoles(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var input struct {
		Roles []string `json:"roles"`
	}
	if !decode(w, r, &input) {
		return
	}
	item, err := h.service.SetAdminRoles(r.Context(), id, input.Roles, h.audit(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) audit(r *http.Request) admin.AuditContext {
	principal, _ := identityhttp.AdminFromContext(r.Context())
	return admin.AuditContext{AdminID: principal.ID, IPAddress: clientIP(r), UserAgent: r.UserAgent(), RequestID: middleware.RequestID(r.Context()), TraceID: middleware.TraceID(r.Context())}
}
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, admin.ErrNotFound):
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "errors.notFound")
	case errors.Is(err, admin.ErrInvalidInput):
		response.Error(w, r, http.StatusUnprocessableEntity, "INPUT_INVALID", "errors.requestInvalid")
	case errors.Is(err, admin.ErrInsufficient):
		response.Error(w, r, http.StatusConflict, "INSUFFICIENT_BALANCE", "errors.insufficientBalance")
	case errors.Is(err, admin.ErrTicketFinalized):
		response.Error(w, r, http.StatusConflict, "TICKET_FINALIZED", "errors.ticketClosed")
	default:
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "errors.internal")
	}
}
func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "ID_INVALID", "errors.requestInvalid")
		return uuid.Nil, false
	}
	return id, true
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
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
	return true
}
func hasPermission(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
