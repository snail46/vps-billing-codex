package commercehttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"vps-billing/backend/internal/commerce"
	identityhttp "vps-billing/backend/internal/http/identity"
	"vps-billing/backend/internal/http/response"
	"vps-billing/backend/internal/payment/fake"
)

const maxWebhookBytes = 64 << 10

type Handler struct {
	service            *commerce.Service
	identity           *identityhttp.Handler
	gateway            *fake.Gateway
	fakePaymentEnabled bool
}

func New(service *commerce.Service, identity *identityhttp.Handler, gateway *fake.Gateway, fakePaymentEnabled bool) *Handler {
	return &Handler{service: service, identity: identity, gateway: gateway, fakePaymentEnabled: fakePaymentEnabled}
}

func (h *Handler) Register(router chi.Router) {
	router.Get("/api/v1/products", h.listProducts)
	router.With(h.identity.RequireUser).Get("/api/v1/orders", h.listOrders)
	router.With(h.identity.RequireUser).Post("/api/v1/orders", h.createOrder)
	router.With(h.identity.RequireUser).Get("/api/v1/invoices", h.listInvoices)
	router.With(h.identity.RequireUser).Get("/api/v1/wallet", h.wallet)
	if h.fakePaymentEnabled {
		router.Post("/api/v1/webhooks/payments/fake", h.fakeWebhook)
	}
}

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListCatalog(r.Context())
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "errors.internal")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	user, ok := identityhttp.UserFromContext(r.Context())
	if !ok {
		response.Error(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "errors.unauthenticated")
		return
	}
	var input struct {
		PlanID   uuid.UUID `json:"plan_id"`
		Quantity int32     `json:"quantity"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		response.Error(w, r, http.StatusBadRequest, "REQUEST_INVALID", "errors.requestInvalid")
		return
	}
	order, err := h.service.CreateOrder(r.Context(), user.ID, input.PlanID, input.Quantity, r.Header.Get("Idempotency-Key"))
	if err != nil {
		h.commerceError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, order)
}

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	items, err := h.service.ListOrders(r.Context(), user.ID)
	if err != nil {
		h.commerceError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) listInvoices(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	items, err := h.service.ListInvoices(r.Context(), user.ID)
	if err != nil {
		h.commerceError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) wallet(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	currency := strings.ToUpper(r.URL.Query().Get("currency"))
	if currency == "" {
		currency = "USD"
	}
	wallet, err := h.service.Wallet(r.Context(), user.ID, currency)
	if err != nil {
		h.commerceError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, wallet)
}

func (h *Handler) fakeWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "REQUEST_INVALID", "errors.requestInvalid")
		return
	}
	if err := h.gateway.Verify(body, r.Header.Get("X-Fake-Signature")); err != nil {
		response.Error(w, r, http.StatusUnauthorized, "WEBHOOK_SIGNATURE_INVALID", "errors.webhookSignatureInvalid")
		return
	}
	var event commerce.Webhook
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		response.Error(w, r, http.StatusBadRequest, "REQUEST_INVALID", "errors.requestInvalid")
		return
	}
	result, err := h.service.CompletePayment(r.Context(), event, body)
	if err != nil {
		h.commerceError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, result)
}

func (h *Handler) commerceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, commerce.ErrPlanNotFound):
		response.Error(w, r, http.StatusNotFound, "PLAN_NOT_FOUND", "errors.planNotFound")
	case errors.Is(err, commerce.ErrInvalidQuantity):
		response.Error(w, r, http.StatusUnprocessableEntity, "QUANTITY_INVALID", "errors.quantityInvalid")
	case errors.Is(err, commerce.ErrInvalidIdempotency):
		response.Error(w, r, http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_INVALID", "errors.idempotencyInvalid")
	case errors.Is(err, commerce.ErrIdempotencyConflict):
		response.Error(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "errors.idempotencyConflict")
	case errors.Is(err, commerce.ErrPaymentNotFound):
		response.Error(w, r, http.StatusNotFound, "PAYMENT_NOT_FOUND", "errors.paymentNotFound")
	case errors.Is(err, commerce.ErrPaymentMismatch):
		response.Error(w, r, http.StatusUnprocessableEntity, "PAYMENT_MISMATCH", "errors.paymentMismatch")
	case errors.Is(err, commerce.ErrPaymentState):
		response.Error(w, r, http.StatusConflict, "PAYMENT_STATE_INVALID", "errors.paymentStateInvalid")
	case errors.Is(err, commerce.ErrSubscriptionNotFound):
		response.Error(w, r, http.StatusNotFound, "SUBSCRIPTION_NOT_FOUND", "errors.subscriptionNotFound")
	case errors.Is(err, commerce.ErrSubscriptionState):
		response.Error(w, r, http.StatusConflict, "SUBSCRIPTION_STATE_INVALID", "errors.subscriptionStateInvalid")
	default:
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "errors.internal")
	}
}
