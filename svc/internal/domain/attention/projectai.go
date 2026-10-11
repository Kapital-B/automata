package attention

import (
	"context"
	"github.com/google/uuid"
)

type AttentionProjects interface {
	ProjectIDsNeedingInput(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]struct{}, error)
}
