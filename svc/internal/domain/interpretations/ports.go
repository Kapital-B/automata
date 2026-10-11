package interpretations

import (
	"context"
	"github.com/google/uuid"
	"time"
)

// InterpretationRow is one LLM interpret run (candidates in payload).
type InterpretationRow struct {
	ID             uuid.UUID
	OrganisationID uuid.UUID
	ProjectID      uuid.UUID
	AccountID      *uuid.UUID
	RunID          *uuid.UUID
	Status         string
	PayloadJSON    string
	Confidence     *float64
	Reason         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// InterpretationSourceRow links correspondence used as interpret input.
type InterpretationSourceRow struct {
	ID                 uuid.UUID
	InterpretationID   uuid.UUID
	MessageID          *uuid.UUID
	ManualItemID       *uuid.UUID
	ConnectorMessageID *uuid.UUID
}

// InterpretationRepository persists interpretation candidates and sources.
type InterpretationRepository interface {
	CreateInterpretation(ctx context.Context, row InterpretationRow) error
	AddInterpretationSource(ctx context.Context, row InterpretationSourceRow) error
	GetInterpretation(ctx context.Context, organisationID, id uuid.UUID) (*InterpretationRow, error)
	ListPendingInterpretations(ctx context.Context, organisationID, projectID uuid.UUID) ([]InterpretationRow, error)
	UpdateInterpretationStatus(ctx context.Context, organisationID, id uuid.UUID, status string, updatedAt time.Time) error
	ListInterpretationSources(ctx context.Context, interpretationID uuid.UUID) ([]InterpretationSourceRow, error)
}
