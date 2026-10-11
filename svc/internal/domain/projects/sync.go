package projects

import (
	"context"
	"github.com/google/uuid"
)

type SyncAssigner interface {
	AssignAfterSync(ctx context.Context, userID, accountID uuid.UUID) error
}
