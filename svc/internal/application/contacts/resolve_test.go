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
