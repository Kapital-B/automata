package http

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/attention"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appdecisions "github.com/Kapital-B/automata/svc/internal/application/decisions"
	appfacts "github.com/Kapital-B/automata/svc/internal/application/facts"
	appinterpret "github.com/Kapital-B/automata/svc/internal/application/interpret"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojectai "github.com/Kapital-B/automata/svc/internal/application/projectai"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	appreconcile "github.com/Kapital-B/automata/svc/internal/application/reconcile"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type wave2LLM struct {
	content string
}

func (f *wave2LLM) ChatCompletion(ctx context.Context, messages []driven.LLMMessage) (*driven.LLMResponse, error) {
	return &driven.LLMResponse{Content: f.content}, nil
}

func setupWave2HTTP(t *testing.T) (*httptest.Server, *auth.Service, *sqlite.Repository, *wave2LLM) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:wave2http-"+uuid.NewString()+"?mode=memory&cache=shared")
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
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	authSvc := auth.NewService(repo, repo, repo, nil, nil, jwtSecret, time.Hour, 30*24*time.Hour)
	projectSvc := &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Manuals: repo, Timeline: repo, Contacts: repo, Messages: repo,
	}
	factSvc := &appfacts.Service{
		Users: repo, Projects: repo, Facts: repo, Issues: repo, Assignments: repo, Manuals: repo, Messages: repo,
	}
	decisionSvc := &appdecisions.Service{
		Users: repo, Projects: repo, Decisions: repo, Issues: repo, Assignments: repo, Manuals: repo, Messages: repo,
	}
	llm := &wave2LLM{content: `{"schema_version":1,"candidates":[]}`}
	interpretSvc := &appinterpret.Service{
		Users: repo, Projects: repo, Interpretations: repo, Facts: repo, Timeline: repo,
		Assignments: repo, Manuals: repo, Messages: repo, JobRuns: repo, LLM: llm,
	}
	reconcileSvc := &appreconcile.Service{
		Users: repo, Projects: repo, Interpretations: repo, FactsRepo: repo, Facts: factSvc,
		Decisions: decisionSvc, Contradictions: repo, JobRuns: repo,
	}
	attentionSvc := &attention.Service{
		Users: repo, Projects: repo, Issues: repo, Facts: repo, Decisions: repo, Contradictions: repo,
	}
	projectAISvc := &appprojectai.Service{
		Users: repo, Projects: repo, Facts: repo, Decisions: repo, Issues: repo, Timeline: repo, JobRuns: repo, LLM: llm,
	}
	h := &Handlers{
		Log: slog.Default(), AuthSvc: authSvc, ProjectSvc: projectSvc,
		FactSvc: factSvc, InterpretSvc: interpretSvc, ReconcileSvc: reconcileSvc,
		DecisionSvc: decisionSvc, AttentionSvc: attentionSvc, ProjectAISvc: projectAISvc,
		Users: repo, Projects: repo, Messages: repo, JobRuns: repo,
		JWTSecret: jwtSecret, JWTTTL: time.Hour,
	}
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)
	return srv, authSvc, repo, llm
}

func authJSON(t *testing.T, method, url, token string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func decodeJSON(t *testing.T, res *http.Response, dest any) {
	t.Helper()
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(dest); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func pasteNote(t *testing.T, srv *httptest.Server, token, projectID, title, body string) string {
	t.Helper()
	res := authJSON(t, http.MethodPost, srv.URL+"/api/manual-items", token, map[string]any{
		"channel":     "note",
		"occurred_at": time.Now().UTC().Format(time.RFC3339),
		"title":       title,
		"body_text":   body,
		"project_id":  projectID,
	})
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("paste %d %s", res.StatusCode, b)
	}
	var row map[string]any
	decodeJSON(t, res, &row)
	id, _ := row["id"].(string)
	return id
}

func versionIDByStatus(fact map[string]any, status string) string {
	versions, _ := fact["versions"].([]any)
	for _, v := range versions {
		row, ok := v.(map[string]any)
		if !ok {
			continue
		}
		// Nested VersionView shape: { "version": {...}, "evidence": [...] }
		if inner, ok := row["version"].(map[string]any); ok {
			if inner["status"] == status {
				if id, ok := inner["id"].(string); ok {
					return id
				}
			}
			continue
		}
		if row["status"] == status {
			if id, ok := row["id"].(string); ok {
				return id
			}
		}
	}
	return ""
}

func countVersionStatus(fact map[string]any, status string) int {
	n := 0
	versions, _ := fact["versions"].([]any)
	for _, v := range versions {
		row, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if inner, ok := row["version"].(map[string]any); ok {
			if inner["status"] == status {
				n++
			}
			continue
		}
		if row["status"] == status {
			n++
		}
	}
	return n
}

func valueTextOfActive(fact map[string]any) string {
	versions, _ := fact["versions"].([]any)
	for _, v := range versions {
		row, ok := v.(map[string]any)
		if !ok {
			continue
		}
		inner := row
		if nested, ok := row["version"].(map[string]any); ok {
			inner = nested
		}
		if inner["status"] == "active" {
			if s, ok := inner["value_text"].(string); ok {
				return s
			}
		}
	}
	return ""
}

func TestWave2ExitCriteriaHTTP(t *testing.T) {
	srv, authSvc, repo, llm := setupWave2HTTP(t)
	userID, tokens := registerAndLogin(t, authSvc, "wave2@example.com", "password123")
	token := tokens.AccessToken
	ctx := context.Background()

	// Create project DC01
	res := authJSON(t, http.MethodPost, srv.URL+"/api/projects", token, map[string]any{
		"name": "Cooling Upgrade", "code": "DC01",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create project %d", res.StatusCode)
	}
	var project map[string]any
	decodeJSON(t, res, &project)
	projectID := project["id"].(string)

	// --- 2a: facts 75 → 90 with supersede ---
	res = authJSON(t, http.MethodPost, srv.URL+"/api/projects/"+projectID+"/facts", token, map[string]any{
		"subject_key": "pump.p03.duty_kw", "label": "Pump P-03 duty", "value": 75.0, "unit": "kW", "confirm": true,
	})
	if res.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("create fact 75: %d %s", res.StatusCode, body)
	}
	var fact75 map[string]any
	decodeJSON(t, res, &fact75)
	active75ID := versionIDByStatus(fact75, "active")
	if active75ID == "" {
		t.Fatalf("missing active 75 version: %+v", fact75)
	}

	res = authJSON(t, http.MethodPost, srv.URL+"/api/projects/"+projectID+"/facts", token, map[string]any{
		"subject_key": "pump.p03.duty_kw", "label": "Pump P-03 duty", "value": 90.0, "unit": "kW", "confirm": false,
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create proposed 90: %d", res.StatusCode)
	}
	var fact90 map[string]any
	decodeJSON(t, res, &fact90)
	proposed90ID := versionIDByStatus(fact90, "proposed")
	if proposed90ID == "" {
		t.Fatal("missing proposed 90 version")
	}

	res = authJSON(t, http.MethodPost, srv.URL+"/api/fact-versions/"+proposed90ID+"/confirm", token, map[string]any{
		"supersedes_version_id": active75ID,
	})
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("confirm 90: %d %s", res.StatusCode, body)
	}
	var confirmed map[string]any
	decodeJSON(t, res, &confirmed)
	if countVersionStatus(confirmed, "active") != 1 || countVersionStatus(confirmed, "superseded") != 1 {
		t.Fatalf("active=%d superseded=%d versions=%+v",
			countVersionStatus(confirmed, "active"), countVersionStatus(confirmed, "superseded"), confirmed["versions"])
	}
	if !strings.Contains(valueTextOfActive(confirmed), "90") {
		t.Fatalf("active value %+v", confirmed)
	}

	res = authJSON(t, http.MethodGet, srv.URL+"/api/projects/"+projectID+"/current-position", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("current position %d", res.StatusCode)
	}
	var pos map[string]any
	decodeJSON(t, res, &pos)
	facts := pos["facts"].([]any)
	if len(facts) != 1 {
		t.Fatalf("want 1 active fact, got %+v", pos)
	}
	if !strings.Contains(facts[0].(map[string]any)["value_text"].(string), "90") {
		t.Fatalf("current position %+v", facts[0])
	}

	// --- 2b: pending interpretation can be dismissed ---
	llm.content = `{"schema_version":1,"candidates":[{"kind":"fact","subject_key":"pump.p03.duty_kw","label":"Pump P-03 duty","value":95,"unit":"kW","confidence":0.5,"reason":"note","manual_item_ids":[]}]}`
	manID := pasteNote(t, srv, token, projectID, "Teams note", "Maybe duty is 95 kW")
	res = authJSON(t, http.MethodPost, srv.URL+"/api/projects/"+projectID+"/interpret", token, map[string]any{
		"manual_item_ids": []string{manID},
	})
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("interpret %d %s", res.StatusCode, body)
	}
	var interp map[string]any
	decodeJSON(t, res, &interp)
	interpID := interp["id"].(string)
	if interp["status"] != "pending" {
		t.Fatalf("want pending interpretation, got %+v", interp)
	}

	res = authJSON(t, http.MethodPost, srv.URL+"/api/interpretations/"+interpID+"/dismiss", token, map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("dismiss %d", res.StatusCode)
	}
	res.Body.Close()
	res = authJSON(t, http.MethodGet, srv.URL+"/api/projects/"+projectID+"/interpretations", token, nil)
	var pending []any
	decodeJSON(t, res, &pending)
	if len(pending) != 0 {
		t.Fatalf("pending after dismiss: %+v", pending)
	}

	// --- 2c: conflicting low-confidence claim → contradiction → resolve supersede ---
	llm.content = `{"schema_version":1,"candidates":[{"kind":"fact","subject_key":"pump.p03.duty_kw","label":"Pump P-03 duty","value":110,"unit":"kW","confidence":0.4,"reason":"conflict"}]}`
	man2 := pasteNote(t, srv, token, projectID, "Conflicting note", "Someone said 110 kW")
	res = authJSON(t, http.MethodPost, srv.URL+"/api/projects/"+projectID+"/interpret", token, map[string]any{
		"manual_item_ids": []string{man2},
	})
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("interpret conflict %d %s", res.StatusCode, body)
	}
	res.Body.Close()

	res = authJSON(t, http.MethodPost, srv.URL+"/api/projects/"+projectID+"/reconcile", token, map[string]any{})
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("reconcile %d %s", res.StatusCode, body)
	}
	var recon map[string]any
	decodeJSON(t, res, &recon)
	if int(recon["contradictions_opened"].(float64)) < 1 {
		t.Fatalf("want contradiction opened, got %+v", recon)
	}
	var contradictionID, proposedVersionID string
	for _, o := range recon["outcomes"].([]any) {
		row := o.(map[string]any)
		if row["outcome"] == "contradiction" {
			contradictionID = row["contradiction_id"].(string)
			proposedVersionID = row["version_id"].(string)
		}
	}
	if contradictionID == "" || proposedVersionID == "" {
		t.Fatalf("missing contradiction outcome %+v", recon)
	}

	res = authJSON(t, http.MethodGet, srv.URL+"/api/projects/"+projectID+"/contradictions?status=open", token, nil)
	var contrs []map[string]any
	decodeJSON(t, res, &contrs)
	if len(contrs) != 1 {
		t.Fatalf("open contradictions %+v", contrs)
	}

	res = authJSON(t, http.MethodPost, srv.URL+"/api/contradictions/"+contradictionID+"/resolve", token, map[string]any{
		"resolution": "supersede", "keep_fact_version_id": proposedVersionID,
	})
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("resolve %d %s", res.StatusCode, body)
	}
	var resolved map[string]any
	decodeJSON(t, res, &resolved)
	if resolved["status"] != "resolved" {
		t.Fatalf("want resolved, got %+v", resolved)
	}

	res = authJSON(t, http.MethodGet, srv.URL+"/api/projects/"+projectID+"/current-position", token, nil)
	decodeJSON(t, res, &pos)
	facts = pos["facts"].([]any)
	if len(facts) != 1 || !strings.Contains(facts[0].(map[string]any)["value_text"].(string), "110") {
		t.Fatalf("after resolve current position %+v", pos)
	}
	activeVersionID := facts[0].(map[string]any)["version_id"].(string)

	factID := facts[0].(map[string]any)["fact_id"].(string)
	res = authJSON(t, http.MethodGet, srv.URL+"/api/facts/"+factID, token, nil)
	var factDetail map[string]any
	decodeJSON(t, res, &factDetail)
	if countVersionStatus(factDetail, "active") != 1 {
		t.Fatalf("want exactly one active version, got %+v", factDetail["versions"])
	}

	// --- 2d: decision confirm + attention why_me ---
	res = authJSON(t, http.MethodPost, srv.URL+"/api/projects/"+projectID+"/decisions", token, map[string]any{
		"statement": "Proceed with 110 kW duty for Pump P-03", "confirm": false,
	})
	if res.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("create decision %d %s", res.StatusCode, body)
	}
	var decision map[string]any
	decodeJSON(t, res, &decision)
	decisionID := decision["id"].(string)

	res = authJSON(t, http.MethodGet, srv.URL+"/api/attention", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("attention %d", res.StatusCode)
	}
	var attn map[string]any
	decodeJSON(t, res, &attn)
	counts := attn["counts"].(map[string]any)
	if int(counts["total"].(float64)) < 1 || int(counts["provisional_decision"].(float64)) < 1 {
		t.Fatalf("attention counts %+v items %+v", counts, attn["items"])
	}
	foundWhy := false
	for _, it := range attn["items"].([]any) {
		row := it.(map[string]any)
		if row["why_me"] == "provisional_decision" && row["ref_id"] == decisionID {
			foundWhy = true
		}
	}
	if !foundWhy {
		t.Fatalf("missing provisional_decision item %+v", attn["items"])
	}

	res = authJSON(t, http.MethodPost, srv.URL+"/api/decisions/"+decisionID+"/confirm", token, map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("confirm decision %d", res.StatusCode)
	}
	res.Body.Close()

	res = authJSON(t, http.MethodGet, srv.URL+"/api/projects/"+projectID+"/current-position", token, nil)
	decodeJSON(t, res, &pos)
	decs := pos["decisions"].([]any)
	if len(decs) < 1 {
		t.Fatalf("want accepted decision on current position, got %+v", pos)
	}

	// Org isolation: other user sees empty attention / cannot see project
	_, tokensB := registerAndLogin(t, authSvc, "wave2b@example.com", "password123")
	res = authJSON(t, http.MethodGet, srv.URL+"/api/attention", tokensB.AccessToken, nil)
	decodeJSON(t, res, &attn)
	if int(attn["counts"].(map[string]any)["total"].(float64)) != 0 {
		t.Fatalf("user B attention should be empty, got %+v", attn)
	}
	res = authJSON(t, http.MethodGet, srv.URL+"/api/projects/"+projectID+"/facts", tokensB.AccessToken, nil)
	if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusBadRequest {
		// member check returns not found
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode == http.StatusOK {
			t.Fatalf("user B must not list project facts: %s", body)
		}
	} else {
		res.Body.Close()
	}

	// --- 2e: Ask Project AI cites active fact version ---
	askPayload, _ := json.Marshal(map[string]any{
		"schema_version": 1,
		"answer":         "Pump P-03 duty is 110 kW",
		"citations":      []map[string]string{{"type": "fact_version", "id": activeVersionID}},
		"confidence":     0.95,
	})
	llm.content = string(askPayload)
	res = authJSON(t, http.MethodPost, srv.URL+"/api/projects/"+projectID+"/ask", token, map[string]any{
		"question": "What is Pump P-03 duty?",
	})
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("ask %d %s", res.StatusCode, body)
	}
	var ask map[string]any
	decodeJSON(t, res, &ask)
	if !strings.Contains(ask["answer"].(string), "110") {
		t.Fatalf("ask answer %+v", ask)
	}
	cites := ask["citations"].([]any)
	if len(cites) < 1 {
		t.Fatalf("want citations, got %+v", ask)
	}
	cite := cites[0].(map[string]any)
	if cite["type"] != "fact_version" || cite["id"] != activeVersionID {
		t.Fatalf("citation %+v want fact_version %s", cite, activeVersionID)
	}

	// Ask with invented citation id is filtered; heuristic still answers
	llm.content = `{"schema_version":1,"answer":"Invented","citations":[{"type":"fact_version","id":"` + uuid.NewString() + `"}],"confidence":0.9}`
	res = authJSON(t, http.MethodPost, srv.URL+"/api/projects/"+projectID+"/ask", token, map[string]any{
		"question": "What is the pump duty?",
	})
	decodeJSON(t, res, &ask)
	cites = ask["citations"].([]any)
	if len(cites) < 1 {
		t.Fatalf("heuristic should restore grounded citation, got %+v", ask)
	}
	if cites[0].(map[string]any)["type"] != "fact_version" {
		t.Fatalf("want fact_version citation after filter, got %+v", ask)
	}

	// Sanity: user exists in repo (keeps migration/FK healthy)
	if _, err := repo.GetHomeOrganisationID(ctx, userID); err != nil {
		t.Fatal(err)
	}
}

func TestWave2AskInsufficientContext(t *testing.T) {
	srv, authSvc, _, llm := setupWave2HTTP(t)
	_, tokens := registerAndLogin(t, authSvc, "askempty@example.com", "password123")
	token := tokens.AccessToken
	res := authJSON(t, http.MethodPost, srv.URL+"/api/projects", token, map[string]any{
		"name": "Empty", "code": "EM01",
	})
	var project map[string]any
	decodeJSON(t, res, &project)
	projectID := project["id"].(string)

	llm.content = `{"schema_version":1,"answer":"I do not have enough grounded context in this project to answer that.","citations":[],"confidence":0}`
	res = authJSON(t, http.MethodPost, srv.URL+"/api/projects/"+projectID+"/ask", token, map[string]any{
		"question": "What is the chiller setpoint?",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ask %d", res.StatusCode)
	}
	var ask map[string]any
	decodeJSON(t, res, &ask)
	if len(ask["citations"].([]any)) != 0 {
		t.Fatalf("want no citations, got %+v", ask)
	}
	if !strings.Contains(strings.ToLower(ask["answer"].(string)), "enough") &&
		!strings.Contains(strings.ToLower(ask["answer"].(string)), "insufficient") &&
		!strings.Contains(strings.ToLower(ask["answer"].(string)), "do not") {
		t.Fatalf("want insufficiency message, got %+v", ask)
	}
}

func TestWave2ReconcileHighConfidenceSupersedeProposed(t *testing.T) {
	srv, authSvc, _, llm := setupWave2HTTP(t)
	_, tokens := registerAndLogin(t, authSvc, "reconhi@example.com", "password123")
	token := tokens.AccessToken
	res := authJSON(t, http.MethodPost, srv.URL+"/api/projects", token, map[string]any{"name": "Hi", "code": "HI01"})
	var project map[string]any
	decodeJSON(t, res, &project)
	projectID := project["id"].(string)

	res = authJSON(t, http.MethodPost, srv.URL+"/api/projects/"+projectID+"/facts", token, map[string]any{
		"subject_key": "pump.p03.duty_kw", "label": "Duty", "value": 75.0, "unit": "kW", "confirm": true,
	})
	res.Body.Close()

	llm.content = `{"schema_version":1,"candidates":[{"kind":"fact","subject_key":"pump.p03.duty_kw","label":"Duty","value":90,"unit":"kW","confidence":0.9}]}`
	manID := pasteNote(t, srv, token, projectID, "Update", "Duty now 90 kW")
	res = authJSON(t, http.MethodPost, srv.URL+"/api/projects/"+projectID+"/interpret", token, map[string]any{
		"manual_item_ids": []string{manID},
	})
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("interpret %d %s", res.StatusCode, body)
	}
	res.Body.Close()
	res = authJSON(t, http.MethodPost, srv.URL+"/api/projects/"+projectID+"/reconcile", token, map[string]any{})
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("reconcile %d %s", res.StatusCode, body)
	}
	var recon map[string]any
	decodeJSON(t, res, &recon)
	opened, _ := recon["contradictions_opened"].(float64)
	if int(opened) != 0 {
		t.Fatalf("high confidence should propose supersede not contradiction: %+v", recon)
	}
	found := false
	for _, o := range recon["outcomes"].([]any) {
		if o.(map[string]any)["outcome"] == "supersede" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want supersede outcome, got %+v", recon)
	}

	// Active value must still be 75 until user confirms proposed
	res = authJSON(t, http.MethodGet, srv.URL+"/api/projects/"+projectID+"/current-position", token, nil)
	var pos map[string]any
	decodeJSON(t, res, &pos)
	val := pos["facts"].([]any)[0].(map[string]any)["value_text"].(string)
	if !strings.Contains(val, "75") {
		t.Fatalf("must not silently overwrite active value, got %q", val)
	}
}
