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
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestProjectsHTTPIsolation(t *testing.T) {
	db, err := sql.Open("sqlite", "file:projectshttp?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	authSvc := auth.NewService(repo, repo, repo, nil, nil, jwtSecret, time.Hour, 30*24*time.Hour)
	projectSvc := &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo,
	}
	h := &Handlers{
		Log: slog.Default(), AuthSvc: authSvc, ProjectSvc: projectSvc,
		Users: repo, Contacts: repo, Messages: repo, Projects: repo, Assignments: repo,
		JWTSecret: jwtSecret, JWTTTL: time.Hour,
	}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	_, tokensA := registerAndLogin(t, authSvc, "pa@example.com", "password123")
	_, tokensB := registerAndLogin(t, authSvc, "pb@example.com", "password123")

	body, _ := json.Marshal(map[string]any{"name": "Cooling", "code": "DC01"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/projects", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokensA.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d", res.StatusCode)
	}

	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/projects", nil)
	req2.Header.Set("Authorization", "Bearer "+tokensB.AccessToken)
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	var list []map[string]any
	_ = json.NewDecoder(res2.Body).Decode(&list)
	if len(list) != 0 {
		t.Fatalf("user B must not see user A projects, got %d", len(list))
	}

	req3, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/unassigned/summary", nil)
	req3.Header.Set("Authorization", "Bearer "+tokensA.AccessToken)
	res3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusOK {
		t.Fatalf("summary status %d", res3.StatusCode)
	}
}

func TestAssignMessageHTTP(t *testing.T) {
	db, err := sql.Open("sqlite", "file:assignhttp?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	authSvc := auth.NewService(repo, repo, repo, nil, nil, jwtSecret, time.Hour, 30*24*time.Hour)
	projectSvc := &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo,
	}
	h := &Handlers{
		Log: slog.Default(), AuthSvc: authSvc, ProjectSvc: projectSvc,
		Users: repo, Messages: repo, Projects: repo, Assignments: repo,
		JWTSecret: jwtSecret, JWTTTL: time.Hour,
	}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	userID, tokens := registerAndLogin(t, authSvc, "assign@example.com", "password123")
	ctx := context.Background()
	now := time.Now().UTC()
	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365",
		MsAccountKind: "work", PrimaryEmail: "assign@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	p, err := projectSvc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	conv := "thread-1"
	msgID := uuid.New()
	if err := repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: accountID, ProviderMessageID: "pm1",
		ReceivedAt: now, Subject: "hi", FromJSON: `{}`, ConversationID: &conv,
	}); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{
		"project_id": p.ID.String(),
		"scope":      "thread",
		"status":     "committed",
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/messages/"+msgID.String()+"/project-assignment", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("assign status %d", res.StatusCode)
	}
	var eff map[string]any
	_ = json.NewDecoder(res.Body).Decode(&eff)
	if eff["project_id"] != p.ID.String() {
		t.Fatalf("eff=%v", eff)
	}
}
