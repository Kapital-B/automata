package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/Kapital-B/automata/svc/internal/composition"
	"github.com/Kapital-B/automata/svc/internal/configuration"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

var (
	schedulerMu      sync.Mutex
	schedulerRuntime *composition.Runtime
	schedulerLog     *slog.Logger
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, _ events.CloudWatchEvent) error {
	log := schedulerLogger()
	start := time.Now().UTC()
	rt, err := ensureScheduler(ctx)
	if err != nil {
		log.Error("scheduler unavailable", "err", err)
		return errors.New("scheduler unavailable")
	}
	if rt.Scheduler == nil {
		log.Info("scheduler tick skipped", "reason", "scheduler not configured")
		return nil
	}
	log.Info("scheduler tick start")
	if err := rt.Scheduler.Tick(ctx, time.Now().UTC()); err != nil {
		log.Error("scheduler tick failed", "err", err, "duration_ms", time.Since(start).Milliseconds())
		return err
	}
	log.Info("scheduler tick ok", "duration_ms", time.Since(start).Milliseconds())
	return nil
}

// ensureScheduler initializes once on success. Failures are not sticky so the next
// invoke retries — important when migrate IAM grants land after a warm start.
func ensureScheduler(ctx context.Context) (*composition.Runtime, error) {
	schedulerMu.Lock()
	defer schedulerMu.Unlock()
	if schedulerRuntime != nil {
		return schedulerRuntime, nil
	}

	log := schedulerLogger()
	cfg, err := configuration.Load()
	if err != nil {
		log.Error("scheduler init failed", "err", err)
		return nil, err
	}
	rt, err := composition.Build(ctx, log, cfg, composition.Options{
		EnableJobStore: true,
		LeaseOwner:     "scheduler",
	})
	if err != nil {
		log.Error("scheduler init failed", "err", err)
		return nil, err
	}
	log.Info("scheduler init ok", "jobs_table", cfg.JobsTableName, "database_engine", cfg.DatabaseEngine)
	schedulerRuntime = rt
	return schedulerRuntime, nil
}

func schedulerLogger() *slog.Logger {
	if schedulerLog == nil {
		schedulerLog = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return schedulerLog
}
