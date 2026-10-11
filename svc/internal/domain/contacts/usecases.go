package contacts

import (
	context "context"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	uuid "github.com/google/uuid"
	time "time"
)

// ContactRecentMessageRef is enough of a message to recognise it in a list.
type ContactRecentMessageRef struct {
	MessageID   uuid.UUID
	AccountID   uuid.UUID
	Subject     string
	FromName    string
	FromAddress string
	ReceivedAt  time.Time
}

type ContactResolveChunkResult struct {
	MessagesProcessed int
	NextCursor        *domainjobs.JobCursor
	Done              bool
}

type ContactContactDetail struct {
	Contact         ContactRow
	Identities      []ContactIdentityRow
	RecentMessages  []ContactRecentMessageRef
	SuggestedMerges []ContactRow
}

type ContactCreateContactInput struct {
	DisplayName string
	Company     *string
	Identities  []struct {
		Kind  string
		Value string
	}
}

type ContactResolveUseCases interface {
	ResolveMessage(ctx context.Context, userID, messageID uuid.UUID) error
	BackfillAccount(ctx context.Context, userID, accountID uuid.UUID) error
	ResolveAccountChunk(ctx context.Context, run domainjobs.RunContext) (*ContactResolveChunkResult, error)
}

type ContactUseCases interface {
	List(ctx context.Context, userID uuid.UUID, filter ContactListFilter) ([]ContactRow, error)
	Get(ctx context.Context, userID, contactID uuid.UUID) (*ContactContactDetail, error)
	Create(ctx context.Context, userID uuid.UUID, in ContactCreateContactInput) (*ContactRow, error)
	AddIdentity(ctx context.Context, userID, contactID uuid.UUID, kind, value string) (*ContactIdentityRow, error)
	Merge(ctx context.Context, userID, survivorID, sourceID uuid.UUID) error
}
