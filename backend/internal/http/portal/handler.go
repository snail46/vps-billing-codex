package portalhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	identityhttp "vps-billing/backend/internal/http/identity"
	"vps-billing/backend/internal/http/middleware"
	"vps-billing/backend/internal/http/response"
	"vps-billing/backend/internal/operation"
	"vps-billing/backend/internal/portal"
)

const maxBodyBytes = 16 << 10

type Handler struct {
	service    *portal.Service
	operations *operation.Service
	identity   *identityhttp.Handler
}

func New(service *portal.Service, operations *operation.Service, identity *identityhttp.Handler) *Handler {
	return &Handler{service: service, operations: operations, identity: identity}
}

func (h *Handler) Register(router chi.Router) {
	router.Group(func(r chi.Router) {
		r.Use(h.identity.RequireUser)
		r.Get("/api/v1/instances", h.listInstances)
		r.Get("/api/v1/instances/{id}", h.getInstance)
		r.Get("/api/v1/instances/{id}/networks", h.listNetworks)
		r.Get("/api/v1/instances/{id}/traffic", h.listTraffic)
		r.Post("/api/v1/instances/{id}/{action}", h.instanceAction)
		r.Get("/api/v1/notifications", h.listNotifications)
		r.Put("/api/v1/notifications/{id}/read", h.markNotificationRead)
		r.Get("/api/v1/tickets", h.listTickets)
		r.Post("/api/v1/tickets", h.createTicket)
		r.Get("/api/v1/tickets/{id}", h.getTicket)
		r.Post("/api/v1/tickets/{id}/messages", h.addTicketMessage)
	})
}

func (h *Handler) listInstances(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	items, err := h.service.ListInstances(r.Context(), user.ID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) getInstance(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := h.service.GetInstance(r.Context(), user.ID, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}
func (h *Handler) listNetworks(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	items, err := h.service.ListNetworks(r.Context(), user.ID, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) listTraffic(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	items, err := h.service.ListTraffic(r.Context(), user.ID, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) instanceAction(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	action := chi.URLParam(r, "action")
	if action != "start" && action != "stop" && action != "restart" && action != "reinstall" {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "errors.instanceNotFound")
		return
	}
	current, err := h.service.ActionContext(r.Context(), user.ID, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if action == "reinstall" {
		var input struct {
			ImageID string `json:"image_id"`
		}
		if !decode(w, r, &input) {
			return
		}
		if strings.TrimSpace(input.ImageID) == "" || input.ImageID != current.ImageID {
			response.Error(w, r, http.StatusUnprocessableEntity, "IMAGE_INVALID", "errors.imageInvalid")
			return
		}
	}
	steps := []operation.StepDefinition{{Key: "validate", Order: 1}, {Key: "provider", Order: 2}, {Key: "verify", Order: 3}}
	created, err := h.operations.Create(r.Context(), operation.CreateRequest{Type: action, ResourceType: "instance", ResourceID: id, IdempotencyKey: r.Header.Get("Idempotency-Key"), TraceID: middleware.TraceID(r.Context()), UserID: &user.ID, MaxRetries: 3, Steps: steps})
	if err != nil {
		if errors.Is(err, operation.ErrInvalidRequest) {
			response.Error(w, r, http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_INVALID", "errors.idempotencyInvalid")
		} else if errors.Is(err, operation.ErrIdempotencyConflict) {
			response.Error(w, r, http.StatusConflict, "INSTANCE_OPERATION_CONFLICT", "errors.instanceOperationConflict")
		} else {
			h.writeError(w, r, err)
		}
		return
	}
	response.JSON(w, r, http.StatusAccepted, map[string]any{"operation_id": created.ID, "status": created.Status})
}

func (h *Handler) listNotifications(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	items, err := h.service.ListNotifications(r.Context(), user.ID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) markNotificationRead(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.service.MarkNotificationRead(r.Context(), user.ID, id); err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"read": true})
}
func (h *Handler) listTickets(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	items, err := h.service.ListTickets(r.Context(), user.ID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) getTicket(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := h.service.GetTicket(r.Context(), user.ID, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, item)
}
func (h *Handler) createTicket(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	var input struct {
		Subject  string `json:"subject"`
		Priority string `json:"priority"`
		Message  string `json:"message"`
	}
	if !decode(w, r, &input) {
		return
	}
	item, err := h.service.CreateTicket(r.Context(), user.ID, input.Subject, input.Priority, input.Message)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}
func (h *Handler) addTicketMessage(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input struct {
		Message string `json:"message"`
	}
	if !decode(w, r, &input) {
		return
	}
	item, err := h.service.AddTicketMessage(r.Context(), user.ID, id, input.Message)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, item)
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		response.Error(w, r, http.StatusBadRequest, "REQUEST_INVALID", "errors.requestInvalid")
		return false
	}
	return true
}
func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "errors.resourceNotFound")
		return uuid.Nil, false
	}
	return id, true
}
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, portal.ErrNotFound):
		response.Error(w, r, http.StatusNotFound, "NOT_FOUND", "errors.resourceNotFound")
	case errors.Is(err, portal.ErrInvalidRequest):
		response.Error(w, r, http.StatusUnprocessableEntity, "REQUEST_INVALID", "errors.requestInvalid")
	case errors.Is(err, portal.ErrStateConflict):
		response.Error(w, r, http.StatusConflict, "INSTANCE_STATE_CONFLICT", "errors.instanceStateConflict")
	case errors.Is(err, portal.ErrTicketClosed):
		response.Error(w, r, http.StatusConflict, "TICKET_CLOSED", "errors.ticketClosed")
	default:
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "errors.internal")
	}
}
