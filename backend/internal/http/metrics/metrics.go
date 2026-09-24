package metrics

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"vps-billing/backend/internal/observability"
)

type snapshotReader interface {
	Snapshot(context.Context) (observability.Snapshot, error)
}

type Collector struct {
	mu       sync.Mutex
	requests map[string]uint64
	duration map[string]time.Duration
}

func NewCollector() *Collector {
	return &Collector{requests: make(map[string]uint64), duration: make(map[string]time.Duration)}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (w *responseWriter) Push(target string, options *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, options)
}

func (c *Collector) Instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		key := fmt.Sprintf("%s|%d", r.Method, recorder.status)
		c.mu.Lock()
		c.requests[key]++
		c.duration[key] += time.Since(started)
		c.mu.Unlock()
	})
}

type Handler struct {
	collector *Collector
	snapshots snapshotReader
	tokenHash [sha256.Size]byte
}

func New(collector *Collector, snapshots snapshotReader, token string) *Handler {
	return &Handler{collector: collector, snapshots: snapshots, tokenHash: sha256.Sum256([]byte(token))}
}

func (h *Handler) Register(router chi.Router) {
	router.Get("/metrics", h.serve)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	candidate := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	candidateHash := sha256.Sum256([]byte(candidate))
	if candidate == "" || subtle.ConstantTimeCompare(candidateHash[:], h.tokenHash[:]) != 1 {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	h.collector.mu.Lock()
	keys := make([]string, 0, len(h.collector.requests))
	for key := range h.collector.requests {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts := strings.Split(key, "|")
		_, _ = fmt.Fprintf(w, "vps_billing_http_requests_total{method=%q,status=%q} %d\n", parts[0], parts[1], h.collector.requests[key])
		_, _ = fmt.Fprintf(w, "vps_billing_http_request_duration_seconds_sum{method=%q,status=%q} %.6f\n", parts[0], parts[1], h.collector.duration[key].Seconds())
	}
	h.collector.mu.Unlock()

	if h.snapshots == nil {
		return
	}
	snapshot, err := h.snapshots.Snapshot(r.Context())
	if err != nil {
		_, _ = fmt.Fprintln(w, "vps_billing_metrics_collection_success 0")
		return
	}
	_, _ = fmt.Fprintln(w, "vps_billing_metrics_collection_success 1")
	for name, value := range map[string]int64{
		"outbox_pending": snapshot.OutboxPending, "operations_active": snapshot.OperationsActive, "operations_failed_24h": snapshot.OperationsFailed,
		"nodes_offline": snapshot.NodesOffline, "agent_heartbeats_stale": snapshot.AgentsStale, "payments_succeeded_24h": snapshot.Payments24h,
	} {
		_, _ = fmt.Fprintf(w, "vps_billing_%s %d\n", name, value)
	}
	_, _ = fmt.Fprintf(w, "vps_billing_capacity_cpu_available %.2f\n", snapshot.CPUAvailable)
	_, _ = fmt.Fprintf(w, "vps_billing_capacity_memory_mb_available %.0f\n", snapshot.MemoryAvailable)
	_, _ = fmt.Fprintf(w, "vps_billing_capacity_disk_gb_available %.0f\n", snapshot.DiskAvailable)
}
