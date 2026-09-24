package messages

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/memoryjobs"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

var epoch = time.Unix(0, 0).UTC()

// forwardFixture is an account with one invoice message, categorised
// "finance", and an allowlisted destination.
type forwardFixture struct {
	db        *sql.DB
	repo      *sqlite.Repository
	svc       *ForwardRulesService
	box       *fakeMailbox
	userID    uuid.UUID
	accountID uuid.UUID
	messageID uuid.UUID
	ruleID    uuid.UUID
}

func setupForwardRulesService(t *testing.T, graph *fakeMailbox) (*sql.DB, *ForwardRulesService, *sqlite.Repository, uuid.UUID, uuid.UUID, uuid.UUID) {
	f := newForwardFixture(t, graph)
	return f.db, f.svc, f.repo, f.userID, f.accountID, f.messageID
}

func newForwardFixture(t *testing.T, box *fakeMailbox) *forwardFixture {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	f := &forwardFixture{db: db, repo: repo, box: box, userID: uuid.New(), accountID: uuid.New()}
	ctx := context.Background()
	if _, err := repo.CreateUserWithHomeOrg(ctx, f.userID, "u@example.com", nil, time.Now().UTC(), "password", f.userID.String(), "u@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID: f.userID, ID: f.accountID, Label: "Work", Provider: "m365",
		MsAccountKind: domainacc.KindWork, PrimaryEmail: "work@example.com", ConnectionStatus: "connected",
	}, []byte("credential")); err != nil {
		t.Fatal(err)
	}
	f.messageID = f.message(t, "Invoice - INV0018315", time.Date(2026, 5, 1, 9, 12, 20, 0, time.UTC), "finance")
	if err := repo.ReplaceForwardAllowlist(ctx, f.userID, []string{"bills@example.com", "books@example.com"}); err != nil {
		t.Fatal(err)
	}
	f.ruleID = f.rule(t, `{"all":[{"field":"category_slug","op":"equals","value":"finance"}]}`, "bills@example.com", &epoch)
	f.svc = &ForwardRulesService{
		Messages: repo, Forwards: repo, Mailboxes: testOpener(repo, box), JobRuns: repo,
		Effects: memoryjobs.NewStore(),
	}
	return f
}

func (f *forwardFixture) message(t *testing.T, subject string, received time.Time, category string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	id := uuid.New()
	body := subject + " body"
	if err := f.repo.UpsertMessage(ctx, driven.MessageRow{
		ID: id, AccountID: f.accountID, ProviderMessageID: "p-" + id.String(), ReceivedAt: received,
		Subject: subject, FromJSON: `{"name":"Vendor","address":"billing@vendor.example"}`, BodyText: &body,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if category != "" {
		f.categorise(t, id, category)
	}
	return id
}

func (f *forwardFixture) categorise(t *testing.T, id uuid.UUID, slug string) {
	t.Helper()
	ctx := context.Background()
	def, err := f.repo.GetCategoryDefinitionBySlug(ctx, f.userID, slug)
	if err != nil {
		t.Fatal(err)
	}
	if def == nil {
		d := driven.CategoryDefinitionRow{
			ID: uuid.New(), UserID: f.userID, Slug: slug, DisplayName: slug, Definition: slug,
			SortOrder: 99, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		if err := f.repo.CreateCategoryDefinition(ctx, d); err != nil {
			t.Fatal(err)
		}
		def = &d
	}
	conf := 0.9
	if err := f.repo.UpsertMessageCategory(ctx, driven.MessageCategoryRow{
		ID: uuid.New(), MessageID: id, AccountID: f.accountID, CategoryID: def.ID, Source: "llm",
		Confidence: &conf, RunID: uuid.New(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *forwardFixture) rule(t *testing.T, condition, to string, applyFrom *time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := f.repo.CreateForwardRule(context.Background(), driven.ForwardRuleRow{
		ID: id, UserID: f.userID, AccountID: f.accountID, Name: "rule", Mode: ForwardModeLogic,
		ConditionJSON: condition, ForwardTo: to, Enabled: true, ApplyFrom: applyFrom,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *forwardFixture) run(t *testing.T) {
	t.Helper()
	if _, err := f.svc.RunAccount(context.Background(), f.userID, f.accountID, ForwardRulesOptions{Trigger: "test"}); err != nil {
		t.Fatal(err)
	}
}

func (f *forwardFixture) verdict(t *testing.T, messageID, ruleID uuid.UUID) *driven.ForwardAuditRow {
	t.Helper()
	rows, err := f.repo.ListForwardAuditForMessages(context.Background(), f.userID, []uuid.UUID{messageID})
	if err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		if rows[i].RuleID == ruleID {
			return &rows[i]
		}
	}
	return nil
}

func TestForwardRuleForwardsMatchingMailOnce(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{})
	f.run(t)
	f.run(t)
	if f.box.forwardCalls != 1 {
		t.Fatalf("forward calls = %d, want 1 across two runs", f.box.forwardCalls)
	}
	if v := f.verdict(t, f.messageID, f.ruleID); v == nil || v.Status != "forwarded" || v.Pending {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestForwardRuleSkipsWhatDoesNotMatch(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{})
	other := f.message(t, "Lunch?", time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC), "personal")
	f.run(t)
	if v := f.verdict(t, other, f.ruleID); v == nil || v.Status != "skipped" || v.Pending {
		t.Fatalf("verdict = %+v, want a final skip", v)
	}
	if f.box.forwardCalls != 1 {
		t.Fatalf("forward calls = %d, want only the invoice", f.box.forwardCalls)
	}
}

// Switching a rule on for new mail only must not reach back through the
// mailbox: the first run used to walk every message ever synced.
func TestNewMailOnlyRuleLeavesExistingMailAlone(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{})
	if err := f.repo.DeleteForwardRule(context.Background(), f.userID, f.ruleID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	rule := f.rule(t, `{"all":[{"field":"category_slug","op":"equals","value":"finance"}]}`, "bills@example.com", &now)
	f.run(t)
	if f.box.forwardCalls != 0 {
		t.Fatalf("forwarded %d existing messages under a new-mail-only rule", f.box.forwardCalls)
	}
	if v := f.verdict(t, f.messageID, rule); v != nil {
		t.Fatalf("existing mail was evaluated: %+v", v)
	}
	fresh := f.message(t, "Invoice - INV0019000", now.Add(time.Minute), "finance")
	f.run(t)
	if v := f.verdict(t, fresh, rule); v == nil || v.Status != "forwarded" {
		t.Fatalf("new mail verdict = %+v", v)
	}
}

// A rule added after forwarding has run must still be able to cover existing
// mail. It used to see only mail that arrived after it, because the old
// scheme had marked everything else seen.
func TestRuleAddedLaterCanCoverExistingMail(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{})
	f.run(t)
	later := f.rule(t, `{"all":[{"field":"subject","op":"contains","value":"invoice"}]}`, "books@example.com", &epoch)
	f.run(t)
	if v := f.verdict(t, f.messageID, later); v == nil || v.Status != "forwarded" {
		t.Fatalf("later rule verdict = %+v", v)
	}
	if strings.Join(f.box.forwardedTo, ",") != "bills@example.com,books@example.com" {
		t.Fatalf("forwarded to %v", f.box.forwardedTo)
	}
}

// Review finding 1: a send that failed without going out was recorded as
// retryable, then treated as forwarded on the retry and never sent.
func TestForwardThatDidNotGoOutIsRetried(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{forwardErr: fmt.Errorf("%w: transient", driven.ErrMailNotSent)})
	f.run(t)
	if v := f.verdict(t, f.messageID, f.ruleID); v == nil || !v.Pending || v.Attempts != 1 {
		t.Fatalf("after a failed send, verdict = %+v, want pending with one attempt", v)
	}
	f.box.forwardErr = nil
	f.run(t)
	if f.box.forwardCalls != 2 || len(f.box.forwardedTo) != 1 {
		t.Fatalf("calls = %d, delivered = %v; the retry must actually send", f.box.forwardCalls, f.box.forwardedTo)
	}
	if v := f.verdict(t, f.messageID, f.ruleID); v == nil || v.Status != "forwarded" || v.Pending {
		t.Fatalf("verdict = %+v", v)
	}
}

// A message that can never be sent (moved out of the inbox, say) is given up
// on and says so, instead of being retried every run.
func TestForwardThatCanNeverGoOutIsGivenUp(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{forwardErr: fmt.Errorf("%w: message not in inbox", driven.ErrMailNotSent)})
	for i := 0; i < maxForwardAttempts+2; i++ {
		f.run(t)
	}
	if f.box.forwardCalls != maxForwardAttempts {
		t.Fatalf("calls = %d, want %d then stop", f.box.forwardCalls, maxForwardAttempts)
	}
	v := f.verdict(t, f.messageID, f.ruleID)
	if v == nil || v.Pending || v.Status != "failed" || !strings.Contains(*v.Reason, "gave up") {
		t.Fatalf("verdict = %+v", v)
	}
}

// A send whose outcome is unknown may have gone out; resending could
// duplicate it.
func TestUnknownOutcomeIsNotResent(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{forwardErr: errors.New("connection reset")})
	f.run(t)
	f.box.forwardErr = nil
	f.run(t)
	if f.box.forwardCalls != 1 {
		t.Fatalf("calls = %d, want no resend", f.box.forwardCalls)
	}
	if v := f.verdict(t, f.messageID, f.ruleID); v == nil || v.Pending || !strings.Contains(*v.Reason, "unknown") {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestTwoRulesToTheSameAddressSendOnce(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{})
	second := f.rule(t, `{"all":[{"field":"from","op":"domain","value":"vendor.example"}]}`, "bills@example.com", &epoch)
	f.run(t)
	if f.box.forwardCalls != 1 {
		t.Fatalf("calls = %d, want one send to one address", f.box.forwardCalls)
	}
	if v := f.verdict(t, f.messageID, second); v == nil || v.Status != "forwarded" || !strings.Contains(*v.Reason, "already forwarded") {
		t.Fatalf("second rule verdict = %+v", v)
	}
}

// Review finding 5: forwarding could run before categorisation reached a
// message, skip it, and never look again.
func TestCategoryRuleWaitsForCategorisation(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{})
	fresh := f.message(t, "Invoice - fresh", time.Now().UTC().Add(-time.Hour), "")
	f.run(t)
	if v := f.verdict(t, fresh, f.ruleID); v == nil || !v.Pending {
		t.Fatalf("uncategorised verdict = %+v, want pending", v)
	}
	f.categorise(t, fresh, "finance")
	f.run(t)
	if v := f.verdict(t, fresh, f.ruleID); v == nil || v.Status != "forwarded" {
		t.Fatalf("after categorisation, verdict = %+v", v)
	}

	// Mail that was never categorised stops waiting eventually.
	stale := f.message(t, "Invoice - stale", time.Now().UTC().Add(-categoryWait-time.Hour), "")
	f.run(t)
	if v := f.verdict(t, stale, f.ruleID); v == nil || v.Pending || v.Status != "skipped" {
		t.Fatalf("stale uncategorised verdict = %+v, want a final skip", v)
	}
}

// Review finding 8: removing a destination from the allowlist failed its rule
// on every message, every run, and kept them all unfinished.
func TestRuleToRemovedAddressIsBlockedNotFailed(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{})
	if err := f.repo.ReplaceForwardAllowlist(context.Background(), f.userID, []string{"books@example.com"}); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if f.box.forwardCalls != 0 {
		t.Fatalf("sent to an address no longer allowlisted")
	}
	if v := f.verdict(t, f.messageID, f.ruleID); v != nil {
		t.Fatalf("a blocked rule wrote a verdict: %+v", v)
	}
	rules, _ := f.repo.ListForwardRules(context.Background(), f.userID, f.accountID)
	allow, _ := f.repo.ListForwardAllowlist(context.Background(), f.userID)
	if got := ForwardRuleBlock(rules[0], AllowlistSet(allow)); !strings.Contains(got, "allowlist") {
		t.Fatalf("block reason = %q", got)
	}
}

// Candidates page oldest first across chunks, and each message is handled
// once.
func TestForwardRunPagesThroughEveryCandidate(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{})
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < forwardChunkInline+7; i++ {
		f.message(t, fmt.Sprintf("Invoice %d", i), base.Add(time.Duration(i)*time.Minute), "finance")
	}
	f.run(t)
	if f.box.forwardCalls != forwardChunkInline+8 {
		t.Fatalf("calls = %d, want every invoice once", f.box.forwardCalls)
	}
}

type scriptedLLM struct {
	answers []string
	calls   int
	prompts []string
}

func (s *scriptedLLM) ChatCompletion(ctx context.Context, msgs []driven.LLMMessage) (*driven.LLMResponse, error) {
	for _, m := range msgs {
		s.prompts = append(s.prompts, m.Role+": "+m.Content)
	}
	a := s.answers[min(s.calls, len(s.answers)-1)]
	s.calls++
	return &driven.LLMResponse{Content: a}, nil
}

func TestAIRuleTreatsTheEmailAsDataAndBoundsTheBody(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{})
	llm := &scriptedLLM{answers: []string{`{"forward":false,"reason":"not a tax document"}`}}
	f.svc.LLM = llm
	if err := f.repo.DeleteForwardRule(context.Background(), f.userID, f.ruleID); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", forwardLLMBodyChars*3)
	id := uuid.New()
	if err := f.repo.UpsertMessage(context.Background(), driven.MessageRow{
		ID: id, AccountID: f.accountID, ProviderMessageID: "big", ReceivedAt: time.Now().UTC(), Subject: "Huge",
		FromJSON: `{"address":"a@b.example"}`, BodyText: &big, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	rule := uuid.New()
	if err := f.repo.CreateForwardRule(context.Background(), driven.ForwardRuleRow{
		ID: rule, UserID: f.userID, AccountID: f.accountID, Name: "tax", Mode: ForwardModeLLM,
		ConditionJSON: `{"prompt":"tax documents"}`, ForwardTo: "bills@example.com", Enabled: true, ApplyFrom: &epoch,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	joined := strings.Join(llm.prompts, "\n")
	if !strings.Contains(joined, "untrusted data") {
		t.Error("the system prompt does not tell the model the email is data")
	}
	if strings.Count(joined, "x") > forwardLLMBodyChars+100 {
		t.Errorf("the body was not bounded: %d characters sent", strings.Count(joined, "x"))
	}
	if v := f.verdict(t, id, rule); v == nil || v.Status != "skipped" || *v.Reason != "not a tax document" {
		t.Fatalf("verdict = %+v", v)
	}
}

// A model that keeps answering nonsense is retried a bounded number of times.
func TestAIRuleUnusableAnswersAreRetriedThenGivenUp(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{})
	llm := &scriptedLLM{answers: []string{"no idea"}}
	f.svc.LLM = llm
	rule := uuid.New()
	if err := f.repo.CreateForwardRule(context.Background(), driven.ForwardRuleRow{
		ID: rule, UserID: f.userID, AccountID: f.accountID, Name: "tax", Mode: ForwardModeLLM,
		ConditionJSON: `{"prompt":"tax documents"}`, ForwardTo: "books@example.com", Enabled: true, ApplyFrom: &epoch,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxForwardAttempts+2; i++ {
		f.run(t)
	}
	if llm.calls != maxForwardAttempts {
		t.Fatalf("model calls = %d, want %d", llm.calls, maxForwardAttempts)
	}
	if v := f.verdict(t, f.messageID, rule); v == nil || v.Pending || v.Status != "failed" {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestPreviewCountsWhatARuleWouldReach(t *testing.T) {
	f := newForwardFixture(t, &fakeMailbox{})
	f.message(t, "Receipt", time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC), "finance")
	f.message(t, "Hello", time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC), "personal")
	p, err := f.svc.Preview(context.Background(), f.userID, f.accountID, ForwardModeLogic,
		json.RawMessage(`{"all":[{"field":"category_slug","op":"equals","value":"finance"}]}`), epoch)
	if err != nil {
		t.Fatal(err)
	}
	if p.InScope != 3 || p.Matched == nil || *p.Matched != 2 || len(p.Samples) != 2 || p.Capped {
		t.Fatalf("preview = in scope %d, matched %v, samples %d, capped %v", p.InScope, p.Matched, len(p.Samples), p.Capped)
	}
	ai, err := f.svc.Preview(context.Background(), f.userID, f.accountID, ForwardModeLLM, json.RawMessage(`{"prompt":"tax"}`), epoch)
	if err != nil || ai.InScope != 3 || ai.Matched != nil {
		t.Fatalf("AI preview = %+v, %v; want a scope count and no match count", ai, err)
	}
}
