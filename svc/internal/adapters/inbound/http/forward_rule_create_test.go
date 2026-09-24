package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/memoryjobs"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
)

type forwardAPI struct {
	t         *testing.T
	repo      *sqlite.Repository
	srv       *httptest.Server
	userID    uuid.UUID
	accountID uuid.UUID
}

func newForwardAPI(t *testing.T) *forwardAPI {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	a := &forwardAPI{t: t, repo: repo, userID: uuid.MustParse("a0000001-0000-4000-8000-000000000001"), accountID: uuid.New()}
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID: a.userID, ID: a.accountID, Label: "Work", Provider: "m365", MsAccountKind: domainacc.KindWork,
		PrimaryEmail: "work@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceForwardAllowlist(ctx, a.userID, []string{"dest@example.com"}); err != nil {
		t.Fatal(err)
	}
	h := &Handlers{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Accounts: repo, Forwards: repo, Messages: repo, Schedules: repo,
		ForwardRulesSvc: &appmessages.ForwardRulesService{Messages: repo, Forwards: repo, JobRuns: repo, Effects: memoryjobs.NewStore()},
		JWTSecret:       []byte("abcdefghijklmnopqrstuvwxyz123456"), DefaultUserID: a.userID,
	}
	a.srv = httptest.NewServer(h.Routes())
	t.Cleanup(a.srv.Close)
	return a
}

func (a *forwardAPI) do(method, path, body string, out any) (int, string) {
	a.t.Helper()
	req, err := http.NewRequest(method, a.srv.URL+path, strings.NewReader(body))
	if err != nil {
		a.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if out != nil {
		_ = json.Unmarshal(raw, out)
	}
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	return res.StatusCode, e.Error
}

const financeCondition = `{"all":[{"field":"category_slug","op":"equals","value":"finance"}]}`

func (a *forwardAPI) create(body string) (int, string, string) {
	var created struct {
		ID string `json:"id"`
	}
	status, msg := a.do(http.MethodPost, "/api/accounts/"+a.accountID.String()+"/forward-rules", body, &created)
	return status, msg, created.ID
}

func TestCreateForwardRuleDefaultsDisabled(t *testing.T) {
	a := newForwardAPI(t)
	status, msg, _ := a.create(`{"name":"t","mode":"logic","condition_json":` + financeCondition + `,"forward_to":"dest@example.com"}`)
	if status != http.StatusCreated {
		t.Fatalf("status %d: %s", status, msg)
	}
	rules, _ := a.repo.ListForwardRules(context.Background(), a.userID, a.accountID)
	if len(rules) != 1 || rules[0].Enabled {
		t.Fatalf("rules = %+v, want one paused rule", rules)
	}
}

// Review findings 6 and 7: an empty condition used to be saved, and then
// forwarded every message; an unknown field was saved and never matched.
func TestCreateForwardRuleRefusesInvalidRules(t *testing.T) {
	a := newForwardAPI(t)
	cases := map[string]struct{ body, want string }{
		"empty condition":   {`{"name":"t","mode":"logic","condition_json":{"all":[]},"forward_to":"dest@example.com"}`, "at least one condition"},
		"unknown field":     {`{"name":"t","mode":"logic","condition_json":{"all":[{"field":"x","op":"equals","value":"y"}]},"forward_to":"dest@example.com"}`, "unknown condition field"},
		"not allowlisted":   {`{"name":"t","mode":"logic","condition_json":` + financeCondition + `,"forward_to":"other@example.com"}`, "allowlist"},
		"no name":           {`{"name":" ","mode":"logic","condition_json":` + financeCondition + `,"forward_to":"dest@example.com"}`, "name"},
		"enabled, no start": {`{"name":"t","mode":"logic","condition_json":` + financeCondition + `,"forward_to":"dest@example.com","enabled":true}`, "existing mail"},
	}
	for name, c := range cases {
		status, msg, _ := a.create(c.body)
		if status != http.StatusUnprocessableEntity || !strings.Contains(msg, c.want) {
			t.Errorf("%s: %d %q, want 422 mentioning %q", name, status, msg, c.want)
		}
	}
	if rules, _ := a.repo.ListForwardRules(context.Background(), a.userID, a.accountID); len(rules) != 0 {
		t.Fatalf("an invalid rule was saved: %+v", rules)
	}
}

func TestCreateForwardRuleOnAnotherUsersAccountIsNotFound(t *testing.T) {
	a := newForwardAPI(t)
	other := uuid.New()
	if err := a.repo.InsertAccount(context.Background(), driven.AccountRow{
		UserID: uuid.New(), ID: other, Label: "Theirs", Provider: "m365", MsAccountKind: domainacc.KindWork,
		PrimaryEmail: "them@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	status, _ := a.do(http.MethodPost, "/api/accounts/"+other.String()+"/forward-rules",
		`{"name":"t","mode":"logic","condition_json":`+financeCondition+`,"forward_to":"dest@example.com"}`, nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

// Switching a rule on is a decision about existing mail, and the scope it
// gets reflects it.
func TestSwitchingARuleOnSetsItsScope(t *testing.T) {
	a := newForwardAPI(t)
	_, _, id := a.create(`{"name":"t","mode":"logic","condition_json":` + financeCondition + `,"forward_to":"dest@example.com"}`)
	scope := func() time.Time {
		rules, _ := a.repo.ListForwardRules(context.Background(), a.userID, a.accountID)
		return appmessages.RuleScopeStart(rules[0])
	}
	body := func(extra string) string {
		return `{"name":"t","mode":"logic","condition_json":` + financeCondition + `,"forward_to":"dest@example.com","enabled":true` + extra + `}`
	}
	if status, msg := a.do(http.MethodPatch, "/api/forward-rules/"+id, body(`,"start":"existing"`), nil); status != http.StatusOK {
		t.Fatalf("enable: %d %s", status, msg)
	}
	if !scope().Equal(time.Unix(0, 0).UTC()) {
		t.Fatalf("scope = %v, want all existing mail", scope())
	}
	before := time.Now().UTC().Add(-time.Second)
	a.do(http.MethodPatch, "/api/forward-rules/"+id, body(`,"start":"new"`), nil)
	if s := scope(); s.Before(before) {
		t.Fatalf("scope = %v, want now", s)
	}
	// Editing without a start keeps the scope it has.
	kept := scope()
	a.do(http.MethodPatch, "/api/forward-rules/"+id, strings.Replace(body(""), `"name":"t"`, `"name":"renamed"`, 1), nil)
	if !scope().Equal(kept) {
		t.Fatalf("an edit changed the scope from %v to %v", kept, scope())
	}
}

// Review finding 8, seen from the page: a rule whose destination left the
// allowlist says so, instead of failing on every message.
func TestListReportsBlockedRulesAndStats(t *testing.T) {
	a := newForwardAPI(t)
	_, _, id := a.create(`{"name":"t","mode":"logic","condition_json":` + financeCondition + `,"forward_to":"dest@example.com"}`)
	a.do(http.MethodPut, "/api/forward-allowlist", `{"emails":["new@example.com"]}`, nil)
	var rules []struct {
		ID            string         `json:"id"`
		BlockedReason string         `json:"blocked_reason"`
		Stats         map[string]any `json:"stats"`
	}
	a.do(http.MethodGet, "/api/accounts/"+a.accountID.String()+"/forward-rules", "", &rules)
	if len(rules) != 1 || rules[0].ID != id || !strings.Contains(rules[0].BlockedReason, "allowlist") || rules[0].Stats == nil {
		t.Fatalf("rules = %+v", rules)
	}
}

func TestAllowlistRefusesWhatIsNotAnAddress(t *testing.T) {
	a := newForwardAPI(t)
	status, msg := a.do(http.MethodPut, "/api/forward-allowlist", `{"emails":["ok@example.com","not an address"]}`, nil)
	if status != http.StatusUnprocessableEntity || !strings.Contains(msg, "not an address") {
		t.Fatalf("%d %q", status, msg)
	}
	var list struct {
		Emails []string `json:"emails"`
	}
	a.do(http.MethodGet, "/api/forward-allowlist", "", &list)
	if len(list.Emails) != 1 || list.Emails[0] != "dest@example.com" {
		t.Fatalf("allowlist changed on a refused update: %v", list.Emails)
	}
}

func TestPreviewAndActivity(t *testing.T) {
	a := newForwardAPI(t)
	ctx := context.Background()
	msgID := uuid.New()
	body := "Invoice attached"
	if err := a.repo.UpsertMessage(ctx, driven.MessageRow{
		ID: msgID, AccountID: a.accountID, ProviderMessageID: "p1", ReceivedAt: time.Now().UTC(), Subject: "Invoice 7",
		FromJSON: `{"name":"Vendor","address":"billing@vendor.example"}`, BodyText: &body, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	var preview struct {
		InScope int              `json:"in_scope"`
		Matched *int             `json:"matched"`
		Samples []map[string]any `json:"samples"`
	}
	status, msg := a.do(http.MethodPost, "/api/accounts/"+a.accountID.String()+"/forward-rules/preview",
		`{"mode":"logic","condition_json":{"all":[{"field":"subject","op":"contains","value":"invoice"}]}}`, &preview)
	if status != http.StatusOK || preview.InScope != 1 || preview.Matched == nil || *preview.Matched != 1 || len(preview.Samples) != 1 {
		t.Fatalf("preview %d %s: %+v", status, msg, preview)
	}

	_, _, id := a.create(`{"name":"t","mode":"logic","condition_json":` + financeCondition + `,"forward_to":"dest@example.com"}`)
	reason := "forwarded to dest@example.com"
	if err := a.repo.InsertForwardAudit(ctx, driven.ForwardAuditRow{
		ID: uuid.New(), UserID: a.userID, AccountID: a.accountID, MessageID: msgID, RuleID: uuid.MustParse(id),
		RunID: uuid.New(), Status: "forwarded", Reason: &reason, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	var activity []map[string]any
	a.do(http.MethodGet, "/api/forward-rules/"+id+"/activity", "", &activity)
	if len(activity) != 1 || activity[0]["subject"] != "Invoice 7" || activity[0]["status"] != "forwarded" {
		t.Fatalf("activity = %+v", activity)
	}
}
