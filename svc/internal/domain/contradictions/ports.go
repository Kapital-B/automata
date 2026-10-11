package contradictions

import (
	"context"
	"github.com/google/uuid"
	"time"
)

// ContradictionRow is an open or resolved conflict between claims.
type ContradictionRow struct {
	ID               uuid.UUID
	OrganisationID   uuid.UUID
	ProjectID        uuid.UUID
	Status           string
	Summary          string
	ResolutionNote   *string
	ResolvedAt       *time.Time
	ResolvedByUserID *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ContradictionSideRow links one side of a contradiction.
type ContradictionSideRow struct {
	ID              uuid.UUID
	ContradictionID uuid.UUID
	FactVersionID   *uuid.UUID
	DecisionID      *uuid.UUID
}

// ContradictionRepository persists contradictions.
type ContradictionRepository interface {
	CreateContradiction(ctx context.Context, row ContradictionRow) error
	AddContradictionSide(ctx context.Context, row ContradictionSideRow) error
	GetContradiction(ctx context.Context, organisationID, id uuid.UUID) (*ContradictionRow, error)
	ListContradictionsByProject(ctx context.Context, organisationID, projectID uuid.UUID, status string) ([]ContradictionRow, error)
	UpdateContradiction(ctx context.Context, row ContradictionRow) error
	ListContradictionSides(ctx context.Context, contradictionID uuid.UUID) ([]ContradictionSideRow, error)
}
