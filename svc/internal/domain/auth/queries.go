package auth

import (
	"context"
	"github.com/google/uuid"
)

type UsersQueries interface {
	GetUserProfile(ctx context.Context, user uuid.UUID) (*UserProfile, error)
}
