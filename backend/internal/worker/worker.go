package worker

import (
	"context"
	"log/slog"
)

type Worker struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{logger: logger}
}

func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("worker started")
	<-ctx.Done()
	w.logger.Info("worker stopped")
	return nil
}
