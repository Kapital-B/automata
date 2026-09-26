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
	apptodos "github.com/Kapital-B/automata/svc/internal/application/todos"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// A project's to-dos are shared with its members: each sees everyone's, only
// the owner gets the ids that open the mail, and any member can close one.
// The issue page reads the same list, and resolving with complete_todos
// closes it.
func TestProjectAndIssueTodosHTTP(t *testing.T) {
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
	todoSvc := &apptodos.Service{Users: repo, Projects: repo, Summaries: repo}
	issueSvc := &appissues.Service{
		Users: repo, Projects: repo, Issues: repo, Assignments: repo, Manuals: repo, Contacts: repo, Messages: repo,
		Todos: todoSvc,
	}
	h := &Handlers{
		Log: slog.Default(), AuthSvc: authSvc, ProjectSvc: projectSvc, IssueSvc: issueSvc, TodoSvc: todoSvc,
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

	// A teammate, added to the project directly: the product cannot yet.
	mateID, mateTokens := registerAndLogin(t, authSvc, "mate@example.com", "password123")
	if _, err := db.Exec(`INSERT INTO project_members (id, project_id, user_id, role, created_at, updated_at) VALUES (?, ?, ?, 'member', ?, ?)`,
		uuid.New().String(), p.ID.String(), mateID.String(), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	// Teammates share the owner's organisation.
	orgID, err := repo.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET home_organisation_id = ? WHERE id = ?`, orgID.String(), mateID.String()); err != nil {
		t.Fatal(err)
	}
	_, outsiderTokens := registerAndLogin(t, authSvc, "outsider@example.com", "password123")

	call := func(token, method, path string, body any, want int) []byte {
		t.Helper()
		var rdr *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, srv.URL+path, rdr)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			t.Fatalf("%s %s: status %d, want %d", method, path, res.StatusCode, want)
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(res.Body)
		return buf.Bytes()
	}
	decodeList := func(b []byte) []map[string]any {
		var out []map[string]any
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	decodeObj := func(b []byte) map[string]any {
		var out map[string]any
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	todosPath := "/api/projects/" + p.ID.String() + "/todos"
	issuePath := "/api/issues/" + view.Issue.ID.String()

	// The owner sees their own to-do with the ids that open the mail.
	mine := decodeList(call(tokens.AccessToken, http.MethodGet, todosPath, nil, http.StatusOK))
	if len(mine) != 1 || mine[0]["id"] != todoID.String() || mine[0]["is_mine"] != true || mine[0]["owner_label"] != "You" ||
		mine[0]["message_id"] != msgID.String() || mine[0]["issue_title"] != "Pump P-03" || mine[0]["due_at"] == nil {
		t.Fatalf("owner's view: %v", mine)
	}
	// The teammate sees it too, named, without the mail.
	theirs := decodeList(call(mateTokens.AccessToken, http.MethodGet, todosPath, nil, http.StatusOK))
	if len(theirs) != 1 || theirs[0]["is_mine"] != false || theirs[0]["owner_label"] != "todos@example.com" {
		t.Fatalf("teammate's view: %v", theirs)
	}
	if _, ok := theirs[0]["message_id"]; ok {
		t.Fatalf("teammate must not get the message id: %v", theirs[0])
	}
	if _, ok := theirs[0]["account_id"]; ok {
		t.Fatalf("teammate must not get the account id: %v", theirs[0])
	}
	// Someone not on the project cannot list or close them.
	call(outsiderTokens.AccessToken, http.MethodGet, todosPath, nil, http.StatusNotFound)
	call(outsiderTokens.AccessToken, http.MethodPost, todosPath+"/"+todoID.String()+"/done", nil, http.StatusNotFound)
	// Nor can a colleague in the same organisation who is not on the project.
	colleagueID, colleagueTokens := registerAndLogin(t, authSvc, "colleague@example.com", "password123")
	if _, err := db.Exec(`UPDATE users SET home_organisation_id = ? WHERE id = ?`, orgID.String(), colleagueID.String()); err != nil {
		t.Fatal(err)
	}
	call(colleagueTokens.AccessToken, http.MethodGet, todosPath, nil, http.StatusNotFound)
	call(colleagueTokens.AccessToken, http.MethodPost, todosPath+"/"+todoID.String()+"/done", nil, http.StatusNotFound)
	// An id that is not an open to-do on the project is not found.
	call(mateTokens.AccessToken, http.MethodPost, todosPath+"/"+uuid.New().String()+"/done", nil, http.StatusNotFound)

	// The issue carries the same to-do.
	got := decodeObj(call(tokens.AccessToken, http.MethodGet, issuePath, nil, http.StatusOK))
	if todos, _ := got["todos"].([]any); len(todos) != 1 {
		t.Fatalf("issue to-dos: %v", got["todos"])
	}

	// The teammate closes the owner's to-do.
	call(mateTokens.AccessToken, http.MethodPost, todosPath+"/"+todoID.String()+"/done", nil, http.StatusOK)
	if left := decodeList(call(tokens.AccessToken, http.MethodGet, todosPath, nil, http.StatusOK)); len(left) != 0 {
		t.Fatalf("closed to-do still listed: %v", left)
	}
	open, err := repo.ListOpenActionItems(ctx, userID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("owner still has the to-do open: %+v", open)
	}

	// Resolving with complete_todos closes what is on the issue.
	second := uuid.New()
	if err := repo.InsertActionItems(ctx, []driven.ActionItemRow{{
		ID: second, UserID: userID, AccountID: accountID, MessageID: msgID, RunID: runID,
		Text: "Confirm the duty", Status: "open", CreatedAt: now, UpdatedAt: now,
	}}); err != nil {
		t.Fatal(err)
	}
	got = decodeObj(call(tokens.AccessToken, http.MethodPatch, issuePath, map[string]any{"status": "resolved", "complete_todos": true}, http.StatusOK))
	if todos, _ := got["todos"].([]any); len(todos) != 0 || got["status"] != "resolved" {
		t.Fatalf("after resolve: status %v todos %v", got["status"], got["todos"])
	}
	if open, _ := repo.ListOpenActionItems(ctx, userID, nil); len(open) != 0 {
		t.Fatalf("to-do still open after resolve: %+v", open)
	}
}
