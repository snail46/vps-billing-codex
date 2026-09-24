package operationhttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	identityhttp "vps-billing/backend/internal/http/identity"
	"vps-billing/backend/internal/http/response"
	"vps-billing/backend/internal/operation"
)

type Handler struct {
	service  *operation.Service
	identity *identityhttp.Handler
	redis    *redis.Client
}

func New(service *operation.Service, identity *identityhttp.Handler, redisClient *redis.Client) *Handler {
	return &Handler{service: service, identity: identity, redis: redisClient}
}

func (h *Handler) Register(router chi.Router) {
	router.With(h.identity.RequireUser).Get("/api/v1/operations/{id}", h.get)
	router.With(h.identity.RequireUser).Get("/api/v1/events", h.events)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	operationID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "REQUEST_INVALID", "errors.requestInvalid")
		return
	}
	result, err := h.service.GetForUser(r.Context(), user.ID, operationID)
	if errors.Is(err, operation.ErrNotFound) {
		response.Error(w, r, http.StatusNotFound, "OPERATION_NOT_FOUND", "errors.operationNotFound")
		return
	}
	if err != nil {
		response.Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "errors.internal")
		return
	}
	response.JSON(w, r, http.StatusOK, result)
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	user, _ := identityhttp.UserFromContext(r.Context())
	flusher, ok := w.(http.Flusher)
	if !ok {
		response.Error(w, r, http.StatusInternalServerError, "SSE_UNAVAILABLE", "errors.sseUnavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	cursor := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if cursor == "" {
		cursor = "$"
	}
	for {
		streams, err := h.redis.XRead(r.Context(), &redis.XReadArgs{Streams: []string{"domain-events", cursor}, Count: 25, Block: 15 * time.Second}).Result()
		if errors.Is(err, redis.Nil) {
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
			continue
		}
		if err != nil {
			return
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				cursor = message.ID
				payload := fmt.Sprint(message.Values["payload"])
				if !eventBelongsToUser([]byte(payload), user.ID) {
					continue
				}
				eventType := strings.ReplaceAll(fmt.Sprint(message.Values["event_type"]), "\n", "")
				_, _ = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", message.ID, eventType, strings.ReplaceAll(payload, "\n", ""))
				flusher.Flush()
			}
		}
	}
}

func eventBelongsToUser(payload []byte, userID uuid.UUID) bool {
	var envelope struct {
		Data struct {
			UserID *uuid.UUID `json:"user_id"`
		} `json:"data"`
	}
	return json.Unmarshal(payload, &envelope) == nil && envelope.Data.UserID != nil && *envelope.Data.UserID == userID
}
