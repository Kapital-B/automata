package projects

import (
	"context"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	"github.com/google/uuid"
)

type ExtractionSubmission interface {
	SubmitExtraction(ctx context.Context, user, project uuid.UUID) (*domainjobs.Submission, error)
}
