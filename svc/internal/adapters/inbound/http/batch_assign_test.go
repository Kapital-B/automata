package http

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type batchFixture struct {
	srv        *httptest.Server
	token      string
	repo       *sqlite.Repository
	projectSvc *appprojects.Service
	userID     uuid.UUID
	accountID  uuid.UUID
	hookCalls  *[][2]string
	hookMu     *sync.Mutex
}

func newBatchFixture(t *testing.T, dbName string) batchFixture {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbName+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	authSvc := auth.NewService(repo, repo, repo, nil, nil, jwtSecret, time.Hour, 30*24*time.Hour)

	var mu sync.Mutex
	calls := [][2]string{}
	projectSvc := &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Contacts: repo, Messages: repo, Manuals: repo,
		AfterProjectCorrespondence: func(ctx context.Context, userID, projectID uuid.UUID, messageID, manualItemID *uuid.UUID) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, [2]string{projectID.String(), "hook"})
		},
	}
	h := &Handlers{
		Log: slog.Default(), AuthSvc: authSvc, ProjectSvc: projectSvc,
		Users: repo, Messages: repo, Projects: repo, Assignments: repo, Contacts: repo,
		JWTSecret: jwtSecret, JWTTTL: time.Hour,
	}
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	userID, tokens := registerAndLogin(t, authSvc, dbName+"@example.com", "password123")
	accountID := uuid.New()
	if err := repo.InsertAccount(context.Background(), driven.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365",
		MsAccountKind: "work", PrimaryEmail: dbName + "@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	return batchFixture{
		srv: srv, token: tokens.AccessToken, repo: repo, projectSvc: projectSvc,
		userID: userID, accountID: accountID, hookCalls: &calls, hookMu: &mu,
	}
}

func (f batchFixture) insertMessage(t *testing.T, subject, conv string, at time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	row := driven.MessageRow{
		ID: id, AccountID: f.accountID, ProviderMessageID: id.String(),
		ReceivedAt: at, Subject: subject, FromJSON: `{"address":"a@b.com"}`,
	}
	if conv != "" {
		c := conv
		row.ConversationID = &c
	}
	if err := f.repo.UpsertMessage(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f batchFixture) postBatch(t *testing.T, items []map[string]any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"items": items})
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/api/project-assignments/batch", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestAssignBatchPartialFailure(t *testing.T) {
	f := newBatchFixture(t, "batchpartial")
	ctx := context.Background()
	now := time.Now().UTC()

	p, err := f.projectSvc.Create(ctx, f.userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}

	// Eleven assignable threads plus one message with no conversation, which
	// cannot be assigned at thread scope.
	items := []map[string]any{}
	good := []uuid.UUID{}
	for i := 0; i < 11; i++ {
		id := f.insertMessage(t, "subject", "conv-"+uuid.NewString(), now.Add(-time.Duration(i)*time.Minute))
		good = append(good, id)
		items = append(items, map[string]any{"kind": "message", "id": id.String(), "project_id": p.ID.String(), "scope": "thread"})
	}
	loose := f.insertMessage(t, "loose", "", now.Add(-time.Hour))
	items = append(items, map[string]any{"kind": "message", "id": loose.String(), "project_id": p.ID.String(), "scope": "thread"})

	status, out := f.postBatch(t, items)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got := int(out["assigned"].(float64)); got != 11 {
		t.Errorf("assigned = %d, want 11", got)
	}
	if got := int(out["failed"].(float64)); got != 1 {
		t.Errorf("failed = %d, want 1", got)
	}
	results := out["results"].([]any)
	if len(results) != 12 {
		t.Fatalf("results = %d, want 12", len(results))
	}
	last := results[11].(map[string]any)
	if last["ok"].(bool) {
		t.Error("message with no conversation should fail at thread scope")
	}
	if last["error"] != "conversation_required" {
		t.Errorf("error = %v, want conversation_required", last["error"])
	}

	// The eleven good threads really are assigned.
	for _, id := range good {
		eff, err := f.repo.EffectiveAssignment(ctx, f.userID, id)
		if err != nil {
			t.Fatal(err)
		}
		if eff.ProjectID == nil || *eff.ProjectID != p.ID {
			t.Fatalf("message %s not assigned: %+v", id, eff)
		}
		if eff.Status != "committed" || eff.Source != "user" {
			t.Errorf("want committed/user, got %s/%s", eff.Status, eff.Source)
		}
	}
}

func TestAssignBatchFiresHookOncePerProject(t *testing.T) {
	f := newBatchFixture(t, "batchhook")
	ctx := context.Background()
	now := time.Now().UTC()

	p1, err := f.projectSvc.Create(ctx, f.userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := f.projectSvc.Create(ctx, f.userID, appprojects.CreateProjectInput{Name: "Heating", Code: "DC02"})
	if err != nil {
		t.Fatal(err)
	}

	items := []map[string]any{}
	for i := 0; i < 4; i++ {
		id := f.insertMessage(t, "a", "conv-a-"+uuid.NewString(), now.Add(-time.Duration(i)*time.Minute))
		items = append(items, map[string]any{"kind": "message", "id": id.String(), "project_id": p1.ID.String(), "scope": "thread"})
	}
	for i := 0; i < 3; i++ {
		id := f.insertMessage(t, "b", "conv-b-"+uuid.NewString(), now.Add(-time.Duration(10+i)*time.Minute))
		items = append(items, map[string]any{"kind": "message", "id": id.String(), "project_id": p2.ID.String(), "scope": "thread"})
	}

	status, out := f.postBatch(t, items)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if got := int(out["assigned"].(float64)); got != 7 {
		t.Fatalf("assigned = %d, want 7", got)
	}

	f.hookMu.Lock()
	defer f.hookMu.Unlock()
	// Seven items across two projects must enqueue two downstream runs, not seven.
	if len(*f.hookCalls) != 2 {
		t.Fatalf("AfterProjectCorrespondence called %d times, want 2 (one per distinct project)", len(*f.hookCalls))
	}
	seen := map[string]bool{}
	for _, c := range *f.hookCalls {
		seen[c[0]] = true
	}
	if !seen[p1.ID.String()] || !seen[p2.ID.String()] {
		t.Errorf("hook projects = %v, want both %s and %s", seen, p1.ID, p2.ID)
	}
}

func TestAssignBatchIsIdempotent(t *testing.T) {
	f := newBatchFixture(t, "batchidem")
	ctx := context.Background()
	now := time.Now().UTC()
	p, err := f.projectSvc.Create(ctx, f.userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	id := f.insertMessage(t, "subject", "conv-1", now)
	items := []map[string]any{{"kind": "message", "id": id.String(), "project_id": p.ID.String(), "scope": "thread"}}

	for attempt := 0; attempt < 3; attempt++ {
		status, out := f.postBatch(t, items)
		if status != http.StatusOK || int(out["assigned"].(float64)) != 1 {
			t.Fatalf("attempt %d: status=%d out=%v", attempt, status, out)
		}
	}
	items2, err := f.repo.ListUnassigned(ctx, f.userID, driven.UnassignedListFilter{Status: "all", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(items2) != 0 {
		t.Fatalf("queue should be empty after assignment, got %d", len(items2))
	}
}

func TestAssignBatchRejectsOversizedRequest(t *testing.T) {
	f := newBatchFixture(t, "batchcap")
	items := make([]map[string]any, appprojects.MaxBatchAssignItems+1)
	for i := range items {
		items[i] = map[string]any{"kind": "message", "id": uuid.NewString(), "scope": "thread"}
	}
	status, out := f.postBatch(t, items)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if out["error"] != "too_many_items" {
		t.Errorf("error = %v, want too_many_items", out["error"])
	}
}

func TestAssignBatchClearsAssignment(t *testing.T) {
	f := newBatchFixture(t, "batchclear")
	ctx := context.Background()
	now := time.Now().UTC()
	p, err := f.projectSvc.Create(ctx, f.userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	id := f.insertMessage(t, "subject", "conv-clear", now)
	assign := []map[string]any{{"kind": "message", "id": id.String(), "project_id": p.ID.String(), "scope": "thread"}}
	if status, _ := f.postBatch(t, assign); status != http.StatusOK {
		t.Fatalf("assign status %d", status)
	}
	clear := []map[string]any{{"kind": "message", "id": id.String(), "project_id": nil, "scope": "thread"}}
	status, out := f.postBatch(t, clear)
	if status != http.StatusOK || int(out["assigned"].(float64)) != 1 {
		t.Fatalf("clear status=%d out=%v", status, out)
	}
	eff, err := f.repo.EffectiveAssignment(ctx, f.userID, id)
	if err != nil {
		t.Fatal(err)
	}
	if eff.ProjectID != nil {
		t.Fatalf("expected cleared assignment, got %v", eff.ProjectID)
	}
}

func TestAssignBatchPreservesItemOrder(t *testing.T) {
	f := newBatchFixture(t, "batchorder")
	ctx := context.Background()
	now := time.Now().UTC()
	p, err := f.projectSvc.Create(ctx, f.userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}

	// Alternate assignable and unassignable items so a result list that is
	// merely "all the successes then all the failures" cannot pass.
	items := []map[string]any{}
	wantOK := []bool{}
	ids := []string{}
	for i := 0; i < 6; i++ {
		var id uuid.UUID
		if i%2 == 0 {
			id = f.insertMessage(t, "ok", "conv-"+uuid.NewString(), now.Add(-time.Duration(i)*time.Minute))
			wantOK = append(wantOK, true)
		} else {
			id = f.insertMessage(t, "no conversation", "", now.Add(-time.Duration(i)*time.Minute))
			wantOK = append(wantOK, false)
		}
		ids = append(ids, id.String())
		items = append(items, map[string]any{
			"kind": "message", "id": id.String(), "project_id": p.ID.String(), "scope": "thread",
		})
	}

	status, out := f.postBatch(t, items)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	results := out["results"].([]any)
	if len(results) != len(items) {
		t.Fatalf("results = %d, want %d", len(results), len(items))
	}
	for i, raw := range results {
		row := raw.(map[string]any)
		if row["id"] != ids[i] {
			t.Errorf("result %d id = %v, want %v", i, row["id"], ids[i])
		}
		if row["ok"].(bool) != wantOK[i] {
			t.Errorf("result %d ok = %v, want %v", i, row["ok"], wantOK[i])
		}
	}
}

func TestAssignBatchWritesAcrossChunkBoundary(t *testing.T) {
	f := newBatchFixture(t, "batchchunk")
	ctx := context.Background()
	now := time.Now().UTC()
	p, err := f.projectSvc.Create(ctx, f.userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}

	// Repository writes commit in chunks of 100; span more than one chunk so a
	// boundary bug cannot hide.
	const count = 150
	items := make([]map[string]any, 0, count)
	ids := make([]uuid.UUID, 0, count)
	for i := 0; i < count; i++ {
		id := f.insertMessage(t, "subject", "conv-"+uuid.NewString(), now.Add(-time.Duration(i)*time.Minute))
		ids = append(ids, id)
		items = append(items, map[string]any{
			"kind": "message", "id": id.String(), "project_id": p.ID.String(), "scope": "thread",
		})
	}

	status, out := f.postBatch(t, items)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if got := int(out["assigned"].(float64)); got != count {
		t.Fatalf("assigned = %d, want %d", got, count)
	}
	for _, id := range ids {
		eff, err := f.repo.EffectiveAssignment(ctx, f.userID, id)
		if err != nil {
			t.Fatal(err)
		}
		if eff.ProjectID == nil || *eff.ProjectID != p.ID {
			t.Fatalf("message %s not assigned across chunk boundary", id)
		}
	}
}

func TestBatchMarksAndRestoresNotRelevant(t *testing.T) {
	f := newBatchFixture(t, "batchnotrelevant")
	ctx := context.Background()
	now := time.Now().UTC()

	keep := f.insertMessage(t, "keep", "conv-keep", now)
	drop := f.insertMessage(t, "newsletter", "conv-drop", now.Add(-time.Minute))

	// Mark.
	yes := true
	status, out := f.postBatch(t, []map[string]any{
		{"kind": "message", "id": drop.String(), "not_relevant": yes, "scope": "thread"},
	})
	if status != http.StatusOK || int(out["assigned"].(float64)) != 1 {
		t.Fatalf("mark status=%d out=%v", status, out)
	}

	queue, err := f.repo.ListUnassigned(ctx, f.userID, driven.UnassignedListFilter{Status: "all", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].MessageID == nil || *queue[0].MessageID != keep {
		t.Fatalf("queue should hold only the kept message, got %+v", queue)
	}

	dismissed, err := f.repo.ListUnassigned(ctx, f.userID, driven.UnassignedListFilter{Status: "not_relevant", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(dismissed) != 1 {
		t.Fatalf("dismissed = %d, want 1", len(dismissed))
	}

	// Restore.
	no := false
	status, out = f.postBatch(t, []map[string]any{
		{"kind": "message", "id": drop.String(), "not_relevant": no, "scope": "thread"},
	})
	if status != http.StatusOK || int(out["assigned"].(float64)) != 1 {
		t.Fatalf("restore status=%d out=%v", status, out)
	}
	queue, err = f.repo.ListUnassigned(ctx, f.userID, driven.UnassignedListFilter{Status: "all", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 2 {
		t.Fatalf("restored item should be back in the queue, got %d rows", len(queue))
	}
}

func TestBatchRejectsProjectAndNotRelevantTogether(t *testing.T) {
	f := newBatchFixture(t, "batchbothflags")
	ctx := context.Background()
	p, err := f.projectSvc.Create(ctx, f.userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
	if err != nil {
		t.Fatal(err)
	}
	id := f.insertMessage(t, "subject", "conv-1", time.Now().UTC())

	yes := true
	status, out := f.postBatch(t, []map[string]any{
		{"kind": "message", "id": id.String(), "project_id": p.ID.String(), "not_relevant": yes, "scope": "thread"},
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if int(out["failed"].(float64)) != 1 {
		t.Fatalf("an item cannot be both filed and dismissed: %v", out)
	}
	res := out["results"].([]any)[0].(map[string]any)
	if res["error"] != "project_and_not_relevant" {
		t.Errorf("error = %v, want project_and_not_relevant", res["error"])
	}
}

func TestSummaryReportsNotRelevantSeparately(t *testing.T) {
	f := newBatchFixture(t, "batchsummary")
	now := time.Now().UTC()
	f.insertMessage(t, "keep", "conv-keep", now)
	drop := f.insertMessage(t, "drop", "conv-drop", now.Add(-time.Minute))

	yes := true
	if status, _ := f.postBatch(t, []map[string]any{
		{"kind": "message", "id": drop.String(), "not_relevant": yes, "scope": "thread"},
	}); status != http.StatusOK {
		t.Fatalf("mark status %d", status)
	}

	req, _ := http.NewRequest(http.MethodGet, f.srv.URL+"/api/unassigned/summary", nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var sum map[string]any
	_ = json.NewDecoder(res.Body).Decode(&sum)

	// The badge counts work to do; dismissed items are not work.
	if int(sum["unassigned"].(float64)) != 1 {
		t.Errorf("unassigned = %v, want 1", sum["unassigned"])
	}
	if int(sum["not_relevant"].(float64)) != 1 {
		t.Errorf("not_relevant = %v, want 1", sum["not_relevant"])
	}
}
