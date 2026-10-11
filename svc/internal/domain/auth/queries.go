package auth

import (
	"context"
	"github.com/google/uuid"
)

type UsersQueries interface {
	GetUserProfile(ctx context.Context, user uuid.UUID) (*UserProfile, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (email string, err error)
	GetHomeOrganisationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
}
