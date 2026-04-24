package driven

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AuthSessionRepository stores opaque refresh tokens (SHA-256 hash at rest).
type AuthSessionRepository interface {
	InsertAuthSession(ctx context.Context, sessionID, userID uuid.UUID, tokenHash string, createdAt, expiresAt time.Time) error
	// ConsumeAuthSession deletes the row if token_hash matches and expires_at is in the future; returns user id.
	ConsumeAuthSession(ctx context.Context, tokenHash string) (userID uuid.UUID, ok bool, err error)
}
