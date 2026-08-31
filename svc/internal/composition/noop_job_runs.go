package composition

import (
	"context"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

type noopJobRuns struct{}

func resolveJobRuns(repo any) driven.JobRunRepository {
	if jobRuns, ok := repo.(driven.JobRunRepository); ok {
		return jobRuns
	}
	return noopJobRuns{}
}

func (noopJobRuns) InsertJobRun(context.Context, uuid.UUID, uuid.UUID, string, string, string, time.Time, time.Time, *string, string) error {
	return nil
}

func (noopJobRuns) PromoteJobRunToRunning(context.Context, uuid.UUID, time.Time) error {
	return nil
}

func (noopJobRuns) UpdateJobRunMeta(context.Context, uuid.UUID, string) error {
	return nil
}

func (noopJobRuns) UpdateJobRunStatus(context.Context, uuid.UUID, string, *time.Time, *string, string) error {
	return nil
}

func (noopJobRuns) ListJobRuns(context.Context, uuid.UUID, driven.JobRunListFilter) ([]driven.JobRunRow, error) {
	return []driven.JobRunRow{}, nil
}

func (noopJobRuns) GetJobRun(context.Context, uuid.UUID, uuid.UUID) (*driven.JobRunRow, error) {
	return nil, nil
}
