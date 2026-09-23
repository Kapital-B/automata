package driven

import (
	"context"
	"errors"
	"fmt"

	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
)

// Errors a mailbox reports so callers can act on the cause rather than the
// provider's wording.
var (
	// ErrMailNotSent means nothing was handed to the provider, so retrying
	// cannot deliver a duplicate. Anything else from a send is "maybe sent".
	ErrMailNotSent = errors.New("mail not sent")
	// ErrMailTooLarge is a permanent ErrMailNotSent: retrying will not help.
	ErrMailTooLarge = errors.New("message too large to send")
	// ErrCursorExpired means the provider no longer recognises a resume
	// cursor and a full enumeration is needed.
	ErrCursorExpired = errors.New("mailbox cursor expired")
	// ErrCredentialsRejected means the provider refused the stored
	// credentials outright — revoked, expired or wrong — and the account
	// needs reconnecting rather than retrying.
	ErrCredentialsRejected = errors.New("mailbox credentials rejected")
	// ErrUnsupportedProvider means no adapter is registered for an account.
	ErrUnsupportedProvider = errors.New("unsupported mail provider")
)

// TooLarge reports a message over a send limit. It is both not-sent (nothing
// was handed over) and permanent (retrying will not shrink it), so callers
// test ErrMailTooLarge before ErrMailNotSent.
func TooLarge(limit int64) error {
	return fmt.Errorf("%w: %w: over %d bytes", ErrMailNotSent, ErrMailTooLarge, limit)
}

// MailboxCapabilities declares what a provider does natively. It exists so
// callers can show the difference, not so they can refuse: every mailbox
// implements every operation, some by a slower route.
type MailboxCapabilities struct {
	// IncrementalSync is false when every run re-lists the folder.
	IncrementalSync bool
	// ServerSideForward is false when Forward fetches the original and
	// re-sends it, which changes deliverability and moves attachment bytes
	// through our infrastructure.
	ServerSideForward bool
	// ServerSideReply is false when Reply composes a new message.
	ServerSideReply bool
	// ReportsRemovals is false when the provider cannot tell us a message
	// left the folder.
	ReportsRemovals bool
	// StableMessageIDs is false when ids can change under us.
	StableMessageIDs bool
}

// MailMessage is one message as a mailbox reports it. A MailChangePage only
// ever carries full payloads in Messages; removals travel separately.
type MailMessage struct {
	ID               string
	ConversationID   string
	ReceivedDateTime string
	Subject          string
	FromName         string
	FromAddress      string
	ToRecipients     []MailRecipient
	CcRecipients     []MailRecipient
	BodyPreview      string
	BodyContent      string
	BodyContentType  string // Text or HTML
	HasAttachments   bool
	ChangeKey        string
}

// MailRecipient is a To/Cc address.
type MailRecipient struct {
	Name    string
	Address string
}

// MailChangePage is one page of changes. It either continues (NextCursor) or
// completes (FinalCursor), never both. Removed lists ids that left the folder:
// a removal is never encoded as a message with empty fields, which is how a
// Graph tombstone once blanked a real row.
type MailChangePage struct {
	Messages    []MailMessage
	Removed     []string
	NextCursor  string
	FinalCursor string
}

// Mailbox is an opened, authenticated mailbox for one account.
type Mailbox interface {
	Capabilities() MailboxCapabilities
	// ListChanges returns one page. An empty cursor starts a full enumeration.
	ListChanges(ctx context.Context, cursor string, pageSize int) (*MailChangePage, error)
	// GetRawMessage returns the full RFC 5322 message.
	GetRawMessage(ctx context.Context, providerMessageID string) ([]byte, error)
	Reply(ctx context.Context, providerMessageID, body string) error
	Forward(ctx context.Context, providerMessageID, to, comment string) error
}

// MailProvider turns an account's stored credentials into an open mailbox.
// Credentials are opaque to everything above the provider.
type MailProvider interface {
	// Open returns a ready mailbox. rotated is non-nil when the provider
	// issued replacement credentials that must be persisted.
	Open(ctx context.Context, account AccountRow, credential []byte) (box Mailbox, rotated []byte, err error)
}

// ConnectOptions carries provider-specific connect choices.
type ConnectOptions struct {
	MsAccountKind accounts.MsAccountKind
}

// ConnectedMailbox is what a successful connect learned about the mailbox.
type ConnectedMailbox struct {
	Email         string
	DefaultLabel  string
	TenantID      *string
	MsAccountKind accounts.MsAccountKind
	// Credential is plaintext; the caller encrypts it.
	Credential []byte
}

// OAuthMailConnector connects a mailbox through an OAuth consent flow.
type OAuthMailConnector interface {
	AuthorizationURL(ctx context.Context, state string, opts ConnectOptions) (string, error)
	Complete(ctx context.Context, code string, opts ConnectOptions) (*ConnectedMailbox, error)
}
