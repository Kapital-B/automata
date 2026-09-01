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
	workerMu      sync.Mutex
	workerRuntime *composition.Runtime
	workerLog     *slog.Logger
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, event events.DynamoDBEvent) (events.DynamoDBEventResponse, error) {
	log := workerLogger()
	start := time.Now().UTC()
	rt, err := ensureWorker(ctx)
	if err != nil {
		log.Error("worker unavailable", "err", err, "records", len(event.Records))
		return events.DynamoDBEventResponse{}, errors.New("worker unavailable")
	}
	resp := events.DynamoDBEventResponse{
		BatchItemFailures: make([]events.DynamoDBBatchItemFailure, 0),
	}
	handled, skipped, conflicts, failed := 0, 0, 0, 0
	for _, record := range event.Records {
		entityType, _ := streamString(record.Change.NewImage, "entity_type")
		status, _ := streamString(record.Change.NewImage, "status")
		jobType, _ := streamString(record.Change.NewImage, "job_type")
		if !shouldHandleRecord(record) {
			skipped++
			log.Info("worker skip record",
				"event_name", record.EventName,
				"entity_type", entityType,
				"status", status,
				"sequence", record.Change.SequenceNumber,
			)
			continue
		}
		jobID, ok := streamString(record.Change.NewImage, "job_id")
		if !ok {
			skipped++
			log.Warn("worker skip record missing job_id", "sequence", record.Change.SequenceNumber)
			continue
		}
		id, err := uuid.Parse(jobID)
		if err != nil {
			skipped++
			log.Warn("worker skip record invalid job_id", "job_id", jobID, "err", err)
			continue
		}
		log.Info("worker handle record",
			"job_id", id,
			"job_type", jobType,
			"status", status,
			"event_name", record.EventName,
			"sequence", record.Change.SequenceNumber,
		)
		if err := rt.Execution.HandleStreamRecord(ctx, id, time.Now().UTC()); err != nil {
			if errors.Is(err, driven.ErrJobConflict) {
				conflicts++
				log.Info("worker job conflict", "job_id", id, "status", status)
				continue
			}
			failed++
			log.Error("worker handle stream record failed", "job_id", id, "job_type", jobType, "status", status, "err", err)
			resp.BatchItemFailures = append(resp.BatchItemFailures, events.DynamoDBBatchItemFailure{
				ItemIdentifier: record.Change.SequenceNumber,
			})
			continue
		}
		handled++
	}
	log.Info("worker invoke complete",
		"records", len(event.Records),
		"handled", handled,
		"skipped", skipped,
		"conflicts", conflicts,
		"failed", failed,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return resp, nil
}

func ensureWorker(ctx context.Context) (*composition.Runtime, error) {
	workerMu.Lock()
	defer workerMu.Unlock()
	if workerRuntime != nil {
		return workerRuntime, nil
	}

	log := workerLogger()
	cfg, err := configuration.Load()
	if err != nil {
		log.Error("worker init failed", "err", err)
		return nil, err
	}
	rt, err := composition.Build(ctx, log, cfg, composition.Options{
		EnableJobStore: true,
		LeaseOwner:     "worker",
	})
	if err != nil {
		log.Error("worker init failed", "err", err)
		return nil, err
	}
	log.Info("worker init ok", "jobs_table", cfg.JobsTableName, "database_engine", cfg.DatabaseEngine)
	workerRuntime = rt
	return workerRuntime, nil
}

func workerLogger() *slog.Logger {
	if workerLog == nil {
		workerLog = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return workerLog
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
