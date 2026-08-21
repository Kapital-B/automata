package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func openMigrated(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+uuid.New().String()+"?mode=memory&cache=shared")
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
	return db
}

func TestMigrateBackfillsSeedUserHomeOrg(t *testing.T) {
	db := openMigrated(t)
	var orgID sql.NullString
	err := db.QueryRow(`SELECT home_organisation_id FROM users WHERE id = ?`, "a0000001-0000-4000-8000-000000000001").Scan(&orgID)
	if err != nil {
		t.Fatal(err)
	}
	if !orgID.Valid || orgID.String == "" {
		t.Fatal("expected seed user home_organisation_id")
	}
	var role string
	err = db.QueryRow(`
		SELECT org_role FROM organisation_members WHERE organisation_id = ? AND user_id = ?
	`, orgID.String, "a0000001-0000-4000-8000-000000000001").Scan(&role)
	if err != nil {
		t.Fatal(err)
	}
	if role != "owner" {
		t.Fatalf("role=%s", role)
	}
}

func TestCreateUserWithHomeOrgAtomic(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	id := uuid.New()
	now := time.Now().UTC()
	hash := "hash"
	orgID, err := repo.CreateUserWithHomeOrg(context.Background(), id, "alice@example.com", &hash, now, "password", id.String(), "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetHomeOrganisationID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got != orgID {
		t.Fatalf("home org mismatch")
	}
}

func TestContactEmailUniquenessPerOrg(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	ctx := context.Background()
	now := time.Now().UTC()

	u1 := uuid.New()
	org1, err := repo.CreateUserWithHomeOrg(ctx, u1, "u1@example.com", nil, now, "password", u1.String(), "u1@example.com")
	if err != nil {
		t.Fatal(err)
	}
	u2 := uuid.New()
	org2, err := repo.CreateUserWithHomeOrg(ctx, u2, "u2@example.com", nil, now, "password", u2.String(), "u2@example.com")
	if err != nil {
		t.Fatal(err)
	}

	c1, err := repo.ResolveEmailContact(ctx, org1, "sarah@acme.com", "Sarah", now)
	if err != nil {
		t.Fatal(err)
	}
	c1b, err := repo.ResolveEmailContact(ctx, org1, "sarah@acme.com", "Sarah Other", now)
	if err != nil {
		t.Fatal(err)
	}
	if c1 != c1b {
		t.Fatal("same org email should reuse contact")
	}
	c2, err := repo.ResolveEmailContact(ctx, org2, "sarah@acme.com", "Sarah", now)
	if err != nil {
		t.Fatal(err)
	}
	if c1 == c2 {
		t.Fatal("different orgs may both have sarah@")
	}
}

func TestMessageDeleteDropsParticipantsKeepsContact(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	ctx := context.Background()
	now := time.Now().UTC()
	userID := uuid.New()
	orgID, err := repo.CreateUserWithHomeOrg(ctx, userID, "owner@example.com", nil, now, "password", userID.String(), "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365",
		MsAccountKind: "work", PrimaryEmail: "owner@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	msgID := uuid.New()
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: accountID, ProviderMessageID: "p1",
		ReceivedAt: now, Subject: "hi", FromJSON: `{"name":"S","address":"sarah@acme.com"}`,
		ToJSON: "[]", CcJSON: "[]", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	contactID, err := repo.ResolveEmailContact(ctx, orgID, "sarah@acme.com", "Sarah", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertParticipant(ctx, driven.CorrespondenceParticipantRow{
		ID: uuid.New(), OrganisationID: orgID, ContactID: contactID, Role: "from", MessageID: &msgID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM messages WHERE id = ?`, msgID.String()); err != nil {
		t.Fatal(err)
	}
	var partCount int
	if err := db.QueryRow(`SELECT COUNT(1) FROM correspondence_participants WHERE contact_id = ?`, contactID.String()).Scan(&partCount); err != nil {
		t.Fatal(err)
	}
	if partCount != 0 {
		t.Fatalf("expected participants cascaded, got %d", partCount)
	}
	c, err := repo.GetContact(ctx, orgID, contactID)
	if err != nil || c == nil {
		t.Fatal("contact should remain")
	}
}

func TestMergeContactsHidesLoser(t *testing.T) {
	db := openMigrated(t)
	repo := sqlite.NewRepository(db, time.Minute)
	ctx := context.Background()
	now := time.Now().UTC()
	userID := uuid.New()
	orgID, err := repo.CreateUserWithHomeOrg(ctx, userID, "owner@example.com", nil, now, "password", userID.String(), "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	a, err := repo.ResolveEmailContact(ctx, orgID, "a@example.com", "Alex", now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := repo.ResolveEmailContact(ctx, orgID, "b@example.com", "Alex", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MergeContacts(ctx, orgID, a, b, now); err != nil {
		t.Fatal(err)
	}
	list, err := repo.ListContacts(ctx, orgID, driven.ContactListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != a {
		t.Fatalf("expected only survivor listed, got %+v", list)
	}
	loser, err := repo.GetContact(ctx, orgID, b)
	if err != nil || loser == nil || loser.MergedIntoContactID == nil || *loser.MergedIntoContactID != a {
		t.Fatalf("loser not marked merged: %+v", loser)
	}
	idents, err := repo.ListIdentities(ctx, orgID, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(idents) != 2 {
		t.Fatalf("expected 2 identities on survivor, got %d", len(idents))
	}
}

func TestNoContactProfileLinkWrites(t *testing.T) {
	db := openMigrated(t)
	var n int
	if err := db.QueryRow(`SELECT COUNT(1) FROM contact_profile_links`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected empty contact_profile_links, got %d", n)
	}
}
