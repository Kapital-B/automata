package decisions

import (
	"context"
	"github.com/google/uuid"
	"time"
)

// DecisionRow is a project decision (proposed through withdrawn).
type DecisionRow struct {
	ID                   uuid.UUID
	OrganisationID       uuid.UUID
	ProjectID            uuid.UUID
	IssueID              *uuid.UUID
	Statement            string
	Status               string
	DecidedAt            *time.Time
	AssigneeUserID       *uuid.UUID
	AssigneeContactID    *uuid.UUID
	Source               string
	Confidence           *float64
	SupersedesDecisionID *uuid.UUID
	CreatedByUserID      *uuid.UUID
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// DecisionEvidenceRow links correspondence to a decision.
type DecisionEvidenceRow struct {
	ID           uuid.UUID
	DecisionID   uuid.UUID
	MessageID    *uuid.UUID
	ManualItemID *uuid.UUID
	AddedAt      time.Time
}

// DecisionRepository persists decisions and evidence.
type DecisionRepository interface {
	CreateDecision(ctx context.Context, row DecisionRow) error
	GetDecision(ctx context.Context, organisationID, id uuid.UUID) (*DecisionRow, error)
	ListDecisionsByProject(ctx context.Context, organisationID, projectID uuid.UUID, status string) ([]DecisionRow, error)
	UpdateDecision(ctx context.Context, row DecisionRow) error
	AddDecisionEvidence(ctx context.Context, row DecisionEvidenceRow) error
	RemoveDecisionEvidence(ctx context.Context, organisationID, decisionID, evidenceID uuid.UUID) error
	ListDecisionEvidence(ctx context.Context, decisionID uuid.UUID) ([]DecisionEvidenceRow, error)
}
