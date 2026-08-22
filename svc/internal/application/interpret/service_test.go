package interpret_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appfacts "github.com/Kapital-B/automata/svc/internal/application/facts"
	appinterpret "github.com/Kapital-B/automata/svc/internal/application/interpret"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type fakeLLM struct {
	content string
	err     error
}

func (f *fakeLLM) ChatCompletion(ctx context.Context, messages []driven.LLMMessage) (*driven.LLMResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &driven.LLMResponse{Content: f.content}, nil
}

func setupInterpret(t *testing.T, name string) (*sqlite.Repository, *appinterpret.Service, *appprojects.Service, *appfacts.Service, uuid.UUID, uuid.UUID, uuid.UUID) {
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
	repo := sqlite.NewRepository(db, 15*time.Minute)
	authSvc := auth.NewService(repo, repo, repo, nil, nil, []byte("abcdefghijklmnopqrstuvwxyz123456"), time.Hour, 30*24*time.Hour)
	projectSvc := &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Manuals: repo, Timeline: repo, Contacts: repo, Messages: repo,
	}
	factSvc := &appfacts.Service{
		Users: repo, Projects: repo, Facts: repo, Issues: repo, Assignments: repo, Manuals: repo, Messages: repo,
	}
	interpSvc := &appinterpret.Service{
		Users: repo, Projects: repo, Interpretations: repo, Facts: repo, Timeline: repo,
		Assignments: repo, Manuals: repo, Messages: repo, JobRuns: repo,
	}
	userID, err := authSvc.Register(context.Background(), name+"@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	proj, err := projectSvc.Create(context.Background(), userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	accountID := uuid.New()
	if err := repo.InsertAccount(context.Background(), driven.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365",
		MsAccountKind: "work", PrimaryEmail: name + "@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	return repo, interpSvc, projectSvc, factSvc, userID, proj.ID, accountID
}

func TestRunPersistsPendingWithoutCreatingFacts(t *testing.T) {
	repo, interpSvc, projectSvc, factSvc, userID, projectID, accountID := setupInterpret(t, "interprun")
	ctx := context.Background()
	manual, err := projectSvc.CreateManualItem(ctx, userID, appprojects.CreateManualInput{
		Channel: "teams", OccurredAt: time.Now().UTC(), Title: "Teams note",
		BodyText: "Pump P-03 duty is 90 kW", ProjectID: &projectID,
	})
	if err != nil {
		t.Fatal(err)
	}
	msgID := uuid.New()
	conv := "conv-i"
	body := "approve 90 kW"
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: accountID, ProviderMessageID: msgID.String(),
		ReceivedAt: time.Now().UTC(), Subject: "Duty", BodyText: &body,
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
		"schema_version": 1,
		"project_id":     projectID.String(),
		"candidates": []map[string]any{
			{
				"kind": "fact", "subject_key": "pump.p03.duty_kw", "label": "Pump P-03 duty",
				"value": 90, "unit": "kW",
				"message_ids": []string{msgID.String()}, "manual_item_ids": []string{manual.ID.String()},
				"confidence": 0.88, "reason": "both mention 90 kW",
			},
		},
	})
	interpSvc.LLM = &fakeLLM{content: string(payload)}

	view, err := interpSvc.Run(ctx, userID, projectID, appinterpret.RunInput{
		MessageIDs: []uuid.UUID{msgID}, ManualItemIDs: []uuid.UUID{manual.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Interpretation.Status != "pending" {
		t.Fatalf("status %s", view.Interpretation.Status)
	}
	if len(view.Candidates) != 1 || view.Candidates[0].SubjectKey != "pump.p03.duty_kw" {
		t.Fatalf("candidates %+v", view.Candidates)
	}
	if len(view.Sources) != 2 {
		t.Fatalf("sources %d", len(view.Sources))
	}

	facts, err := factSvc.List(ctx, userID, projectID, appfacts.ListInclude{Proposed: true, History: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 0 {
		t.Fatalf("interpret must not create facts, got %d", len(facts))
	}

	pending, err := interpSvc.ListPending(ctx, userID, projectID)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%v err=%v", pending, err)
	}
}

func TestDismissPending(t *testing.T) {
	repo, interpSvc, projectSvc, _, userID, projectID, _ := setupInterpret(t, "interpdismiss")
	ctx := context.Background()
	manual, err := projectSvc.CreateManualItem(ctx, userID, appprojects.CreateManualInput{
		Channel: "teams", OccurredAt: time.Now().UTC(), Title: "Note",
		BodyText: "75 kW", ProjectID: &projectID,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{
		"schema_version": 1, "project_id": projectID.String(),
		"candidates": []map[string]any{
			{"kind": "fact", "subject_key": "pump.p03.duty_kw", "label": "Duty", "value": 75, "confidence": 0.5},
		},
	})
	interpSvc.LLM = &fakeLLM{content: string(payload)}
	view, err := interpSvc.Run(ctx, userID, projectID, appinterpret.RunInput{
		ManualItemIDs: []uuid.UUID{manual.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	dismissed, err := interpSvc.Dismiss(ctx, userID, view.Interpretation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dismissed.Interpretation.Status != "dismissed" {
		t.Fatalf("status %s", dismissed.Interpretation.Status)
	}
	pending, err := interpSvc.ListPending(ctx, userID, projectID)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending should be empty: %v %v", pending, err)
	}
	_ = repo
}

func TestMixedAccountsRejected(t *testing.T) {
	repo, interpSvc, projectSvc, _, userID, projectID, accountID := setupInterpret(t, "interpmix")
	ctx := context.Background()
	account2 := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID: userID, ID: account2, Label: "Personal", Provider: "m365",
		MsAccountKind: "personal", PrimaryEmail: "p@example.com", ConnectionStatus: "connected",
	}, []byte("tok2")); err != nil {
		t.Fatal(err)
	}
	msg1 := uuid.New()
	msg2 := uuid.New()
	conv1, conv2 := "c1", "c2"
	body := "duty"
	for _, pair := range []struct {
		id  uuid.UUID
		acc uuid.UUID
		cv  string
	}{
		{msg1, accountID, conv1},
		{msg2, account2, conv2},
	} {
		if err := repo.UpsertMessage(ctx, driven.MessageRow{
			ID: pair.id, AccountID: pair.acc, ProviderMessageID: pair.id.String(),
			ReceivedAt: time.Now().UTC(), Subject: "x", BodyText: &body,
			FromJSON: `{}`, ConversationID: &pair.cv,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := projectSvc.AssignMessage(ctx, userID, pair.id, appprojects.AssignInput{
			ProjectID: &projectID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
		}); err != nil {
			t.Fatal(err)
		}
	}
	interpSvc.LLM = &fakeLLM{content: `{"schema_version":1,"project_id":"` + projectID.String() + `","candidates":[]}`}
	_, err := interpSvc.Run(ctx, userID, projectID, appinterpret.RunInput{
		MessageIDs: []uuid.UUID{msg1, msg2},
	})
	if !errors.Is(err, appinterpret.ErrMixedAccounts) {
		t.Fatalf("want mixed accounts, got %v", err)
	}
}

func TestLLMUnavailable(t *testing.T) {
	_, interpSvc, _, _, userID, projectID, _ := setupInterpret(t, "interpnollm")
	_, err := interpSvc.Run(context.Background(), userID, projectID, appinterpret.RunInput{})
	if !errors.Is(err, appinterpret.ErrLLMUnavailable) {
		t.Fatalf("want llm unavailable, got %v", err)
	}
}

func TestNothingToInterpret(t *testing.T) {
	_, interpSvc, _, _, userID, projectID, _ := setupInterpret(t, "interpempty")
	interpSvc.LLM = &fakeLLM{content: `{"schema_version":1,"candidates":[]}`}
	_, err := interpSvc.Run(context.Background(), userID, projectID, appinterpret.RunInput{})
	if !errors.Is(err, appinterpret.ErrNothingToInterpret) {
		t.Fatalf("want nothing, got %v", err)
	}
}
