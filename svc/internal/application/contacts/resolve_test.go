package contacts_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	appcontacts "github.com/Kapital-B/automata/svc/internal/application/contacts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestResolveMessageCreatesParticipants(t *testing.T) {
	db, err := sql.Open("sqlite", "file:resolve?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, time.Minute)
	ctx := context.Background()
	now := time.Now().UTC()
	userID := uuid.New()
	if _, err := repo.CreateUserWithHomeOrg(ctx, userID, "u@example.com", nil, now, "password", userID.String(), "u@example.com"); err != nil {
		t.Fatal(err)
	}
	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365",
		MsAccountKind: "work", PrimaryEmail: "u@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	msgID := uuid.New()
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: accountID, ProviderMessageID: "g1",
		ReceivedAt: now, Subject: "Hello",
		FromJSON:  `{"name":"From","address":"from@acme.com"}`,
		ToJSON:    `[{"name":"To","address":"to@acme.com"}]`,
		CcJSON:    `[{"name":"Cc","address":"cc@acme.com"}]`,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	svc := &appcontacts.ResolveService{Users: repo, Messages: repo, Contacts: repo}
	if err := svc.ResolveMessage(ctx, userID, msgID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(1) FROM correspondence_participants WHERE message_id = ?`, msgID.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("expected 3 participants, got %d", n)
	}
	// Second resolve must not duplicate.
	if err := svc.ResolveMessage(ctx, userID, msgID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(1) FROM correspondence_participants WHERE message_id = ?`, msgID.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("expected still 3 participants, got %d", n)
	}
	var contacts int
	if err := db.QueryRow(`SELECT COUNT(1) FROM contacts`).Scan(&contacts); err != nil {
		t.Fatal(err)
	}
	if contacts != 3 {
		t.Fatalf("expected 3 contacts, got %d", contacts)
	}
	var links int
	if err := db.QueryRow(`SELECT COUNT(1) FROM contact_profile_links`).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 0 {
		t.Fatalf("must not write contact_profile_links, got %d", links)
	}
}

func setupResolve(t *testing.T, name string) (*sqlite.Repository, *sql.DB, uuid.UUID, uuid.UUID) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, time.Minute)
	ctx := context.Background()
	now := time.Now().UTC()
	userID := uuid.New()
	if _, err := repo.CreateUserWithHomeOrg(ctx, userID, name+"@example.com", nil, now, "password", userID.String(), name+"@example.com"); err != nil {
		t.Fatal(err)
	}
	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365",
		MsAccountKind: "work", PrimaryEmail: name + "@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	return repo, db, userID, accountID
}

// The chunk drains a backlog instead of re-reading the mailbox from the start,
// so a second run over an already-resolved account does no work.
func TestResolveAccountChunkDrainsAndConverges(t *testing.T) {
	repo, db, userID, accountID := setupResolve(t, "resolvedrain")
	ctx := context.Background()
	now := time.Now().UTC()

	for i := 0; i < 3; i++ {
		id := uuid.New()
		if err := repo.UpsertMessage(ctx, driven.MessageRow{
			ID: id, AccountID: accountID, ProviderMessageID: id.String(),
			ReceivedAt: now.Add(-time.Duration(i) * time.Hour), Subject: "Hello",
			FromJSON:  `{"name":"Sender","address":"sender@acme.com"}`,
			ToJSON:    `[{"name":"Dee","address":"dee@acme.com"}]`,
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	svc := &appcontacts.ResolveService{Users: repo, Messages: repo, Contacts: repo}
	res, err := svc.ResolveAccountChunk(ctx, driven.RunContext{UserID: userID, AccountID: &accountID})
	if err != nil {
		t.Fatal(err)
	}
	if res.MessagesProcessed != 3 || !res.Done {
		t.Fatalf("processed=%d done=%v, want 3/true", res.MessagesProcessed, res.Done)
	}
	var contacts int
	if err := db.QueryRow(`SELECT COUNT(1) FROM contacts`).Scan(&contacts); err != nil {
		t.Fatal(err)
	}
	if contacts != 2 {
		t.Fatalf("contacts = %d, want 2 (one sender, one recipient)", contacts)
	}

	// Nothing left to do: the watermark is what stops the rescan.
	again, err := svc.ResolveAccountChunk(ctx, driven.RunContext{UserID: userID, AccountID: &accountID})
	if err != nil {
		t.Fatal(err)
	}
	if again.MessagesProcessed != 0 {
		t.Errorf("second run processed %d messages, want 0", again.MessagesProcessed)
	}
}

// A message with nothing usable in it still has to be marked, or it is picked
// up again on every run and blocks the rest of the backlog behind it.
func TestResolveAccountChunkMarksAMessageWithNoAddresses(t *testing.T) {
	repo, _, userID, accountID := setupResolve(t, "resolveempty")
	ctx := context.Background()
	now := time.Now().UTC()

	id := uuid.New()
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: id, AccountID: accountID, ProviderMessageID: id.String(),
		ReceivedAt: now, Subject: "", FromJSON: `{}`, ToJSON: `[]`, CcJSON: `[]`,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	svc := &appcontacts.ResolveService{Users: repo, Messages: repo, Contacts: repo}
	if _, err := svc.ResolveAccountChunk(ctx, driven.RunContext{UserID: userID, AccountID: &accountID}); err != nil {
		t.Fatal(err)
	}
	again, err := svc.ResolveAccountChunk(ctx, driven.RunContext{UserID: userID, AccountID: &accountID})
	if err != nil {
		t.Fatal(err)
	}
	if again.MessagesProcessed != 0 {
		t.Errorf("a message with no addresses was retried: processed %d", again.MessagesProcessed)
	}
}
