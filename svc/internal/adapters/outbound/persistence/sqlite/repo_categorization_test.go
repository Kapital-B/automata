package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestCategorizationTablesAndMessageFilters(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db, 15*time.Minute)
	ctx := context.Background()

	userID := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	accountID := uuid.New()
	err = repo.InsertAccount(ctx, driven.AccountRow{
		UserID:           userID,
		ID:               accountID,
		Label:            "Work",
		Provider:         "m365",
		MsAccountKind:    domainacc.KindWork,
		PrimaryEmail:     "work@example.com",
		ConnectionStatus: "connected",
	}, []byte("cipher"))
	if err != nil {
		t.Fatal(err)
	}

	cats, err := repo.ListCategoryDefinitions(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) < 6 {
		t.Fatalf("expected seeded categories, got %d", len(cats))
	}
	important, err := repo.GetCategoryDefinitionBySlug(ctx, userID, "important")
	if err != nil || important == nil {
		t.Fatalf("important category missing: %v", err)
	}

	msg := driven.MessageRow{
		ID:                uuid.New(),
		AccountID:         accountID,
		ProviderMessageID: "provider-1",
		ReceivedAt:        time.Now().UTC(),
		Subject:           "Invoice for April",
		FromJSON:          `{"name":"Stripe","address":"billing@stripe.com"}`,
		BodyText:          strPtr("Please settle your invoice."),
		HasAttachments:    false,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	if err := repo.UpsertMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}
	runID := uuid.New()
	if err := repo.InsertJobRun(ctx, runID, accountID, "categorize", "api", "success", time.Now().UTC(), time.Now().UTC(), nil, `{}`); err != nil {
		t.Fatal(err)
	}
	conf := 0.93
	if err := repo.UpsertMessageCategory(ctx, driven.MessageCategoryRow{
		ID:         uuid.New(),
		MessageID:  msg.ID,
		AccountID:  accountID,
		CategoryID: important.ID,
		Source:     "llm",
		Confidence: &conf,
		RunID:      runID,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := repo.ListMessages(ctx, userID, driven.MessageListFilter{AccountID: &accountID, Category: "important"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 categorized message, got %d", len(rows))
	}
	if rows[0].CategorySlug == nil || *rows[0].CategorySlug != "important" {
		t.Fatalf("expected category important, got %v", rows[0].CategorySlug)
	}
}

func TestMigrateCanRunRepeatedlyAfterUserScopedCategories(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db, 15*time.Minute)
	userID := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	cats, err := repo.ListCategoryDefinitions(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) < 6 {
		t.Fatalf("expected seeded categories after repeated migration, got %d", len(cats))
	}
}

func TestMigrateSeedsDefaultsWhenUserScopedCategoriesAreEmpty(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM category_definitions`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db, 15*time.Minute)
	userID := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	other, err := repo.GetCategoryDefinitionBySlug(context.Background(), userID, "other")
	if err != nil {
		t.Fatal(err)
	}
	if other == nil {
		t.Fatal("expected default other category to be reseeded")
	}
	if other.Definition == "" {
		t.Fatal("expected reseeded category to include a definition")
	}
}

func TestMigrateDeletesOrphanedMessageCategories(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db, 15*time.Minute)
	ctx := context.Background()
	userID := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID:           userID,
		ID:               accountID,
		Label:            "Work",
		Provider:         "m365",
		MsAccountKind:    domainacc.KindWork,
		PrimaryEmail:     "work@example.com",
		ConnectionStatus: "connected",
	}, []byte("cipher")); err != nil {
		t.Fatal(err)
	}
	msg := driven.MessageRow{
		ID:                uuid.New(),
		AccountID:         accountID,
		ProviderMessageID: "provider-orphan",
		ReceivedAt:        time.Now().UTC(),
		Subject:           "Receipt",
		FromJSON:          `{"name":"Store","address":"store@example.com"}`,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	if err := repo.UpsertMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}
	runID := uuid.New()
	if err := repo.InsertJobRun(ctx, runID, accountID, "categorize", "api", "success", time.Now().UTC(), time.Now().UTC(), nil, `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO message_categories (id, message_id, account_id, category_id, source, run_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'llm', ?, ?, ?)
	`, uuid.New().String(), msg.ID.String(), accountID.String(), uuid.New().String(), runID.String(), formatRFC3339(time.Now().UTC()), formatRFC3339(time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM message_categories`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected orphaned message category to be deleted, got %d", n)
	}
}

func strPtr(s string) *string { return &s }
