package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/Kapital-B/automata/svc/internal/composition"
	"github.com/Kapital-B/automata/svc/internal/configuration"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/google/uuid"
)

var (
	workerOnce    sync.Once
	workerInitErr error
	workerRuntime *composition.Runtime
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, event events.DynamoDBEvent) (events.DynamoDBEventResponse, error) {
	workerOnce.Do(func() {
		log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
		cfg, err := configuration.Load()
		if err != nil {
			log.Error("worker init failed", "err", err)
			workerInitErr = err
			return
		}
		workerRuntime, err = composition.Build(ctx, log, cfg, composition.Options{
			EnableJobStore: true,
			LeaseOwner:     "worker",
		})
		if err != nil {
			log.Error("worker init failed", "err", err)
			workerInitErr = err
		}
	})
	if workerInitErr != nil {
		return events.DynamoDBEventResponse{}, errors.New("worker unavailable")
	}
	resp := events.DynamoDBEventResponse{
		BatchItemFailures: make([]events.DynamoDBBatchItemFailure, 0),
	}
	for _, record := range event.Records {
		if !shouldHandleRecord(record) {
			continue
		}
		jobID, ok := streamString(record.Change.NewImage, "job_id")
		if !ok {
			continue
		}
		id, err := uuid.Parse(jobID)
		if err != nil {
			continue
		}
		if err := workerRuntime.Execution.HandleStreamRecord(ctx, id, time.Now().UTC()); err != nil && !errors.Is(err, driven.ErrJobConflict) {
			resp.BatchItemFailures = append(resp.BatchItemFailures, events.DynamoDBBatchItemFailure{
				ItemIdentifier: record.Change.SequenceNumber,
			})
		}
	}
	return resp, nil
}

func shouldHandleRecord(record events.DynamoDBEventRecord) bool {
	entityType, ok := streamString(record.Change.NewImage, "entity_type")
	if !ok || entityType != "job" {
		return false
	}
	status, ok := streamString(record.Change.NewImage, "status")
	if !ok {
		return false
	}
	return status == driven.JobStatusPending || status == driven.JobStatusRunning
}

func streamString(values map[string]events.DynamoDBAttributeValue, key string) (string, bool) {
	value, ok := values[key]
	if !ok || value.DataType() != events.DataTypeString {
		return "", false
	}
	return value.String(), true
}
