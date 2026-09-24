package subscriptionhttp

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"vps-billing/backend/internal/commerce"
	identityhttp "vps-billing/backend/internal/http/identity"
	"vps-billing/backend/internal/http/response"
	"vps-billing/backend/internal/subscription"
)

type Handler struct {
	service  *subscription.Service
	commerce *commerce.Service
	identity *identityhttp.Handler
}

func New(service *subscription.Service, commerceService *commerce.Service, identity *identityhttp.Handler) *Handler {
	return &Handler{service: service, commerce: commerceService, identity: identity}
}

func (h *Handler) Register(router chi.Router) {
	router.With(h.identity.RequireUser).Get("/api/v1/subscriptions", h.list)
	router.With(h.identity.RequireUser).Post("/api/v1/subscriptions/{id}/renewals", h.renew)
	router.With(h.identity.RequireUser).Put("/api/v1/subscriptions/{id}/cancel-at-period-end", h.setCancelAtPeriodEnd)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	items, err := h.service.List(r.Context(), user.ID)
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "errors.internal")
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) renew(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	subscriptionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "REQUEST_INVALID", "errors.requestInvalid")
		return
	}
	order, err := h.commerce.CreateRenewalOrder(r.Context(), user.ID, subscriptionID, r.Header.Get("Idempotency-Key"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, order)
}

func (h *Handler) setCancelAtPeriodEnd(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	subscriptionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "REQUEST_INVALID", "errors.requestInvalid")
		return
	}
	var input struct {
		Cancel bool `json:"cancel"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		response.Error(w, r, http.StatusBadRequest, "REQUEST_INVALID", "errors.requestInvalid")
		return
	}
	result, err := h.service.SetCancelAtPeriodEnd(r.Context(), user.ID, subscriptionID, input.Cancel)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, result)
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, subscription.ErrNotFound), errors.Is(err, commerce.ErrSubscriptionNotFound):
		response.Error(w, r, http.StatusNotFound, "SUBSCRIPTION_NOT_FOUND", "errors.subscriptionNotFound")
	case errors.Is(err, subscription.ErrInvalidState), errors.Is(err, commerce.ErrSubscriptionState):
		response.Error(w, r, http.StatusConflict, "SUBSCRIPTION_STATE_INVALID", "errors.subscriptionStateInvalid")
	case errors.Is(err, commerce.ErrInvalidIdempotency):
		response.Error(w, r, http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_INVALID", "errors.idempotencyInvalid")
	case errors.Is(err, commerce.ErrIdempotencyConflict):
		response.Error(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "errors.idempotencyConflict")
	default:
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "errors.internal")
	}
}
