package messages

import (
	"context"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	"github.com/google/uuid"
)

type AccountJobSubmission interface {
	RescanAssignments(ctx context.Context, user uuid.UUID, account *uuid.UUID) ([]uuid.UUID, error)
	SubmitAccount(ctx context.Context, user, account uuid.UUID, jobType string, payload domainjobs.JobPayload) (*domainjobs.Submission, error)
}
