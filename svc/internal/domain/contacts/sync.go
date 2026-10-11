package contacts

import (
	"context"
	"github.com/google/uuid"
)

type AccountResolver interface {
	BackfillAccount(ctx context.Context, userID, accountID uuid.UUID) error
}
