package accounts

import (
	"context"
	"github.com/google/uuid"
)

type AccountsQueries interface {
	GetScopedAccount(ctx context.Context, user, id uuid.UUID) (*AccountRow, error)
	OwnsAccount(ctx context.Context, user, id uuid.UUID) (bool, error)
	GetAccount(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*AccountRow, []byte, error)
	ListAccounts(ctx context.Context, userID uuid.UUID) ([]AccountRow, error)
}
