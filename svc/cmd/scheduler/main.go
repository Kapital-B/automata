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
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, _ events.CloudWatchEvent) error {
	rt, err := ensureScheduler(ctx)
	if err != nil {
		return errors.New("scheduler unavailable")
	}
	if rt.Scheduler == nil {
		return nil
	}
	return rt.Scheduler.Tick(ctx, time.Now().UTC())
}

// ensureScheduler initializes once on success. Failures are not sticky so the next
// invoke retries — important when migrate IAM grants land after a warm start.
func ensureScheduler(ctx context.Context) (*composition.Runtime, error) {
	schedulerMu.Lock()
	defer schedulerMu.Unlock()
	if schedulerRuntime != nil {
		return schedulerRuntime, nil
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
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
	schedulerRuntime = rt
	return schedulerRuntime, nil
}
