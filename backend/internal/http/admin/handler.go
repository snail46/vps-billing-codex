package adminhttp

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"vps-billing/backend/internal/admin"
	identityhttp "vps-billing/backend/internal/http/identity"
	"vps-billing/backend/internal/http/middleware"
	"vps-billing/backend/internal/http/response"
	"vps-billing/backend/internal/operation"
)

const maxBodyBytes = 64 << 10

type Handler struct {
	service    *admin.Service
	identity   *identityhttp.Handler
	operations *operation.Service
}

func New(service *admin.Service, identity *identityhttp.Handler, operations *operation.Service) *Handler {
	return &Handler{service: service, identity: identity, operations: operations}
}

func (h *Handler) Register(router chi.Router) {
	router.Route("/api/v1/admin", func(r chi.Router) {
		r.With(h.require("health.read")).Get("/dashboard", h.dashboard)
		for _, route := range []struct{ path, resource, permission string }{
			{"/users", "users", "users.read"}, {"/products", "products", "products.read"}, {"/orders", "orders", "orders.read"},
			{"/payments", "payments", "payments.read"}, {"/ledger", "ledger", "ledger.read"}, {"/subscriptions", "subscriptions", "subscriptions.read"},
			{"/instances", "instances", "instances.read"}, {"/nodes", "nodes", "nodes.read"}, {"/providers", "providers", "providers.read"},
			{"/operations", "operations", "operations.read"}, {"/tickets", "tickets", "tickets.read"}, {"/audit", "audit", "audit.read"},
			{"/usage", "usage", "usage.read"}, {"/dead-letters", "dead_letters", "operations.read"},
			{"/admins", "admins", "admins.read"}, {"/roles", "roles", "roles.read"}, {"/settings", "settings", "settings.read"},
		} {
			resource := route.resource
			r.With(h.require(route.permission)).Get(route.path, h.list(resource))
		}
		r.With(h.require("instances.read")).Get("/instances/{id}", h.instance)
		r.With(h.require("operations.read")).Get("/operations/{id}", h.operation)
		r.With(h.require("providers.read")).Get("/providers/{id}", h.provider)
		r.With(h.require("operations.retry")).Post("/operations/{id}/retry", h.retryOperation)
		r.With(h.require("operations.retry")).Post("/operations/{id}/cancel", h.cancelOperation)
		r.With(h.require("outbox.replay")).Post("/dead-letters/{id}/replay", h.replayOutbox)
		r.With(h.require("payments.refund")).Post("/payments/{id}/refunds", h.refundPayment)
		r.With(h.require("products.manage")).Post("/products", h.createProduct)
		r.With(h.require("products.manage")).Patch("/products/{id}", h.updateProduct)
		r.With(h.require("products.manage")).Post("/products/{id}/plans", h.createPlan)
		r.With(h.require("products.manage")).Patch("/plans/{id}", h.updatePlan)
		r.With(h.require("users.suspend")).Patch("/users/{id}/status", h.updateUserStatus)
		r.With(h.require("ledger.adjust")).Post("/ledger/adjustments", h.adjustWallet)
		r.With(h.require("tickets.reply")).Post("/tickets/{id}/messages", h.replyTicket)
		r.With(h.require("tickets.manage")).Patch("/tickets/{id}/status", h.updateTicketStatus)
		r.With(h.require("settings.manage")).Put("/settings/{key}", h.updateSetting)
		r.With(h.require("admins.manage")).Put("/admins/{id}/roles", h.setAdminRoles)
	})
}

func (h *Handler) replayOutbox(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	item, err := h.service.ReplayOutbox(r.Context(), id, h.audit(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, item)
}

func (h *Handler) refundPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var input struct {
		AmountMinor int64  `json:"amount_minor"`
		Reason      string `json:"reason"`
	}
	if !decode(w, r, &input) {
		return
	}
	item, err := h.service.RefundPayment(r.Context(), id, input.AmountMinor, input.Reason, r.Header.Get("Idempotency-Key"), h.audit(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) retryOperation(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	principal, _ := identityhttp.AdminFromContext(r.Context())
	item, err := h.operations.RetryAdmin(r.Context(), id, principal.ID, r.Header.Get("Idempotency-Key"), middleware.TraceID(r.Context()))
	if err != nil {
		h.writeOperationError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, map[string]any{"operation_id": item.ID, "status": item.Status, "parent_operation_id": id})
}

func (h *Handler) cancelOperation(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	principal, _ := identityhttp.AdminFromContext(r.Context())
	item, err := h.operations.CancelAdmin(r.Context(), id, principal.ID, middleware.TraceID(r.Context()))
	if err != nil {
		h.writeOperationError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) writeOperationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, operation.ErrNotFound):
		response.Error(w, r, http.StatusNotFound, "OPERATION_NOT_FOUND", "errors.operationNotFound")
	case errors.Is(err, operation.ErrInvalidRequest):
		response.Error(w, r, http.StatusUnprocessableEntity, "OPERATION_REQUEST_INVALID", "errors.requestInvalid")
	case errors.Is(err, operation.ErrIdempotencyConflict), errors.Is(err, operation.ErrStateConflict):
		response.Error(w, r, http.StatusConflict, "OPERATION_STATE_CONFLICT", "errors.operationStateConflict")
	default:
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "errors.internal")
	}
}

type productInput struct {
	Slug            string            `json:"slug"`
	NameI18n        map[string]string `json:"name_i18n"`
	DescriptionI18n map[string]string `json:"description_i18n"`
	Status          string            `json:"status"`
	SortOrder       int32             `json:"sort_order"`
	ProductType     string            `json:"product_type"`
	Featured        bool              `json:"featured"`
}

type planInput struct {
	Slug           string            `json:"slug"`
	NameI18n       map[string]string `json:"name_i18n"`
	Status         string            `json:"status"`
	NodeGroupID    *uuid.UUID        `json:"node_group_id"`
	CPUCores       float64           `json:"cpu_cores"`
	MemoryMB       int32             `json:"memory_mb"`
	DiskGB         int32             `json:"disk_gb"`
	TrafficGB      *int64            `json:"traffic_gb"`
	BandwidthMbps  *int32            `json:"bandwidth_mbps"`
	IPv4Count      int32             `json:"ipv4_count"`
	IPv6Count      int32             `json:"ipv6_count"`
	NATPortCount   int32             `json:"nat_port_count"`
	Virtualization string            `json:"virtualization"`
	BillingCycle   string            `json:"billing_cycle"`
	PriceMinor     int64             `json:"price_minor"`
	Currency       string            `json:"currency"`
	StockMode      string            `json:"stock_mode"`
	DefaultImageID string            `json:"default_image_id"`
	StockQuantity  *int32            `json:"stock_quantity"`
	SetupFeeMinor  int64             `json:"setup_fee_minor"`
	OverageMinor   int64             `json:"traffic_overage_price_minor"`
}

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request) {
	var input productInput
	if !decode(w, r, &input) {
		return
	}
	item, err := h.service.CreateProduct(r.Context(), admin.ProductInput{Slug: input.Slug, NameI18n: input.NameI18n, Description: input.DescriptionI18n, Status: input.Status, SortOrder: input.SortOrder, ProductType: input.ProductType, Featured: input.Featured}, h.audit(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var input productInput
	if !decode(w, r, &input) {
		return
	}
	item, err := h.service.UpdateProduct(r.Context(), id, admin.ProductInput{Slug: input.Slug, NameI18n: input.NameI18n, Description: input.DescriptionI18n, Status: input.Status, SortOrder: input.SortOrder, ProductType: input.ProductType, Featured: input.Featured}, h.audit(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (h *Handler) createPlan(w http.ResponseWriter, r *http.Request) {
	productID, ok := parseID(w, r)
	if !ok {
		return
	}
	var input planInput
	if !decode(w, r, &input) {
		return
	}
	item, err := h.service.CreatePlan(r.Context(), productID, input.domain(), h.audit(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func (h *Handler) updatePlan(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var input planInput
	if !decode(w, r, &input) {
		return
	}
	item, err := h.service.UpdatePlan(r.Context(), id, input.domain(), h.audit(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}

func (p planInput) domain() admin.PlanInput {
	return admin.PlanInput{Slug: p.Slug, NameI18n: p.NameI18n, Status: p.Status, NodeGroupID: p.NodeGroupID, CPUCores: p.CPUCores, MemoryMB: p.MemoryMB, DiskGB: p.DiskGB, TrafficGB: p.TrafficGB, BandwidthMbps: p.BandwidthMbps, IPv4Count: p.IPv4Count, IPv6Count: p.IPv6Count, NATPortCount: p.NATPortCount, Virtualization: p.Virtualization, BillingCycle: p.BillingCycle, PriceMinor: p.PriceMinor, Currency: p.Currency, StockMode: p.StockMode, DefaultImageID: p.DefaultImageID, StockQuantity: p.StockQuantity, SetupFeeMinor: p.SetupFeeMinor, OverageMinor: p.OverageMinor}
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
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if offset < 0 {
			offset = 0
		}
		items, total, err := h.service.ListFiltered(r.Context(), resource, r.URL.Query().Get("q"), r.URL.Query().Get("status"), limit, offset)
		if err != nil {
			h.writeError(w, r, err)
			return
		}
		response.JSON(w, r, http.StatusOK, map[string]any{"items": items, "pagination": map[string]int{"total": total, "limit": limit, "offset": offset}})
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
func (h *Handler) provider(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	principal, _ := identityhttp.AdminFromContext(r.Context())
	item, err := h.service.Provider(r.Context(), id, hasPermission(principal.Permissions, "providers.manage"))
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
	case errors.Is(err, admin.ErrRefundInvalid):
		response.Error(w, r, http.StatusConflict, "REFUND_INVALID", "errors.refundInvalid")
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
