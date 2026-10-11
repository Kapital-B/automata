package attention

import (
	"context"
	"github.com/google/uuid"
)

type AttentionCounts interface {
	CountsForUser(ctx context.Context, userID uuid.UUID) (total int, byProject map[uuid.UUID]int, err error)
}
