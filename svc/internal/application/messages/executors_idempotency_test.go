package messages

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/memoryjobs"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func TestSyncExecutorReplayKeepsSingleMessageUpsert(t *testing.T) {
	graph := &fakeDeltaGraph{
		results: []*driven.GraphDeltaResult{
			{
				Messages:  []driven.GraphMessage{{ID: "provider-1", Subject: "one", ReceivedDateTime: time.Now().UTC().Format(time.RFC3339), FromAddress: "a@example.com"}},
				DeltaLink: "delta-1",
			},
			{
				Messages:  []driven.GraphMessage{{ID: "provider-1", Subject: "one", ReceivedDateTime: time.Now().UTC().Format(time.RFC3339), FromAddress: "a@example.com"}},
				DeltaLink: "delta-1",
			},
		},
	}
	svc, repo, userID, accountID := setupSyncService(t, graph)
	svc.JobRuns = nil
	exec := &SyncExecutor{Service: svc}
	run := driven.RunContext{
		RunID:     uuid.New(),
		AttemptID: uuid.New(),
		UserID:    userID,
		AccountID: &accountID,
		Cursor:    &driven.JobCursor{Kind: "graph_next_link", Value: "delta-replay"},
	}
	if _, err := exec.ExecuteChunk(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.ExecuteChunk(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListMessages(context.Background(), userID, driven.MessageListFilter{AccountID: &accountID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one message after replay, got %d", len(rows))
	}
}

func TestCategorizeExecutorReplayKeepsSingleCategoryState(t *testing.T) {
	repo, userID, accountID := setupCategorizeRepo(t)
	svc := &CategorizeService{
		Messages: repo,
		LLM: &fakeLLM{responses: []string{
			`{"schema_version":1,"category_slug":"finance","confidence":0.88}`,
			`{"schema_version":1,"category_slug":"finance","confidence":0.88}`,
		}},
	}
	exec := &CategorizeExecutor{Service: svc}
	run := driven.RunContext{
		RunID:     uuid.New(),
		AttemptID: uuid.New(),
		UserID:    userID,
		AccountID: &accountID,
		Payload:   driven.JobPayload{Recategorize: true},
	}
	if _, err := exec.ExecuteChunk(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.ExecuteChunk(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListMessages(context.Background(), userID, driven.MessageListFilter{AccountID: &accountID, Category: "finance"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one categorized message after replay, got %d", len(rows))
	}
}

func TestDraftSuggestExecutorReplayKeepsSingleDraft(t *testing.T) {
	db, repo, userID, accountID, msgID := setupSummarizeRepo(t)
	now := time.Now().UTC()
	if err := repo.InsertActionItems(context.Background(), []driven.ActionItemRow{{
		ID:        uuid.New(),
		UserID:    userID,
		AccountID: accountID,
		MessageID: msgID,
		RunID:     uuid.New(),
		Text:      "Reply to this message",
		Status:    "open",
		CreatedAt: now,
		UpdatedAt: now,
	}}); err != nil {
		t.Fatal(err)
	}
	svc := &AutoDraftService{
		Messages:   repo,
		Summaries:  repo,
		LLM:        &fakeLLM{responses: []string{`{"subject":"Re: Friday meeting slot","body":"Friday works for me."}`, `{"subject":"Re: Friday meeting slot","body":"Friday works for me."}`}},
		ModelLabel: "test-model",
	}
	exec := &DraftSuggestExecutor{Service: svc}
	run := driven.RunContext{
		RunID:     uuid.New(),
		AttemptID: uuid.New(),
		UserID:    userID,
		AccountID: &accountID,
		Payload:   driven.JobPayload{MessageID: &msgID, Force: true},
	}
	if _, err := exec.ExecuteChunk(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	// Re-open the same action item to prove the draft idempotency is not just the seen marker.
	if _, err := db.Exec(`UPDATE action_items SET auto_draft_seen_at = NULL WHERE message_id = ?`, msgID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.ExecuteChunk(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	drafts, err := repo.ListDraftSuggestions(context.Background(), userID, &accountID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 1 {
		t.Fatalf("expected one draft suggestion after replay, got %d", len(drafts))
	}
}

func TestForwardRulesExecutorEffectClaimPreventsDuplicateSend(t *testing.T) {
	db, svc, repo, userID, accountID, messageID := setupForwardRulesService(t, &fakeForwardGraph{})
	graph := &fakeForwardGraph{}
	svc.Graph = graph
	store := memoryjobs.NewStore()
	exec := &ForwardRulesExecutor{Service: svc, Store: store}
	run := driven.RunContext{
		RunID:     uuid.New(),
		AttemptID: uuid.New(),
		UserID:    userID,
		AccountID: &accountID,
	}
	if _, err := exec.ExecuteChunk(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE messages SET forward_seen_at = NULL WHERE id = ?`, messageID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.ExecuteChunk(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if graph.forwardCalls != 1 {
		t.Fatalf("expected effect claim to suppress duplicate send, got %d calls", graph.forwardCalls)
	}
	rules, err := repo.ListForwardRules(context.Background(), userID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) == 0 {
		t.Fatal("expected forward rule")
	}
	effectKey := fmt.Sprintf("forward:%s:%s:%s", messageID.String(), rules[0].ID.String(), rules[0].ForwardTo)
	effect, err := store.GetEffect(context.Background(), accountID, effectKey)
	if err != nil {
		t.Fatal(err)
	}
	if effect == nil {
		t.Fatal("expected stored effect claim")
	}
}
