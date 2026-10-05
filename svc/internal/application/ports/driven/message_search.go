package driven

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MessageSearchFilter narrows a keyword search over a user's own mail.
type MessageSearchFilter struct {
	// Terms are matched case-insensitively as substrings of the subject and
	// body. A message must match at least one. Callers lowercase them.
	Terms []string
	// ProjectIDs keeps messages whose effective project is any of these.
	ProjectIDs []uuid.UUID
	// ContactIDs keeps messages any of these contacts took part in.
	ContactIDs []uuid.UUID
	Since      *time.Time
	Limit      int
}

// MessageSearchHit is one matching message. Score ranks hits: a term in the
// subject counts 2, in the body 1, summed over terms.
type MessageSearchHit struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	ConversationID *string
	ReceivedAt     time.Time
	Subject        string
	FromJSON       string
	BodyText       *string
	Score          int
}

type MessageSearchRepository interface {
	// SearchMessages returns the best hits, highest score first, then newest.
	// With no terms, it returns the newest messages in scope; with no terms,
	// projects or contacts at all, it returns nothing rather than all mail.
	SearchMessages(ctx context.Context, userID uuid.UUID, filter MessageSearchFilter) ([]MessageSearchHit, error)
}
