package contacts_test

import (
	"context"
	"testing"
	"time"

	appcontacts "github.com/Kapital-B/automata/svc/internal/application/contacts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

// A person's page lists their mail. Ids alone gave a column of identical
// "Open in Inbox" links; each entry needs enough to recognise the message.
func TestContactDetailDescribesRecentMessages(t *testing.T) {
	repo, _, userID, accountID := setupResolve(t, "detail")
	ctx := context.Background()
	received := time.Date(2026, 9, 20, 9, 30, 0, 0, time.UTC)
	msgID := uuid.New()
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: accountID, ProviderMessageID: "d1",
		ReceivedAt: received, Subject: "Pump sizing",
		FromJSON:  `{"name":"Sarah Ng","address":"sarah@acme.com"}`,
		ToJSON:    `[]`,
		CcJSON:    `[]`,
		CreatedAt: received, UpdatedAt: received,
	}); err != nil {
		t.Fatal(err)
	}
	if err := (&appcontacts.ResolveService{Users: repo, Messages: repo, Contacts: repo}).ResolveMessage(ctx, userID, msgID); err != nil {
		t.Fatal(err)
	}
	svc := &appcontacts.Service{Users: repo, Contacts: repo, Messages: repo}
	list, err := svc.List(ctx, userID, driven.ContactListFilter{Query: "sarah"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].PrimaryEmail != "sarah@acme.com" {
		t.Fatalf("list = %+v, want Sarah with her address", list)
	}
	detail, err := svc.Get(ctx, userID, list[0].ID)
	if err != nil || detail == nil {
		t.Fatalf("detail = %+v, err = %v", detail, err)
	}
	if len(detail.RecentMessages) != 1 {
		t.Fatalf("recent = %+v", detail.RecentMessages)
	}
	got := detail.RecentMessages[0]
	if got.Subject != "Pump sizing" || got.FromName != "Sarah Ng" || got.FromAddress != "sarah@acme.com" || !got.ReceivedAt.Equal(received) {
		t.Errorf("recent message = %+v", got)
	}
}
