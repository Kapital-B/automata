package http

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appissues "github.com/Kapital-B/automata/svc/internal/application/issues"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// The issue page reads the caller's to-dos from the issue, and resolving it
// with complete_todos closes them.
func TestIssueHTTPTodosAndCompleteOnResolve(t *testing.T) {
	db, err := sql.Open("sqlite", "file:issuetodoshttp?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, _ = db.Exec(`PRAGMA foreign_keys=ON`)
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	authSvc := auth.NewService(repo, repo, repo, nil, nil, jwtSecret, time.Hour, 30*24*time.Hour)
	projectSvc := &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Manuals: repo, Timeline: repo, Contacts: repo, Messages: repo,
	}
	issueSvc := &appissues.Service{
		Users: repo, Projects: repo, Issues: repo, Assignments: repo, Manuals: repo, Contacts: repo, Messages: repo,
		Summaries: repo,
	}
	h := &Handlers{
		Log: slog.Default(), AuthSvc: authSvc, ProjectSvc: projectSvc, IssueSvc: issueSvc,
		Users: repo, Projects: repo, Assignments: repo, Issues: repo, Messages: repo,
		JWTSecret: jwtSecret, JWTTTL: time.Hour,
	}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	userID, tokens := registerAndLogin(t, authSvc, "todos@example.com", "password123")
	p, err := projectSvc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365",
		MsAccountKind: "work", PrimaryEmail: "todos@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	msgID := uuid.New()
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: accountID, ProviderMessageID: msgID.String(),
		ReceivedAt: now, Subject: "Pump duty", FromJSON: `{}`,
	}); err != nil {
		t.Fatal(err)
	}
	runID := uuid.New()
	if err := repo.InsertJobRun(ctx, runID, accountID, "summarize", "api", "success", now, now, nil, `{}`); err != nil {
		t.Fatal(err)
	}
	todoID := uuid.New()
	due := now.Add(48 * time.Hour)
	if err := repo.InsertActionItems(ctx, []driven.ActionItemRow{{
		ID: todoID, UserID: userID, AccountID: accountID, MessageID: msgID, RunID: runID,
		Text: "Reply to Jan", DueAt: &due, Status: "open", CreatedAt: now, UpdatedAt: now,
	}}); err != nil {
		t.Fatal(err)
	}
	view, err := issueSvc.Create(ctx, userID, p.ID, appissues.CreateInput{Title: "Pump P-03"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AddIssueItem(ctx, driven.IssueItemRow{ID: uuid.New(), IssueID: view.Issue.ID, MessageID: &msgID, AddedAt: now}); err != nil {
		t.Fatal(err)
	}

	do := func(method string, body any) map[string]any {
		t.Helper()
		var rdr *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, srv.URL+"/api/issues/"+view.Issue.ID.String(), rdr)
		req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s status %d", method, res.StatusCode)
		}
		var out map[string]any
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	got := do(http.MethodGet, nil)
	todos, _ := got["todos"].([]any)
	if len(todos) != 1 {
		t.Fatalf("want one to-do, got %v", got["todos"])
	}
	first, _ := todos[0].(map[string]any)
	if first["id"] != todoID.String() || first["text"] != "Reply to Jan" || first["message_id"] != msgID.String() || first["due_at"] == nil {
		t.Fatalf("to-do shape: %v", first)
	}

	got = do(http.MethodPatch, map[string]any{"status": "resolved", "complete_todos": true})
	if todos, _ := got["todos"].([]any); len(todos) != 0 || got["status"] != "resolved" {
		t.Fatalf("after resolve: status %v todos %v", got["status"], got["todos"])
	}
	open, err := repo.ListOpenActionItems(ctx, userID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("to-do still open after resolve: %+v", open)
	}
}
