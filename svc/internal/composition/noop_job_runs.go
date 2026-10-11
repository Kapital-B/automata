package composition

import (
	"context"
	"time"

	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	"github.com/google/uuid"
)

type noopJobRuns struct{}

func resolveJobRuns(repo any) domainjobs.JobRunRepository {
	if jobRuns, ok := repo.(domainjobs.JobRunRepository); ok {
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

func (noopJobRuns) ListJobRuns(context.Context, uuid.UUID, domainjobs.JobRunListFilter) ([]domainjobs.JobRunRow, error) {
	return []domainjobs.JobRunRow{}, nil
}

func (noopJobRuns) GetJobRun(context.Context, uuid.UUID, uuid.UUID) (*domainjobs.JobRunRow, error) {
	return nil, nil
}
