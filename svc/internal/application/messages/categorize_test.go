package messages

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type fakeLLM struct {
	responses []string
	idx       int
	onCall    func()
}

func (f *fakeLLM) ChatCompletion(ctx context.Context, messages []driven.LLMMessage) (*driven.LLMResponse, error) {
	if f.onCall != nil {
		f.onCall()
	}
	if f.idx >= len(f.responses) {
		return &driven.LLMResponse{Content: `{"schema_version":1,"category_slug":"other"}`}, nil
	}
	out := f.responses[f.idx]
	f.idx++
	return &driven.LLMResponse{Content: out}, nil
}

func setupCategorizeRepo(t *testing.T) (*sqlite.Repository, uuid.UUID, uuid.UUID) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	userID := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	accountID := uuid.New()
	if err := repo.InsertAccount(context.Background(), driven.AccountRow{
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
	msgBody := "please pay this invoice"
	if err := repo.UpsertMessage(context.Background(), driven.MessageRow{
		ID:                uuid.New(),
		AccountID:         accountID,
		ProviderMessageID: "provider-1",
		ReceivedAt:        time.Now().UTC(),
		Subject:           "Invoice due",
		FromJSON:          `{"name":"Stripe","address":"billing@stripe.com"}`,
		BodyText:          &msgBody,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	return repo, userID, accountID
}

func TestCategorizeAccountValidJSON(t *testing.T) {
	repo, userID, accountID := setupCategorizeRepo(t)
	svc := &CategorizeService{
		Messages: repo,
		LLM: &fakeLLM{responses: []string{
			`{"schema_version":1,"category_slug":"finance","confidence":0.88}`,
		}},
		JobRuns: repo,
	}
	res, err := svc.CategorizeAccount(context.Background(), userID, accountID, CategorizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.MessagesCategorized != 1 {
		t.Fatalf("expected one categorized message, got %d", res.MessagesCategorized)
	}
	rows, err := repo.ListMessages(context.Background(), userID, driven.MessageListFilter{AccountID: &accountID, Category: "finance"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected finance category persisted, got %d rows", len(rows))
	}
}

func TestCategorizeAccountRetriesInvalidJSON(t *testing.T) {
	repo, userID, accountID := setupCategorizeRepo(t)
	svc := &CategorizeService{
		Messages: repo,
		LLM: &fakeLLM{responses: []string{
			`not-json`,
			`{"schema_version":1,"category_slug":"important"}`,
		}},
		JobRuns: repo,
	}
	if _, err := svc.CategorizeAccount(context.Background(), userID, accountID, CategorizeOptions{}); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListMessages(context.Background(), userID, driven.MessageListFilter{AccountID: &accountID, Category: "important"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected important category persisted, got %d rows", len(rows))
	}
}

func TestCategorizeAccountFailsWhenInvalidTwice(t *testing.T) {
	repo, userID, accountID := setupCategorizeRepo(t)
	svc := &CategorizeService{
		Messages: repo,
		LLM: &fakeLLM{responses: []string{
			`nope`,
			`still-not-json`,
		}},
		JobRuns: repo,
	}
	if _, err := svc.CategorizeAccount(context.Background(), userID, accountID, CategorizeOptions{}); err == nil {
		t.Fatal("expected invalid JSON error")
	}
	rows, err := repo.ListMessages(context.Background(), userID, driven.MessageListFilter{AccountID: &accountID, Category: "important"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no category write on failure, got %d", len(rows))
	}
}

func TestCategorizeAccountSkipsAlreadyCategorizedUnlessRecategorize(t *testing.T) {
	repo, userID, accountID := setupCategorizeRepo(t)
	llm := &fakeLLM{responses: []string{
		`{"schema_version":1,"category_slug":"important"}`,
		`{"schema_version":1,"category_slug":"finance"}`,
	}}
	svc := &CategorizeService{
		Messages: repo,
		LLM:      llm,
		JobRuns:  repo,
	}

	first, err := svc.CategorizeAccount(context.Background(), userID, accountID, CategorizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if first.MessagesCategorized != 1 {
		t.Fatalf("expected first categorize to process one message, got %d", first.MessagesCategorized)
	}

	second, err := svc.CategorizeAccount(context.Background(), userID, accountID, CategorizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if second.MessagesCategorized != 0 {
		t.Fatalf("expected second categorize without recategorize to process 0, got %d", second.MessagesCategorized)
	}

	third, err := svc.CategorizeAccount(context.Background(), userID, accountID, CategorizeOptions{Recategorize: true})
	if err != nil {
		t.Fatal(err)
	}
	if third.MessagesCategorized != 1 {
		t.Fatalf("expected recategorize to process one message, got %d", third.MessagesCategorized)
	}
}

func TestClampText(t *testing.T) {
	got := clampText("  abcdef  ", 3)
	if got != "abc...[truncated]" {
		t.Fatalf("unexpected clamp result: %q", got)
	}
	got2 := clampText(" short ", 20)
	if got2 != "short" {
		t.Fatalf("unexpected short clamp result: %q", got2)
	}
}

func TestNormalizeJSONContent(t *testing.T) {
	in := "```json\n{\"schema_version\":1,\"category_slug\":\"important\"}\n```"
	got := normalizeJSONContent(in)
	if got != "{\"schema_version\":1,\"category_slug\":\"important\"}" {
		t.Fatalf("unexpected normalized result: %q", got)
	}
	in2 := "Here is JSON:\n{\"schema_version\":1,\"category_slug\":\"other\"}\nThanks"
	got2 := normalizeJSONContent(in2)
	if got2 != "{\"schema_version\":1,\"category_slug\":\"other\"}" {
		t.Fatalf("unexpected extracted result: %q", got2)
	}
}
