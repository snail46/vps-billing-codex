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
	"github.com/jackc/pgx/v5"
)

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
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
	db        rowQuerier
	tokenHash [sha256.Size]byte
}

func New(collector *Collector, db rowQuerier, token string) *Handler {
	return &Handler{collector: collector, db: db, tokenHash: sha256.Sum256([]byte(token))}
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

	if h.db == nil {
		return
	}
	var outboxPending, operationsActive, operationsFailed, nodesOffline, agentsStale, payments24h int64
	var cpuAvailable, memoryAvailable, diskAvailable float64
	err := h.db.QueryRow(r.Context(), `SELECT
		(SELECT count(*) FROM outbox_events WHERE status='pending'),
		(SELECT count(*) FROM operations WHERE status IN ('queued','running','waiting_provider','waiting_resource','verifying','retrying')),
		(SELECT count(*) FROM operations WHERE status='failed' AND created_at>now()-interval '24 hours'),
		(SELECT count(*) FROM nodes WHERE status<>'online'),
		(SELECT count(*) FROM agent_connections WHERE status='connected' AND last_heartbeat_at<now()-interval '60 seconds'),
		(SELECT count(*) FROM payments WHERE status='succeeded' AND paid_at>now()-interval '24 hours'),
		(SELECT COALESCE(sum(cpu_total-cpu_allocated-cpu_reserved),0) FROM nodes WHERE status='online'),
		(SELECT COALESCE(sum(memory_total_mb-memory_allocated_mb-memory_reserved_mb),0) FROM nodes WHERE status='online'),
		(SELECT COALESCE(sum(disk_total_gb-disk_allocated_gb-disk_reserved_gb),0) FROM nodes WHERE status='online')`).Scan(
		&outboxPending, &operationsActive, &operationsFailed, &nodesOffline, &agentsStale, &payments24h, &cpuAvailable, &memoryAvailable, &diskAvailable,
	)
	if err != nil {
		_, _ = fmt.Fprintln(w, "vps_billing_metrics_collection_success 0")
		return
	}
	_, _ = fmt.Fprintln(w, "vps_billing_metrics_collection_success 1")
	for name, value := range map[string]int64{
		"outbox_pending": outboxPending, "operations_active": operationsActive, "operations_failed_24h": operationsFailed,
		"nodes_offline": nodesOffline, "agent_heartbeats_stale": agentsStale, "payments_succeeded_24h": payments24h,
	} {
		_, _ = fmt.Fprintf(w, "vps_billing_%s %d\n", name, value)
	}
	_, _ = fmt.Fprintf(w, "vps_billing_capacity_cpu_available %.2f\n", cpuAvailable)
	_, _ = fmt.Fprintf(w, "vps_billing_capacity_memory_mb_available %.0f\n", memoryAvailable)
	_, _ = fmt.Fprintf(w, "vps_billing_capacity_disk_gb_available %.0f\n", diskAvailable)
}
