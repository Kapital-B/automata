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

func setupSummarizeRepo(t *testing.T) (*sql.DB, *sqlite.Repository, uuid.UUID, uuid.UUID, uuid.UUID) {
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
	msgID := uuid.New()
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
	body := "Hi David,\n\nLet me know when you are available to meet on Friday?"
	if err := repo.UpsertMessage(context.Background(), driven.MessageRow{
		ID:                msgID,
		AccountID:         accountID,
		ProviderMessageID: "provider-friday-1",
		ReceivedAt:        time.Date(2026, 4, 28, 11, 0, 0, 0, time.UTC),
		Subject:           "Friday meeting slot",
		FromJSON:          `{"name":"David","address":"other@example.com"}`,
		BodyText:          &body,
		CreatedAt:         time.Date(2026, 4, 28, 13, 24, 13, 0, time.UTC),
		UpdatedAt:         time.Date(2026, 4, 28, 13, 24, 13, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	return db, repo, userID, accountID, msgID
}

func TestSummarizeConsidersUnseenBySummarySeenNotReceivedAt(t *testing.T) {
	db, repo, userID, accountID, msgID := setupSummarizeRepo(t)
	llmJSON := `{"general_summary":"ok","action_items":[{"message_id":"` + msgID.String() + `","text":"Reply with your Friday availability"}],"fyi":[]}`
	svc := &SummarizeService{
		Messages:  repo,
		Summaries: repo,
		LLM:       &fakeLLM{responses: []string{llmJSON}},
		JobRuns:   repo,
	}
	_, err := svc.SummarizeAccount(context.Background(), userID, accountID, SummarizeOptions{
		Trigger: "schedule",
		Since:   ptrTime(time.Date(2026, 4, 28, 13, 24, 11, 0, time.UTC)),
	})
	if err != nil {
		t.Fatal(err)
	}
	m, err := repo.GetMessage(context.Background(), userID, msgID)
	if err != nil {
		t.Fatal(err)
	}
	if m == nil || m.SummarySeenAt == nil {
		t.Fatalf("expected message marked summary seen, got %+v", m)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(1) FROM action_items WHERE message_id = ?`, msgID.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected one action item, got %d", n)
	}
}

func TestSummarizeMarksExcludedBySettingsAsSeenWithoutLLM(t *testing.T) {
	_, repo, userID, accountID, msgID := setupSummarizeRepo(t)
	def, err := repo.GetCategoryDefinitionBySlug(context.Background(), userID, "important")
	if err != nil || def == nil {
		t.Fatalf("important category: %v", err)
	}
	runID := uuid.New()
	if err := repo.UpsertMessageCategory(context.Background(), driven.MessageCategoryRow{
		ID:         uuid.New(),
		MessageID:  msgID,
		AccountID:  accountID,
		CategoryID: def.ID,
		Source:     "llm",
		Confidence: ptrFloat64(0.9),
		RunID:      runID,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSummarySettings(context.Background(), driven.SummarySettingsRow{
		UserID:               userID,
		IncludeCategorySlugs: nil,
		ExcludeCategorySlugs: []string{"important"},
		ChunkSize:            12,
		UpdatedAt:            time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	svc := &SummarizeService{
		Messages:  repo,
		Summaries: repo,
		LLM: &fakeLLM{
			responses: []string{`{"general_summary":"should not run","action_items":[],"fyi":[]}`},
			onCall:    func() { calls++ },
		},
		JobRuns: repo,
	}
	if _, err := svc.SummarizeAccount(context.Background(), userID, accountID, SummarizeOptions{Trigger: "schedule"}); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("expected LLM not invoked when all messages filtered out, got %d calls", calls)
	}
	m, err := repo.GetMessage(context.Background(), userID, msgID)
	if err != nil {
		t.Fatal(err)
	}
	if m == nil || m.SummarySeenAt == nil {
		t.Fatalf("expected excluded message still marked summary seen, got %+v", m)
	}
}

func TestSummarizeSecondRunFindsNoUnseenMessages(t *testing.T) {
	_, repo, userID, accountID, msgID := setupSummarizeRepo(t)
	llmJSON := `{"general_summary":"ok","action_items":[{"message_id":"` + msgID.String() + `","text":"Reply"}],"fyi":[]}`
	svc := &SummarizeService{
		Messages:  repo,
		Summaries: repo,
		LLM:       &fakeLLM{responses: []string{llmJSON, llmJSON}},
		JobRuns:   repo,
	}
	if _, err := svc.SummarizeAccount(context.Background(), userID, accountID, SummarizeOptions{Trigger: "schedule"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SummarizeAccount(context.Background(), userID, accountID, SummarizeOptions{Trigger: "schedule"}); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListMessages(context.Background(), userID, driven.MessageListFilter{
		AccountID:         &accountID,
		OnlySummaryUnseen: true,
		Limit:             50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no unseen messages after two summarize runs, got %d", len(rows))
	}
}

func TestSummarizeDoesNotDuplicateOpenActionItem(t *testing.T) {
	db, repo, userID, accountID, msgID := setupSummarizeRepo(t)
	now := time.Now().UTC()
	if err := repo.InsertActionItems(context.Background(), []driven.ActionItemRow{{
		ID:        uuid.New(),
		UserID:    userID,
		AccountID: accountID,
		MessageID: msgID,
		RunID:     uuid.New(),
		Text:      "Existing open item",
		Status:    "open",
		CreatedAt: now,
		UpdatedAt: now,
	}}); err != nil {
		t.Fatal(err)
	}
	llmJSON := `{"general_summary":"ok","action_items":[{"message_id":"` + msgID.String() + `","text":"Another ask"}],"fyi":[]}`
	svc := &SummarizeService{
		Messages:  repo,
		Summaries: repo,
		LLM:       &fakeLLM{responses: []string{llmJSON}},
		JobRuns:   repo,
	}
	if _, err := svc.SummarizeAccount(context.Background(), userID, accountID, SummarizeOptions{Trigger: "schedule"}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(1) FROM action_items WHERE message_id = ? AND status = 'open'`, msgID.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected still one open action item for message, got %d", n)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func ptrFloat64(f float64) *float64 { return &f }
