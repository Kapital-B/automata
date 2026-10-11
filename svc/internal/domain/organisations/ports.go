package organisations

import (
	"context"
	"github.com/google/uuid"
	"time"
)

// OrganisationRow is a persisted organisation.
type OrganisationRow struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// OrganisationRepository reads organisations and membership.
type OrganisationRepository interface {
	GetOrganisation(ctx context.Context, id uuid.UUID) (*OrganisationRow, error)
}
