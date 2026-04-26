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

	cats, err := repo.ListCategoryDefinitions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) < 6 {
		t.Fatalf("expected seeded categories, got %d", len(cats))
	}
	important, err := repo.GetCategoryDefinitionBySlug(ctx, "important")
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

func strPtr(s string) *string { return &s }
