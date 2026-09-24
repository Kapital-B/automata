package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domaincontacts "github.com/Kapital-B/automata/svc/internal/domain/contacts"
	"github.com/google/uuid"
)

// ResolveService upserts contacts and participants from mail headers.
type ResolveService struct {
	Users    driven.UserRepository
	Messages driven.MessageRepository
	Contacts driven.ContactRepository
}

type recipientPayload struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type ResolveChunkResult struct {
	MessagesProcessed int
	NextCursor        *driven.JobCursor
	Done              bool
}

// ResolveMessage resolves From/To/Cc on one stored message into the user's home organisation.
func (s *ResolveService) ResolveMessage(ctx context.Context, userID, messageID uuid.UUID) error {
	orgID, err := s.Users.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		return err
	}
	msg, err := s.Messages.GetMessage(ctx, userID, messageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return fmt.Errorf("message not found")
	}
	return s.resolveStoredMessage(ctx, orgID, msg)
}

// BackfillAccount resolves contacts for existing messages on an account (best-effort).
func (s *ResolveService) BackfillAccount(ctx context.Context, userID, accountID uuid.UUID) error {
	orgID, err := s.Users.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		return err
	}
	// Inline fallback for a deployment with no queue: drain the same backlog
	// the job would, in batches, rather than re-reading the whole mailbox.
	for {
		_, done, err := s.resolveNextBatch(ctx, orgID, userID, accountID)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}

// resolveChunkSize bounds one chunk of contact resolution.
const resolveChunkSize = 100

func (s *ResolveService) ResolveAccountChunk(ctx context.Context, run driven.RunContext) (*ResolveChunkResult, error) {
	if s == nil || s.Users == nil || s.Messages == nil || s.Contacts == nil {
		return nil, fmt.Errorf("resolve service not configured")
	}
	if run.AccountID == nil || *run.AccountID == uuid.Nil {
		return nil, fmt.Errorf("account_id is required")
	}
	orgID, err := s.Users.GetHomeOrganisationID(ctx, run.UserID)
	if err != nil {
		return nil, err
	}
	processed, done, err := s.resolveNextBatch(ctx, orgID, run.UserID, *run.AccountID)
	if err != nil {
		return nil, err
	}
	return &ResolveChunkResult{MessagesProcessed: processed, Done: done}, nil
}

// resolveNextBatch resolves the next unresolved messages on an account and
// marks them. The backlog drains instead of being re-read from the start on
// every run, so there is no cursor: the work queue shrinks as it is done.
func (s *ResolveService) resolveNextBatch(ctx context.Context, orgID, userID, accountID uuid.UUID) (int, bool, error) {
	rows, err := s.Messages.ListMessagesNeedingContactResolution(ctx, userID, accountID, resolveChunkSize+1)
	if err != nil {
		return 0, false, err
	}
	done := len(rows) <= resolveChunkSize
	if !done {
		rows = rows[:resolveChunkSize]
	}
	resolved := make([]uuid.UUID, 0, len(rows))
	for i := range rows {
		msg := rows[i]
		if err := s.resolveStoredMessage(ctx, orgID, &msg); err != nil {
			return 0, false, err
		}
		resolved = append(resolved, msg.ID)
	}
	// Mark after resolving, so a batch that fails part-way is retried rather
	// than skipped. Resolution is idempotent, so the retry costs nothing.
	if err := s.Messages.MarkContactsResolved(ctx, userID, resolved, time.Now().UTC()); err != nil {
		return 0, false, err
	}
	return len(rows), done, nil
}

func (s *ResolveService) resolveStoredMessage(ctx context.Context, orgID uuid.UUID, msg *driven.MessageRow) error {
	now := time.Now().UTC()
	type part struct {
		role domaincontacts.ParticipantRole
		name string
		addr string
	}
	var parts []part
	from := parseSingleRecipient(msg.FromJSON)
	if from.Address != "" {
		parts = append(parts, part{role: domaincontacts.RoleFrom, name: from.Name, addr: from.Address})
	}
	for _, r := range parseRecipientList(msg.ToJSON) {
		if r.Address == "" {
			continue
		}
		parts = append(parts, part{role: domaincontacts.RoleTo, name: r.Name, addr: r.Address})
	}
	for _, r := range parseRecipientList(msg.CcJSON) {
		if r.Address == "" {
			continue
		}
		parts = append(parts, part{role: domaincontacts.RoleCc, name: r.Name, addr: r.Address})
	}
	msgID := msg.ID
	for _, p := range parts {
		contactID, err := s.Contacts.ResolveEmailContact(ctx, orgID, p.addr, p.name, now)
		if err != nil {
			continue
		}
		_ = s.Contacts.UpsertParticipant(ctx, driven.CorrespondenceParticipantRow{
			ID:             uuid.New(),
			OrganisationID: orgID,
			ContactID:      contactID,
			Role:           string(p.role),
			MessageID:      &msgID,
		})
	}
	return nil
}

func parseSingleRecipient(raw string) recipientPayload {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return recipientPayload{}
	}
	var p recipientPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return recipientPayload{}
	}
	p.Address = domaincontacts.NormalizeEmail(p.Address)
	p.Name = strings.TrimSpace(p.Name)
	return p
}

func parseRecipientList(raw string) []recipientPayload {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var list []recipientPayload
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil
	}
	out := make([]recipientPayload, 0, len(list))
	for _, p := range list {
		p.Address = domaincontacts.NormalizeEmail(p.Address)
		p.Name = strings.TrimSpace(p.Name)
		if p.Address == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// Service exposes contact CRUD and merge for HTTP.
type Service struct {
	Users    driven.UserRepository
	Contacts driven.ContactRepository
	Messages driven.MessageRepository
}

func (s *Service) homeOrg(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return s.Users.GetHomeOrganisationID(ctx, userID)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, filter driven.ContactListFilter) ([]driven.ContactRow, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.Contacts.ListContacts(ctx, orgID, filter)
}

type ContactDetail struct {
	Contact         driven.ContactRow
	Identities      []driven.ContactIdentityRow
	RecentMessages  []RecentMessageRef
	SuggestedMerges []driven.ContactRow
}

// RecentMessageRef is enough of a message to recognise it in a list.
type RecentMessageRef struct {
	MessageID   uuid.UUID
	AccountID   uuid.UUID
	Subject     string
	FromName    string
	FromAddress string
	ReceivedAt  time.Time
}

func (s *Service) Get(ctx context.Context, userID, contactID uuid.UUID) (*ContactDetail, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	c, err := s.Contacts.GetContact(ctx, orgID, contactID)
	if err != nil {
		return nil, err
	}
	if c == nil || c.MergedIntoContactID != nil {
		return nil, nil
	}
	idents, err := s.Contacts.ListIdentities(ctx, orgID, contactID)
	if err != nil {
		return nil, err
	}
	recentIDs, err := s.Contacts.ListRecentMessageIDs(ctx, orgID, contactID, 20)
	if err != nil {
		return nil, err
	}
	recent := make([]RecentMessageRef, 0, len(recentIDs))
	for _, mid := range recentIDs {
		msg, err := s.Messages.GetMessage(ctx, userID, mid)
		if err != nil || msg == nil {
			continue
		}
		ref := RecentMessageRef{MessageID: mid, AccountID: msg.AccountID, Subject: msg.Subject, ReceivedAt: msg.ReceivedAt}
		var from struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		}
		if json.Unmarshal([]byte(msg.FromJSON), &from) == nil {
			ref.FromName, ref.FromAddress = from.Name, from.Address
		}
		recent = append(recent, ref)
	}
	suggestions, err := s.Contacts.SuggestMerges(ctx, orgID, contactID)
	if err != nil {
		return nil, err
	}
	return &ContactDetail{
		Contact:         *c,
		Identities:      idents,
		RecentMessages:  recent,
		SuggestedMerges: suggestions,
	}, nil
}

type CreateContactInput struct {
	DisplayName string
	Company     *string
	Identities  []struct {
		Kind  string
		Value string
	}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, in CreateContactInput) (*driven.ContactRow, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	id := uuid.New()
	row := driven.ContactRow{
		ID:             id,
		OrganisationID: orgID,
		DisplayName:    strings.TrimSpace(in.DisplayName),
		Company:        in.Company,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.Contacts.CreateContact(ctx, row); err != nil {
		return nil, err
	}
	for _, ident := range in.Identities {
		kind := strings.TrimSpace(ident.Kind)
		raw := strings.TrimSpace(ident.Value)
		if kind == "" || raw == "" {
			continue
		}
		norm := raw
		switch domaincontacts.IdentityKind(kind) {
		case domaincontacts.KindEmail:
			norm = domaincontacts.NormalizeEmail(raw)
		case domaincontacts.KindPhone:
			norm = domaincontacts.NormalizePhone(raw)
		case domaincontacts.KindDisplayNameHint:
			norm = strings.ToLower(strings.TrimSpace(raw))
		default:
			return nil, fmt.Errorf("invalid identity kind")
		}
		if norm == "" {
			continue
		}
		if err := s.Contacts.CreateIdentity(ctx, driven.ContactIdentityRow{
			ID:              uuid.New(),
			OrganisationID:  orgID,
			ContactID:       id,
			Kind:            kind,
			ValueNormalized: norm,
			ValueRaw:        raw,
			CreatedAt:       now,
		}); err != nil {
			return nil, err
		}
	}
	return &row, nil
}

func (s *Service) AddIdentity(ctx context.Context, userID, contactID uuid.UUID, kind, value string) (*driven.ContactIdentityRow, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	c, err := s.Contacts.GetContact(ctx, orgID, contactID)
	if err != nil {
		return nil, err
	}
	if c == nil || c.MergedIntoContactID != nil {
		return nil, fmt.Errorf("contact not found")
	}
	kind = strings.TrimSpace(kind)
	raw := strings.TrimSpace(value)
	if !domaincontacts.IdentityKind(kind).Valid() || raw == "" {
		return nil, fmt.Errorf("invalid identity")
	}
	norm := raw
	switch domaincontacts.IdentityKind(kind) {
	case domaincontacts.KindEmail:
		norm = domaincontacts.NormalizeEmail(raw)
	case domaincontacts.KindPhone:
		norm = domaincontacts.NormalizePhone(raw)
	case domaincontacts.KindDisplayNameHint:
		norm = strings.ToLower(strings.TrimSpace(raw))
	}
	if norm == "" {
		return nil, fmt.Errorf("invalid identity value")
	}
	now := time.Now().UTC()
	row := driven.ContactIdentityRow{
		ID:              uuid.New(),
		OrganisationID:  orgID,
		ContactID:       contactID,
		Kind:            kind,
		ValueNormalized: norm,
		ValueRaw:        raw,
		CreatedAt:       now,
	}
	if err := s.Contacts.CreateIdentity(ctx, row); err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) Merge(ctx context.Context, userID, survivorID, sourceID uuid.UUID) error {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return err
	}
	return s.Contacts.MergeContacts(ctx, orgID, survivorID, sourceID, time.Now().UTC())
}
