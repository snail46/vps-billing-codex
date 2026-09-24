package runtime

import (
	"log/slog"
	"os"
	"strings"
)

// Logger returns the process logger. Request and trace correlation are added
// by the observability foundation task.
func Logger(service string) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})).With("service", service)
}
