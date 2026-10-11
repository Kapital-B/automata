package contacts

import (
	"context"
	"github.com/google/uuid"
	"time"
)

// ContactRow is a persisted address-book contact.
type ContactRow struct {
	ID                  uuid.UUID
	OrganisationID      uuid.UUID
	DisplayName         string
	Company             *string
	MergedIntoContactID *uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
	// PrimaryEmail is the contact's earliest email identity. Only
	// ListContacts fills it, so a list can say who someone is without a
	// round trip per row.
	PrimaryEmail string
}

// ContactIdentityRow is one identity on a contact.
type ContactIdentityRow struct {
	ID              uuid.UUID
	OrganisationID  uuid.UUID
	ContactID       uuid.UUID
	Kind            string
	ValueNormalized string
	ValueRaw        string
	CreatedAt       time.Time
}

// CorrespondenceParticipantRow links a contact to a message (or later manual item).
type CorrespondenceParticipantRow struct {
	ID             uuid.UUID
	OrganisationID uuid.UUID
	ContactID      uuid.UUID
	Role           string
	MessageID      *uuid.UUID
	ManualItemID   *uuid.UUID
}

// ContactListFilter narrows contact listing within one organisation.
type ContactListFilter struct {
	Query  string
	Limit  int
	Offset int
}

// ContactRepository persists org-scoped contacts and mail participants.
type ContactRepository interface {
	ListContacts(ctx context.Context, organisationID uuid.UUID, filter ContactListFilter) ([]ContactRow, error)
	GetContact(ctx context.Context, organisationID, contactID uuid.UUID) (*ContactRow, error)
	CreateContact(ctx context.Context, row ContactRow) error
	UpdateContactDisplayNameIfEmpty(ctx context.Context, organisationID, contactID uuid.UUID, displayName string, at time.Time) error
	ListIdentities(ctx context.Context, organisationID, contactID uuid.UUID) ([]ContactIdentityRow, error)
	FindContactIdentity(ctx context.Context, organisationID uuid.UUID, kind, valueNormalized string) (*ContactIdentityRow, error)
	CreateIdentity(ctx context.Context, row ContactIdentityRow) error
	UpsertParticipant(ctx context.Context, row CorrespondenceParticipantRow) error
	ListRecentMessageIDs(ctx context.Context, organisationID, contactID uuid.UUID, limit int) ([]uuid.UUID, error)
	SuggestMerges(ctx context.Context, organisationID, contactID uuid.UUID) ([]ContactRow, error)
	MergeContacts(ctx context.Context, organisationID, survivorID, sourceID uuid.UUID, at time.Time) error
	// ResolveEmailContact finds or creates a contact for an email in the organisation.
	ResolveEmailContact(ctx context.Context, organisationID uuid.UUID, email, displayName string, now time.Time) (contactID uuid.UUID, err error)
	ListContactIDsForMessage(ctx context.Context, organisationID, messageID uuid.UUID) ([]uuid.UUID, error)
	ListContactIDsForThread(ctx context.Context, organisationID, accountID uuid.UUID, conversationID string) ([]uuid.UUID, error)
	ListContactIDsForManualItem(ctx context.Context, organisationID, manualItemID uuid.UUID) ([]uuid.UUID, error)
}
