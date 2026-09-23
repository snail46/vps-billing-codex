package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"sync/atomic"
	"time"
)

type correlationKey string

const (
	requestIDKey correlationKey = "request_id"
	traceIDKey   correlationKey = "trace_id"
)

var (
	validCorrelationID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	fallbackSequence   atomic.Uint64
)

func Correlation(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestID := incomingOrNewID(request.Header.Get("X-Request-ID"), logger)
		traceID := incomingOrNewID(request.Header.Get("X-Trace-ID"), logger)
		ctx := context.WithValue(request.Context(), requestIDKey, requestID)
		ctx = context.WithValue(ctx, traceIDKey, traceID)
		response.Header().Set("X-Request-ID", requestID)
		response.Header().Set("X-Trace-ID", traceID)
		next.ServeHTTP(response, request.WithContext(ctx))
	})
}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func TraceID(ctx context.Context) string {
	value, _ := ctx.Value(traceIDKey).(string)
	return value
}

func incomingOrNewID(candidate string, logger *slog.Logger) string {
	if validCorrelationID.MatchString(candidate) {
		return candidate
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	} else {
		logger.Error("correlation ID entropy unavailable", "error", err)
	}
	sequence := fallbackSequence.Add(1)
	return time.Now().UTC().Format("20060102T150405.000000000") + "-" + formatSequence(sequence)
}

func formatSequence(value uint64) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	var buffer [13]byte
	index := len(buffer)
	for {
		index--
		buffer[index] = digits[value%uint64(len(digits))]
		value /= uint64(len(digits))
		if value == 0 {
			return string(buffer[index:])
		}
	}
}
