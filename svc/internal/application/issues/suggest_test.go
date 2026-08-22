package issues_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	appissues "github.com/Kapital-B/automata/svc/internal/application/issues"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

type fakeLLM struct {
	content string
	err     error
	calls   int
}

func (f *fakeLLM) ChatCompletion(ctx context.Context, messages []driven.LLMMessage) (*driven.LLMResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &driven.LLMResponse{Content: f.content}, nil
}

func TestSuggestReturnsProposalWithoutCreating(t *testing.T) {
	_, repo, issueSvc, projectSvc, userID, projectID, accountID := setupIssues(t, "issuesuggest")
	ctx := context.Background()
	manual, err := projectSvc.CreateManualItem(ctx, userID, appprojects.CreateManualInput{
		Channel: "teams", OccurredAt: time.Now().UTC(), Title: "Teams pump note",
		BodyText: "Need 90 kW for P-03", ProjectID: &projectID,
	})
	if err != nil {
		t.Fatal(err)
	}
	msgID := uuid.New()
	conv := "conv-sug"
	body := "pump sizing P-03"
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: accountID, ProviderMessageID: msgID.String(),
		ReceivedAt: time.Now().UTC(), Subject: "Outlook: P-03", BodyText: &body,
		FromJSON: `{}`, ConversationID: &conv,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.AssignMessage(ctx, userID, msgID, appprojects.AssignInput{
		ProjectID: &projectID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
	}); err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(map[string]any{
		"schema_version":   1,
		"project_id":       projectID.String(),
		"issue_title":      "Pump P-03",
		"message_ids":      []string{msgID.String()},
		"manual_item_ids":  []string{manual.ID.String()},
		"confidence":       0.91,
		"reason":           "both mention P-03",
	})
	llm := &fakeLLM{content: string(payload)}
	issueSvc.LLM = llm
	issueSvc.Timeline = repo

	res, err := issueSvc.Suggest(ctx, userID, projectID, appissues.SuggestInput{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "Pump P-03" || res.Confidence < 0.9 {
		t.Fatalf("got %+v", res)
	}
	if len(res.ItemRefs) != 2 {
		t.Fatalf("refs=%d", len(res.ItemRefs))
	}
	list, err := issueSvc.List(ctx, userID, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatal("suggest must not create issues")
	}
}

func TestSuggestRejectsForeignIDsAndSingleAccount(t *testing.T) {
	_, repo, issueSvc, projectSvc, userID, projectID, accountID := setupIssues(t, "issuesuggest2")
	ctx := context.Background()
	msgID := uuid.New()
	conv := "conv-a"
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: accountID, ProviderMessageID: msgID.String(),
		ReceivedAt: time.Now().UTC(), Subject: "A", FromJSON: `{}`, ConversationID: &conv,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.AssignMessage(ctx, userID, msgID, appprojects.AssignInput{
		ProjectID: &projectID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
	}); err != nil {
		t.Fatal(err)
	}
	foreign := uuid.New()
	payload, _ := json.Marshal(map[string]any{
		"schema_version": 1,
		"issue_title":    "Only A",
		"message_ids":    []string{msgID.String(), foreign.String()},
		"confidence":     0.5,
	})
	issueSvc.LLM = &fakeLLM{content: string(payload)}
	issueSvc.Timeline = repo
	res, err := issueSvc.Suggest(ctx, userID, projectID, appissues.SuggestInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ItemRefs) != 1 || res.ItemRefs[0].MessageID == nil || *res.ItemRefs[0].MessageID != msgID {
		t.Fatalf("expected only allowed message, got %+v", res.ItemRefs)
	}
}

func TestSuggestRequiresLLM(t *testing.T) {
	_, _, issueSvc, _, userID, projectID, _ := setupIssues(t, "issuesuggest3")
	_, err := issueSvc.Suggest(context.Background(), userID, projectID, appissues.SuggestInput{})
	if !errors.Is(err, appissues.ErrLLMUnavailable) {
		t.Fatalf("got %v", err)
	}
}
