package persistencetest

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

type Repository interface {
	driven.UserRepository
	driven.AccountRepository
	driven.MessageRepository
	driven.OAuthStateRepository
	driven.AuthSessionRepository
	driven.OrganisationRepository
	driven.ContactRepository
	driven.ProjectRepository
	driven.ManualItemRepository
	driven.TimelineRepository
	driven.AssignmentRepository
	driven.SummaryRepository
	driven.ScheduleRepository
	driven.ForwardRepository
	MarkScheduleExecutedIfDue(ctx context.Context, id uuid.UUID, scheduledFor, lastRunAt, nextRunAt time.Time) (bool, error)
}

type Handle struct {
	DB   *sql.DB
	Repo Repository
}

type Factory func(t *testing.T) Handle

func Run(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("auth_session_consume_once", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID := uuid.New()
		orgID, err := h.Repo.CreateUserWithHomeOrg(ctx, userID, "alice@example.com", nil, now, "password", userID.String(), "alice@example.com")
		if err != nil {
			t.Fatal(err)
		}
		if orgID == uuid.Nil {
			t.Fatal("expected home org")
		}
		sessionID := uuid.New()
		if err := h.Repo.InsertAuthSession(ctx, sessionID, userID, "tok", now, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		gotUser, ok, err := h.Repo.ConsumeAuthSession(ctx, "tok")
		if err != nil {
			t.Fatal(err)
		}
		if !ok || gotUser != userID {
			t.Fatalf("consume returned ok=%v user=%s", ok, gotUser)
		}
		_, ok, err = h.Repo.ConsumeAuthSession(ctx, "tok")
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatal("expected second consume to be empty")
		}
	})

	t.Run("seed_categories_present", func(t *testing.T) {
		h := factory(t)
		cats, err := h.Repo.ListCategoryDefinitions(context.Background(), uuid.MustParse("a0000001-0000-4000-8000-000000000001"))
		if err != nil {
			t.Fatal(err)
		}
		if len(cats) < 6 {
			t.Fatalf("expected seeded categories, got %d", len(cats))
		}
	})

	t.Run("contacts_merge_and_isolation", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		u1 := uuid.New()
		org1, err := h.Repo.CreateUserWithHomeOrg(ctx, u1, "u1@example.com", nil, now, "password", u1.String(), "u1@example.com")
		if err != nil {
			t.Fatal(err)
		}
		u2 := uuid.New()
		org2, err := h.Repo.CreateUserWithHomeOrg(ctx, u2, "u2@example.com", nil, now, "password", u2.String(), "u2@example.com")
		if err != nil {
			t.Fatal(err)
		}
		c1, err := h.Repo.ResolveEmailContact(ctx, org1, "sarah@acme.com", "Sarah", now)
		if err != nil {
			t.Fatal(err)
		}
		c1b, err := h.Repo.ResolveEmailContact(ctx, org1, "sarah@acme.com", "Sarah Other", now)
		if err != nil {
			t.Fatal(err)
		}
		if c1 != c1b {
			t.Fatal("same org should reuse contact")
		}
		c2, err := h.Repo.ResolveEmailContact(ctx, org2, "sarah@acme.com", "Sarah", now)
		if err != nil {
			t.Fatal(err)
		}
		if c1 == c2 {
			t.Fatal("different orgs should not reuse contact")
		}
		other, err := h.Repo.ResolveEmailContact(ctx, org1, "other@acme.com", "Sarah", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.MergeContacts(ctx, org1, c1, other, now); err != nil {
			t.Fatal(err)
		}
		list, err := h.Repo.ListContacts(ctx, org1, driven.ContactListFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].ID != c1 {
			t.Fatalf("unexpected contact list: %+v", list)
		}
	})

	t.Run("project_manual_timeline_flow", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		svc := &appprojects.Service{
			Users: h.Repo, Projects: h.Repo, Assignments: h.Repo, Manuals: h.Repo, Timeline: h.Repo, Contacts: h.Repo, Messages: h.Repo,
		}
		now := time.Now().UTC()
		userID := uuid.New()
		_, _, accountID := seedUserAccount(t, h.Repo, userID, now)
		project, err := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
		if err != nil {
			t.Fatal(err)
		}
		early := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
		late := time.Date(2026, 3, 2, 15, 0, 0, 0, time.UTC)
		conv := "conv-tl"
		msgID := insertMsg(t, h.Repo, accountID, "Outlook: pump", conv, "pump sizing", early)
		if _, err := svc.AssignMessage(ctx, userID, msgID, appprojects.AssignInput{
			ProjectID: &project.ID, Scope: domainprojects.ScopeThread, Status: domainprojects.StatusCommitted,
		}); err != nil {
			t.Fatal(err)
		}
		manual, err := svc.CreateManualItem(ctx, userID, appprojects.CreateManualInput{
			Channel: "teams", OccurredAt: late, Title: "Teams note", BodyText: "Consider 90 kW", ProjectID: &project.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		items, err := svc.GetTimeline(ctx, userID, project.ID, driven.TimelineFilter{Source: "all", Limit: 20})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) < 2 {
			t.Fatalf("expected 2 timeline items, got %d", len(items))
		}
		if items[0].Source != "manual" || items[0].ManualItemID == nil || *items[0].ManualItemID != manual.ID {
			t.Fatalf("unexpected first timeline item: %+v", items[0])
		}
		if items[1].Source != "mail" || items[1].MessageID == nil || *items[1].MessageID != msgID {
			t.Fatalf("unexpected second timeline item: %+v", items[1])
		}
	})

	t.Run("summary_forward_and_schedule_roundtrip", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
		accountID := uuid.New()
		if err := h.Repo.InsertAccount(ctx, driven.AccountRow{
			UserID: userID, ID: accountID, Label: "Work", Provider: "m365", MsAccountKind: "work", PrimaryEmail: "work@example.com", ConnectionStatus: "connected",
		}, []byte("cipher")); err != nil {
			t.Fatal(err)
		}
		msgID := uuid.New()
		body := "Please settle your invoice."
		if err := h.Repo.UpsertMessage(ctx, driven.MessageRow{
			ID: msgID, AccountID: accountID, ProviderMessageID: "provider-1", ReceivedAt: now, Subject: "Invoice", FromJSON: `{"name":"Stripe","address":"billing@example.com"}`,
			BodyText: &body, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		def, err := h.Repo.GetCategoryDefinitionBySlug(ctx, userID, "important")
		if err != nil || def == nil {
			t.Fatalf("important category missing: %v", err)
		}
		runID := uuid.New()
		ensureLegacyJobRunIfPresent(t, h.DB, runID, accountID, now)
		if err := h.Repo.UpsertMessageCategory(ctx, driven.MessageCategoryRow{
			ID: uuid.New(), MessageID: msgID, AccountID: accountID, CategoryID: def.ID, Source: "llm", Confidence: ptrFloat(0.9), RunID: runID, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.UpsertSummarySettings(ctx, driven.SummarySettingsRow{
			UserID: userID, IncludeCategorySlugs: nil, ExcludeCategorySlugs: []string{"spam"}, ChunkSize: 12, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		snapshot := driven.SummarySnapshotRow{
			ID: uuid.New(), UserID: userID, AccountID: &accountID, RunID: runID, WindowStart: now.Add(-time.Hour), WindowEnd: now, GeneralSummary: "ok", CreatedAt: now,
		}
		if err := h.Repo.InsertSummarySnapshot(ctx, snapshot); err != nil {
			t.Fatal(err)
		}
		snapshots, err := h.Repo.ListSummarySnapshots(ctx, userID, &accountID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshots) == 0 {
			t.Fatal("expected summary snapshot")
		}
		actionID := uuid.New()
		if err := h.Repo.InsertActionItems(ctx, []driven.ActionItemRow{{
			ID: actionID, UserID: userID, AccountID: accountID, MessageID: msgID, RunID: runID, Text: "Reply", Status: "open", CreatedAt: now, UpdatedAt: now,
		}}); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.InsertFYI(ctx, []driven.FYIRow{{
			ID: uuid.New(), UserID: userID, AccountID: accountID, MessageID: msgID, RunID: runID, Text: "FYI", CreatedAt: now,
		}}); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.ReplaceForwardAllowlist(ctx, userID, []string{"bills@example.com"}); err != nil {
			t.Fatal(err)
		}
		ruleID := uuid.New()
		if err := h.Repo.CreateForwardRule(ctx, driven.ForwardRuleRow{
			ID: ruleID, UserID: userID, AccountID: accountID, Name: "Forward", Mode: "logic", ConditionJSON: `{"all":[]}`, ForwardTo: "bills@example.com", Enabled: true, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.InsertForwardAudit(ctx, driven.ForwardAuditRow{
			ID: uuid.New(), UserID: userID, AccountID: accountID, MessageID: msgID, RuleID: ruleID, RunID: runID, Status: "forwarded", CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		audit, err := h.Repo.ListForwardAuditByRun(ctx, userID, runID)
		if err != nil {
			t.Fatal(err)
		}
		if len(audit) != 1 || audit[0].Status != "forwarded" {
			t.Fatalf("expected forward audit row, got %+v", audit)
		}
		if err := h.Repo.InsertDraftSuggestions(ctx, []driven.DraftSuggestionRow{{
			ID: uuid.New(), UserID: userID, AccountID: accountID, MessageID: msgID, ActionItemID: actionID, RunID: runID, Subject: "First", Body: "Body", Model: "test", CreatedAt: now, UpdatedAt: ptrTime(now),
		}}); err != nil {
			t.Fatal(err)
		}
		drafts, err := h.Repo.ListDraftSuggestions(ctx, userID, &accountID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(drafts) != 1 || drafts[0].Subject != "First" {
			t.Fatalf("expected draft suggestion row, got %+v", drafts)
		}
		scheduleID := uuid.New()
		if err := h.Repo.ReplaceSchedulesByUser(ctx, userID, []driven.ScheduleChainRow{{
			ID: scheduleID, UserID: userID, Name: "Nightly", AccountID: &accountID, Jobs: []string{"sync", "summarize"}, IntervalMinutes: 60, Enabled: true, NextRunAt: now, CreatedAt: now, UpdatedAt: now,
		}}); err != nil {
			t.Fatal(err)
		}
		due, err := h.Repo.ListDueSchedules(ctx, now.Add(time.Minute), 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(due) != 1 {
			t.Fatalf("expected due schedule, got %d", len(due))
		}
		ok, err := h.Repo.MarkScheduleExecutedIfDue(ctx, scheduleID, now, now, now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("expected schedule CAS success")
		}
		ok, err = h.Repo.MarkScheduleExecutedIfDue(ctx, scheduleID, now, now, now.Add(2*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatal("expected stale schedule CAS failure")
		}
	})
}

func seedUserAccount(t *testing.T, repo Repository, userID uuid.UUID, now time.Time) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	orgID, err := repo.CreateUserWithHomeOrg(ctx, userID, userID.String()+"@ex.com", nil, now, "password", userID.String(), userID.String()+"@ex.com")
	if err != nil {
		t.Fatal(err)
	}
	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365", MsAccountKind: "work", PrimaryEmail: userID.String() + "@ex.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	return userID, orgID, accountID
}

func insertMsg(t *testing.T, repo Repository, accountID uuid.UUID, subject, conv, body string, occurredAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	convPtr := &conv
	bodyPtr := &body
	if err := repo.UpsertMessage(context.Background(), driven.MessageRow{
		ID: id, AccountID: accountID, ProviderMessageID: id.String(), Subject: subject, FromJSON: `{"address":"a@b.com"}`,
		BodyText: bodyPtr, ConversationID: convPtr, ReceivedAt: occurredAt, CreatedAt: occurredAt, UpdatedAt: occurredAt,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func ptrFloat(v float64) *float64    { return &v }
func ptrTime(v time.Time) *time.Time { return &v }

func ensureLegacyJobRunIfPresent(t *testing.T, db *sql.DB, runID, accountID uuid.UUID, now time.Time) {
	t.Helper()
	if db == nil {
		return
	}
	_, err := db.Exec(`
		INSERT INTO job_runs (id, account_id, job_type, trigger_kind, status, started_at, finished_at, error_message, meta_json)
		VALUES (?, ?, 'summarize', 'api', 'success', ?, ?, NULL, '{}')
	`, runID.String(), accountID.String(), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err == nil {
		return
	}
	if strings.Contains(strings.ToLower(err.Error()), "no such table") || strings.Contains(strings.ToLower(err.Error()), "does not exist") {
		return
	}
	t.Fatal(err)
}
