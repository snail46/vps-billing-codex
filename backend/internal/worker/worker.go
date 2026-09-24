package worker

import (
	"context"
	"log/slog"
	"time"
)

type Processor interface {
	ProcessBatch(context.Context) (int, error)
}

type Worker struct {
	logger     *slog.Logger
	processors []Processor
}

func New(logger *slog.Logger, processors ...Processor) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{logger: logger, processors: processors}
}

func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("worker started")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		for _, processor := range w.processors {
			count, err := processor.ProcessBatch(ctx)
			if err != nil && ctx.Err() == nil {
				w.logger.Error("worker processor failed", "error", err)
			}
			if count > 0 {
				w.logger.Info("worker batch processed", "count", count)
			}
		}
		select {
		case <-ctx.Done():
			w.logger.Info("worker stopped")
			return nil
		case <-ticker.C:
		}
	}
}
