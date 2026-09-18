package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lmsv3/internal/labdb"
	"lmsv3/internal/migration/runner"
	"lmsv3/internal/outbox"
)

type syntheticSink struct{ logger *slog.Logger }

func (s syntheticSink) Deliver(ctx context.Context, event outbox.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// EventID is the provider idempotency key. This local sink intentionally
	// emits no actor, request, payload, credential, or database detail.
	s.logger.Info("synthetic_outbox_delivery", "event_id", event.EventID, "topic", event.Topic)
	return nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if os.Getenv("LMS_WORKER_MODE") != "local-synthetic" {
		logger.Error("worker_configuration_rejected")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	startup, stop := context.WithTimeout(ctx, 5*time.Second)
	pool, err := labdb.WorkerRuntime(startup)
	if err == nil {
		var catalogue []runner.Migration
		catalogue, err = labdb.SyntheticCatalogueScope(os.Getenv("QA_CATALOGUE_SCOPE"))
		if err == nil {
			err = runner.Ready(startup, pool, catalogue)
		}
	}
	stop()
	if err != nil {
		if pool != nil {
			pool.Close()
		}
		logger.Error("worker_database_rejected")
		os.Exit(1)
	}
	defer pool.Close()
	worker := outbox.Worker{
		Queue: outbox.Store{Pool: pool, Topic: "audit.v1"},
		Sink:  syntheticSink{logger: logger},
		Config: outbox.Config{
			BatchSize:       8,
			Parallelism:     4,
			PollInterval:    time.Second,
			LeaseDuration:   30 * time.Second,
			DeliveryTimeout: 10 * time.Second,
		},
	}
	logger.Info("worker_started", "mode", "local-synthetic")
	if err = worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("worker_stopped_with_error")
		os.Exit(1)
	}
	logger.Info("worker_stopped")
}
