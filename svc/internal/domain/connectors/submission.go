package connectors

import (
	"context"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	"github.com/google/uuid"
)

type SyncSubmission interface {
	SubmitConnector(ctx context.Context, user, account uuid.UUID) (*domainjobs.Submission, error)
}
