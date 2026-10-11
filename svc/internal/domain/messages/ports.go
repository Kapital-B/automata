package messages

import (
	"context"

	"github.com/google/uuid"
	"time"
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

type CategoryDefinitionRow struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Slug        string
	DisplayName string
	Definition  string
	SortOrder   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type MessageCategoryRow struct {
	ID         uuid.UUID
	MessageID  uuid.UUID
	AccountID  uuid.UUID
	CategoryID uuid.UUID
	Source     string
	Confidence *float64
	RunID      uuid.UUID
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type MessageListFilter struct {
	AccountID         *uuid.UUID
	ProjectID         *uuid.UUID
	Category          string
	Since             *time.Time
	OnlySummaryUnseen bool
	OmitBody          bool
	Limit             int
	Offset            int
	// BeforeReceivedAt/BeforeID page a received_at DESC, id DESC scan from a
	// stable position. Prefer these over Offset for jobs, whose result set
	// mutates between chunks.
	BeforeReceivedAt *time.Time
	BeforeID         *uuid.UUID
}

type SummarySettingsRow struct {
	UserID               uuid.UUID
	IncludeCategorySlugs []string
	ExcludeCategorySlugs []string
	ChunkSize            int
	UpdatedAt            time.Time
}

type SummarySnapshotRow struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	AccountID      *uuid.UUID
	RunID          uuid.UUID
	WindowStart    time.Time
	WindowEnd      time.Time
	GeneralSummary string
	CreatedAt      time.Time
}

type SummaryJobChunkRow struct {
	ID          uuid.UUID
	RunID       uuid.UUID
	AccountID   uuid.UUID
	ChunkIndex  int
	Phase       string
	MessageIDs  []uuid.UUID
	PayloadJSON string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ActionItemRow struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	AccountID       uuid.UUID
	MessageID       uuid.UUID
	RunID           uuid.UUID
	Text            string
	DueAt           *time.Time
	Status          string
	ActionedAt      *time.Time
	AutoDraftSeenAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ProjectTodoRow is an open to-do on a project, with whose it is and the
// project issue its message is on, if any.
type ProjectTodoRow struct {
	Item       ActionItemRow
	OwnerEmail string
	IssueID    *uuid.UUID
	IssueTitle string
}

type FYIRow struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	AccountID uuid.UUID
	MessageID uuid.UUID
	RunID     uuid.UUID
	Text      string
	CreatedAt time.Time
}

type DraftSuggestionRow struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	AccountID    uuid.UUID
	MessageID    uuid.UUID
	ActionItemID uuid.UUID
	RunID        uuid.UUID
	Subject      string
	Body         string
	Model        string
	FromJSON     string
	Status       string
	SentAt       *time.Time
	DiscardedAt  *time.Time
	UpdatedAt    *time.Time
	CreatedAt    time.Time
}

type SendAttemptRow struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	AccountID         uuid.UUID
	DraftID           uuid.UUID
	MessageID         uuid.UUID
	Status            string
	ProviderMessageID *string
	ErrorMessage      *string
	CreatedAt         time.Time
}

type ForwardAllowlistRow struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Email     string
	CreatedAt time.Time
}

type ForwardRuleRow struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	AccountID     uuid.UUID
	Name          string
	Mode          string
	ConditionJSON string
	ForwardTo     string
	Enabled       bool
	// ApplyFrom is the earliest received time a rule considers. It is set
	// when the rule is switched on — to that moment for "new mail only", or
	// to the epoch to include existing mail — so switching a rule on never
	// quietly reaches back through the whole mailbox. Nil is treated as the
	// rule's creation time.
	ApplyFrom *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ForwardAuditRow is a rule's verdict on one message. One row per (message,
// rule): it is how the engine knows a rule has finished with a message.
type ForwardAuditRow struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	AccountID uuid.UUID
	MessageID uuid.UUID
	RuleID    uuid.UUID
	RunID     uuid.UUID
	Status    string // forwarded | skipped | failed
	Reason    *string
	// Pending marks a verdict that is not final: a send that did not go out
	// and will be retried, or a category rule waiting for the message to be
	// categorised. A later run looks at the message again.
	Pending bool
	// Attempts counts sends that failed without going out, so a message that
	// can never be sent is given up on rather than retried forever.
	Attempts  int
	CreatedAt time.Time
}

// ForwardCandidateCursor is the keyset forward runs page by: oldest first,
// so mail that arrives mid-run lands after the cursor rather than shifting it.
type ForwardCandidateCursor struct {
	ReceivedAt time.Time
	MessageID  uuid.UUID
}

// ForwardRuleStats summarises what a rule has done.
type ForwardRuleStats struct {
	RuleID          uuid.UUID
	Forwarded       int
	Failed          int
	Pending         int
	LastForwardedAt *time.Time
	LastActivityAt  *time.Time
}

// ForwardActivityRow is one audit verdict with enough of the message to
// recognise it.
type ForwardActivityRow struct {
	ForwardAuditRow
	Subject    string
	FromJSON   string
	ReceivedAt time.Time
}

type ManualForwardAuditRow struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	AccountID uuid.UUID
	MessageID uuid.UUID
	ToEmail   string
	Comment   *string
	Status    string
	Reason    *string
	CreatedAt time.Time
}

// MessageRepository stores Graph messages.
type MessageRepository interface {
	UpsertMessage(ctx context.Context, m MessageRow) error
	GetMessageIDByProvider(ctx context.Context, accountID uuid.UUID, providerMessageID string) (uuid.UUID, error)
	ListMessagesByAccount(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, limit, offset int) ([]MessageRow, error)
	ListMessages(ctx context.Context, userID uuid.UUID, filter MessageListFilter) ([]MessageRow, error)
	GetMessage(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*MessageRow, error)
	MarkMessagesSummarySeen(ctx context.Context, userID uuid.UUID, messageIDs []uuid.UUID, at time.Time) error
	// ListMessagesNeedingContactResolution returns messages on an account
	// whose From/To/Cc have not been turned into contacts yet, oldest first so
	// a backlog drains in the order it arrived.
	ListMessagesNeedingContactResolution(ctx context.Context, userID, accountID uuid.UUID, limit int) ([]MessageRow, error)
	// MarkContactsResolved records that these messages have been looked at,
	// whether or not they yielded a contact, so one with no usable addresses
	// is not retried on every run.
	MarkContactsResolved(ctx context.Context, userID uuid.UUID, messageIDs []uuid.UUID, at time.Time) error
	UpsertMessageCategory(ctx context.Context, row MessageCategoryRow) error
	ListCategoryDefinitions(ctx context.Context, userID uuid.UUID) ([]CategoryDefinitionRow, error)
	GetCategoryDefinitionBySlug(ctx context.Context, userID uuid.UUID, slug string) (*CategoryDefinitionRow, error)
	GetCategoryDefinitionByID(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*CategoryDefinitionRow, error)
	CreateCategoryDefinition(ctx context.Context, row CategoryDefinitionRow) error
	UpdateCategoryDefinition(ctx context.Context, row CategoryDefinitionRow) error
	DeleteCategoryDefinition(ctx context.Context, userID uuid.UUID, id uuid.UUID) error
	ReassignMessageCategories(ctx context.Context, userID uuid.UUID, fromCategoryID, toCategoryID uuid.UUID) (int, error)
	CountMessageCategoriesByCategory(ctx context.Context, userID uuid.UUID, categoryID uuid.UUID) (int, error)
}

type SummaryRepository interface {
	GetSummarySettings(ctx context.Context, userID uuid.UUID) (*SummarySettingsRow, error)
	UpsertSummarySettings(ctx context.Context, row SummarySettingsRow) error
	InsertSummarySnapshot(ctx context.Context, row SummarySnapshotRow) error
	ListSummarySnapshots(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]SummarySnapshotRow, error)
	UpsertSummaryJobChunk(ctx context.Context, row SummaryJobChunkRow) error
	ListSummaryJobChunks(ctx context.Context, runID uuid.UUID) ([]SummaryJobChunkRow, error)
	InsertActionItems(ctx context.Context, rows []ActionItemRow) error
	ListOpenActionItems(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID) ([]ActionItemRow, error)
	MarkActionItemDone(ctx context.Context, userID uuid.UUID, itemID uuid.UUID, at time.Time) error
	// ListOpenActionItemsForProject returns every open to-do filed to the
	// project, whoever's mailbox it came from, as long as its owner is a
	// member. Mail filed nowhere but on one of the project's issues counts
	// as filed to the project.
	ListOpenActionItemsForProject(ctx context.Context, organisationID, projectID uuid.UUID) ([]ProjectTodoRow, error)
	// CompleteActionItem marks a to-do done on behalf of byUserID, who need
	// not own it. Callers authorise; this only records.
	CompleteActionItem(ctx context.Context, itemID, byUserID uuid.UUID, at time.Time) error
	InsertFYI(ctx context.Context, rows []FYIRow) error
	ListFYIByRun(ctx context.Context, userID uuid.UUID, runID uuid.UUID) ([]FYIRow, error)
	ListOpenFYI(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]FYIRow, error)
	DeleteFYI(ctx context.Context, userID uuid.UUID, id uuid.UUID) error
	ListActionItemsForAutoDraft(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, onlyUnseen bool, limit int) ([]ActionItemRow, error)
	MarkActionItemsAutoDraftSeen(ctx context.Context, userID uuid.UUID, itemIDs []uuid.UUID, at time.Time) error
	InsertDraftSuggestions(ctx context.Context, rows []DraftSuggestionRow) error
	ListDraftSuggestions(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]DraftSuggestionRow, error)
	GetDraftSuggestion(ctx context.Context, userID uuid.UUID, draftID uuid.UUID) (*DraftSuggestionRow, error)
	UpdateDraftSuggestion(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, subject, body string, at time.Time) error
	MarkDraftSuggestionStatus(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, status string, at time.Time) error
	InsertSendAttempt(ctx context.Context, row SendAttemptRow) error
	ListSendAttemptsByDraft(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, limit int) ([]SendAttemptRow, error)
}

type ForwardRepository interface {
	ListForwardAllowlist(ctx context.Context, userID uuid.UUID) ([]ForwardAllowlistRow, error)
	ReplaceForwardAllowlist(ctx context.Context, userID uuid.UUID, emails []string) error
	ListForwardRules(ctx context.Context, userID, accountID uuid.UUID) ([]ForwardRuleRow, error)
	CreateForwardRule(ctx context.Context, row ForwardRuleRow) error
	// UpdateForwardRule replaces a rule's settings. A nil ApplyFrom keeps the
	// rule's current start.
	UpdateForwardRule(ctx context.Context, row ForwardRuleRow) error
	DeleteForwardRule(ctx context.Context, userID, ruleID uuid.UUID) error
	ListForwardAuditByRun(ctx context.Context, userID, runID uuid.UUID) ([]ForwardAuditRow, error)
	// InsertForwardAudit upserts the verdict for (message, rule).
	InsertForwardAudit(ctx context.Context, row ForwardAuditRow) error
	// ListForwardCandidates returns messages at least one of ruleIDs has not
	// finished with: in the rule's scope (received at or after its
	// ApplyFrom) and without a final verdict. Oldest first, after cursor.
	ListForwardCandidates(ctx context.Context, userID, accountID uuid.UUID, ruleIDs []uuid.UUID, after *ForwardCandidateCursor, limit int) ([]MessageRow, error)
	// ListForwardAuditForMessages returns every verdict on these messages.
	ListForwardAuditForMessages(ctx context.Context, userID uuid.UUID, messageIDs []uuid.UUID) ([]ForwardAuditRow, error)
	// ForwardRuleStats summarises each rule on the account.
	ForwardRuleStats(ctx context.Context, userID, accountID uuid.UUID) ([]ForwardRuleStats, error)
	// ListForwardActivity returns a rule's most recent verdicts, newest first.
	ListForwardActivity(ctx context.Context, userID, ruleID uuid.UUID, limit int) ([]ForwardActivityRow, error)
	InsertManualForwardAudit(ctx context.Context, row ManualForwardAuditRow) error
}

// MessageRow is a stored inbox message.
type MessageRow struct {
	ID                 uuid.UUID
	AccountID          uuid.UUID
	ProviderMessageID  string
	ConversationID     *string
	ReceivedAt         time.Time
	Subject            string
	FromJSON           string
	ToJSON             string
	CcJSON             string
	ToCCPreview        *string
	BodyText           *string
	BodyFetchedAt      *time.Time
	HasAttachments     bool
	RawEtag            *string
	CategorySlug       *string
	CategoryConfidence *float64
	SummarySeenAt      *time.Time
	ForwardSeenAt      *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
