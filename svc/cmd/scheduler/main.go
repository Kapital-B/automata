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
	schedulerOnce    sync.Once
	schedulerInitErr error
	schedulerRuntime *composition.Runtime
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, _ events.CloudWatchEvent) error {
	schedulerOnce.Do(func() {
		log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
		cfg, err := configuration.Load()
		if err != nil {
			log.Error("scheduler init failed", "err", err)
			schedulerInitErr = err
			return
		}
		schedulerRuntime, err = composition.Build(ctx, log, cfg, composition.Options{
			EnableJobStore: true,
			LeaseOwner:     "scheduler",
		})
		if err != nil {
			log.Error("scheduler init failed", "err", err)
			schedulerInitErr = err
		}
	})
	if schedulerInitErr != nil {
		return errors.New("scheduler unavailable")
	}
	if schedulerRuntime == nil || schedulerRuntime.Scheduler == nil {
		return nil
	}
	return schedulerRuntime.Scheduler.Tick(ctx, time.Now().UTC())
}
