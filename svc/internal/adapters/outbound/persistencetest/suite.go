package persistencetest

import (
	"context"
	"database/sql"
	"fmt"
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
	// Counter is optional. When set, the suite asserts that hot read paths stay
	// set-based; when nil those assertions are skipped.
	Counter *QueryCounter
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

	t.Run("triage_effective_assignment_parity", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC01", "Riverside")

		// Every permutation of override / thread assignment that Wave 1 §7 defines.
		cases := []struct {
			name         string
			conversation string
			thread       *string // thread project, "" means row with NULL project
			override     *string // override project, "" means row with NULL project
			wantQueued   bool
			wantStatus   string
			threadStatus string
			ovrStatus    string
		}{
			{name: "bare", conversation: "c-bare", wantQueued: true, wantStatus: "unassigned"},
			{name: "no_conversation", conversation: "", wantQueued: true, wantStatus: "unassigned"},
			{name: "thread_committed", conversation: "c-tc", thread: ptrStr(projectID.String()), threadStatus: "committed", wantQueued: false},
			{name: "thread_provisional", conversation: "c-tp", thread: ptrStr(projectID.String()), threadStatus: "provisional", wantQueued: true, wantStatus: "provisional"},
			{name: "override_committed", conversation: "c-oc", override: ptrStr(projectID.String()), ovrStatus: "committed", wantQueued: false},
			{name: "override_provisional", conversation: "c-op", override: ptrStr(projectID.String()), ovrStatus: "provisional", wantQueued: true, wantStatus: "provisional"},
			// An override that clears the project wins over an assigned thread.
			{name: "override_null_over_committed_thread", conversation: "c-onc", thread: ptrStr(projectID.String()), threadStatus: "committed", override: ptrStr(""), ovrStatus: "committed", wantQueued: true, wantStatus: "unassigned"},
		}

		ids := make(map[string]uuid.UUID, len(cases))
		for i, tc := range cases {
			msgID := insertMsg(t, h.Repo, accountID, tc.name, tc.conversation, "body", now.Add(-time.Duration(i)*time.Minute))
			ids[tc.name] = msgID
			if tc.thread != nil {
				row := driven.AssignmentRow{
					ID: uuid.New(), OrganisationID: orgID, AccountID: accountID,
					ConversationID: tc.conversation, Status: tc.threadStatus,
					Reason: "seed", Source: string(domainprojects.SourceRule), CreatedAt: now, UpdatedAt: now,
				}
				if *tc.thread != "" {
					pid := uuid.MustParse(*tc.thread)
					row.ProjectID = &pid
				}
				if err := h.Repo.UpsertThreadAssignment(ctx, row); err != nil {
					t.Fatal(err)
				}
			}
			if tc.override != nil {
				mid := msgID
				row := driven.AssignmentRow{
					OrganisationID: orgID, AccountID: accountID, MessageID: &mid,
					Status: tc.ovrStatus, Reason: "seed", Source: string(domainprojects.SourceUser),
					CreatedAt: now, UpdatedAt: now,
				}
				if *tc.override != "" {
					pid := uuid.MustParse(*tc.override)
					row.ProjectID = &pid
				}
				if err := h.Repo.UpsertMessageOverride(ctx, row); err != nil {
					t.Fatal(err)
				}
			}
		}

		items, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		queued := map[uuid.UUID]driven.UnassignedItem{}
		for _, it := range items {
			if it.MessageID != nil {
				queued[*it.MessageID] = it
			}
		}

		for _, tc := range cases {
			msgID := ids[tc.name]
			got, inQueue := queued[msgID]
			if inQueue != tc.wantQueued {
				t.Errorf("%s: queued = %v, want %v", tc.name, inQueue, tc.wantQueued)
				continue
			}
			if !tc.wantQueued {
				continue
			}
			if got.Status != tc.wantStatus {
				t.Errorf("%s: status = %q, want %q", tc.name, got.Status, tc.wantStatus)
			}
			// The list query and the single-message resolver must agree.
			eff, err := h.Repo.EffectiveAssignment(ctx, userID, msgID)
			if err != nil {
				t.Fatal(err)
			}
			if !samePtrUUID(eff.ProjectID, got.ProjectID) {
				t.Errorf("%s: list project=%v, EffectiveAssignment project=%v", tc.name, got.ProjectID, eff.ProjectID)
			}
		}
	})

	t.Run("triage_thread_dedupe", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, _, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)

		// One conversation with four messages collapses to a single decision.
		for i := 0; i < 4; i++ {
			insertMsg(t, h.Repo, accountID, "thread msg", "conv-1", "body", now.Add(-time.Duration(i)*time.Minute))
		}
		// Messages with no conversation are each their own decision.
		insertMsg(t, h.Repo, accountID, "loose a", "", "body", now.Add(-10*time.Minute))
		insertMsg(t, h.Repo, accountID, "loose b", "", "body", now.Add(-11*time.Minute))

		items, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 3 {
			t.Fatalf("want 3 queue rows (1 thread + 2 loose), got %d", len(items))
		}
		var threadRow *driven.UnassignedItem
		for i := range items {
			if items[i].ConversationID != nil && *items[i].ConversationID == "conv-1" {
				threadRow = &items[i]
			}
		}
		if threadRow == nil {
			t.Fatal("thread row missing")
		}
		if threadRow.ThreadCount != 4 {
			t.Errorf("thread_count = %d, want 4", threadRow.ThreadCount)
		}
		for _, it := range items {
			if it.ConversationID == nil || *it.ConversationID == "" {
				if it.ThreadCount != 1 {
					t.Errorf("loose message thread_count = %d, want 1", it.ThreadCount)
				}
			}
		}

		// Counts are over decisions, so the badge matches the queue length.
		sum, err := h.Repo.CountUnassignedSummary(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		if sum.Unassigned+sum.Provisional != len(items) {
			t.Errorf("summary total = %d, queue length = %d", sum.Unassigned+sum.Provisional, len(items))
		}
	})

	t.Run("triage_overridden_message_is_its_own_row", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC02", "Hollow")

		a := insertMsg(t, h.Repo, accountID, "m a", "conv-x", "body", now)
		insertMsg(t, h.Repo, accountID, "m b", "conv-x", "body", now.Add(-time.Minute))

		// Thread is provisional, but one message is individually cleared.
		if err := h.Repo.UpsertThreadAssignment(ctx, driven.AssignmentRow{
			ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, ConversationID: "conv-x",
			ProjectID: &projectID, Status: "provisional", Reason: "seed",
			Source: string(domainprojects.SourceRule), CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		mid := a
		if err := h.Repo.UpsertMessageOverride(ctx, driven.AssignmentRow{
			OrganisationID: orgID, AccountID: accountID, MessageID: &mid,
			Status: "committed", Reason: "seed", Source: string(domainprojects.SourceUser),
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		items, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 {
			t.Fatalf("want 2 rows (cleared message + rest of thread), got %d", len(items))
		}
		for _, it := range items {
			if it.MessageID != nil && *it.MessageID == a {
				if it.Status != "unassigned" || it.ProjectID != nil {
					t.Errorf("overridden message: status=%q project=%v, want unassigned/nil", it.Status, it.ProjectID)
				}
				if it.ThreadCount != 1 {
					t.Errorf("overridden message thread_count = %d, want 1", it.ThreadCount)
				}
			}
		}
	})

	t.Run("triage_status_filter_and_pagination", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC03", "Quarry")

		// 5 provisional threads and 5 unassigned threads, interleaved in time.
		for i := 0; i < 5; i++ {
			conv := fmt.Sprintf("prov-%d", i)
			insertMsg(t, h.Repo, accountID, conv, conv, "body", now.Add(-time.Duration(i*2)*time.Minute))
			if err := h.Repo.UpsertThreadAssignment(ctx, driven.AssignmentRow{
				ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, ConversationID: conv,
				ProjectID: &projectID, Status: "provisional", Reason: "seed",
				Source: string(domainprojects.SourceRule), CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
			plain := fmt.Sprintf("plain-%d", i)
			insertMsg(t, h.Repo, accountID, plain, plain, "body", now.Add(-time.Duration(i*2+1)*time.Minute))
		}

		all, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		if len(all) != 10 {
			t.Fatalf("want 10 rows, got %d", len(all))
		}
		prov, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "provisional", Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		if len(prov) != 5 {
			t.Fatalf("want 5 provisional rows, got %d", len(prov))
		}
		for _, it := range prov {
			if it.Status != "provisional" {
				t.Fatalf("status filter leaked %q", it.Status)
			}
		}

		// Pagination must be a window over the filtered set, not over raw candidates.
		page1, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 4, Offset: 0})
		if err != nil {
			t.Fatal(err)
		}
		page2, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 4, Offset: 4})
		if err != nil {
			t.Fatal(err)
		}
		page3, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 4, Offset: 8})
		if err != nil {
			t.Fatal(err)
		}
		if len(page1) != 4 || len(page2) != 4 || len(page3) != 2 {
			t.Fatalf("page sizes = %d/%d/%d, want 4/4/2", len(page1), len(page2), len(page3))
		}
		seen := map[uuid.UUID]bool{}
		for _, pg := range [][]driven.UnassignedItem{page1, page2, page3} {
			for _, it := range pg {
				if it.MessageID == nil {
					continue
				}
				if seen[*it.MessageID] {
					t.Fatalf("message %s returned on more than one page", it.MessageID)
				}
				seen[*it.MessageID] = true
			}
		}
		if len(seen) != 10 {
			t.Fatalf("paged through %d distinct rows, want 10", len(seen))
		}

		sum, err := h.Repo.CountUnassignedSummary(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		if sum.Provisional != 5 || sum.Unassigned != 5 {
			t.Fatalf("summary = %+v, want 5 provisional / 5 unassigned", sum)
		}
	})

	t.Run("triage_read_path_is_set_based", func(t *testing.T) {
		h := factory(t)
		if h.Counter == nil {
			t.Skip("handle has no query counter")
		}
		ctx := context.Background()
		now := time.Now().UTC()
		userID, _, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		for i := 0; i < 60; i++ {
			insertMsg(t, h.Repo, accountID, "subject", fmt.Sprintf("conv-%d", i), "body", now.Add(-time.Duration(i)*time.Minute))
		}

		// Both calls resolve the home org first, then run one statement. The
		// pre-rewrite implementation issued two to three per candidate message.
		h.Counter.Reset()
		if _, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 50}); err != nil {
			t.Fatal(err)
		}
		if n := h.Counter.Count(); n > 3 {
			t.Errorf("ListUnassigned issued %d statements over 60 messages, want <= 3", n)
		}

		h.Counter.Reset()
		if _, err := h.Repo.CountUnassignedSummary(ctx, userID); err != nil {
			t.Fatal(err)
		}
		if n := h.Counter.Count(); n > 3 {
			t.Errorf("CountUnassignedSummary issued %d statements over 60 messages, want <= 3", n)
		}

		h.Counter.Reset()
		if _, err := h.Repo.ListMessagesNeedingAssign(ctx, userID, accountID, 500); err != nil {
			t.Fatal(err)
		}
		if n := h.Counter.Count(); n > 3 {
			t.Errorf("ListMessagesNeedingAssign issued %d statements over 60 messages, want <= 3", n)
		}
	})

	t.Run("triage_messages_needing_assign_excludes_assigned", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC04", "Bridge")

		free := insertMsg(t, h.Repo, accountID, "free", "conv-free", "body", now)
		threaded := insertMsg(t, h.Repo, accountID, "threaded", "conv-assigned", "body", now.Add(-time.Minute))
		overridden := insertMsg(t, h.Repo, accountID, "overridden", "conv-ovr", "body", now.Add(-2*time.Minute))

		if err := h.Repo.UpsertThreadAssignment(ctx, driven.AssignmentRow{
			ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, ConversationID: "conv-assigned",
			ProjectID: &projectID, Status: "committed", Reason: "seed",
			Source: string(domainprojects.SourceRule), CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		mid := overridden
		if err := h.Repo.UpsertMessageOverride(ctx, driven.AssignmentRow{
			OrganisationID: orgID, AccountID: accountID, MessageID: &mid,
			Status: "committed", Reason: "seed", Source: string(domainprojects.SourceUser),
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		msgs, err := h.Repo.ListMessagesNeedingAssign(ctx, userID, accountID, 500)
		if err != nil {
			t.Fatal(err)
		}
		if len(msgs) != 1 || msgs[0].ID != free {
			got := make([]string, 0, len(msgs))
			for _, m := range msgs {
				got = append(got, m.Subject)
			}
			t.Fatalf("want only the unassigned message, got %v", got)
		}
		_ = threaded
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
	// This helper writes straight to the handle rather than through the repository,
	// so it does not get the repository's placeholder rewriting. SQLite wants `?`
	// and Postgres wants `$n`; try both before treating an error as real.
	stmts := []string{
		`INSERT INTO job_runs (id, account_id, job_type, trigger_kind, status, started_at, finished_at, error_message, meta_json)
		 VALUES (?, ?, 'summarize', 'api', 'success', ?, ?, NULL, '{}')`,
		`INSERT INTO job_runs (id, account_id, job_type, trigger_kind, status, started_at, finished_at, error_message, meta_json)
		 VALUES ($1, $2, 'summarize', 'api', 'success', $3, $4, NULL, '{}')`,
	}
	ts := now.Format(time.RFC3339Nano)
	var lastErr error
	for _, q := range stmts {
		_, err := db.Exec(q, runID.String(), accountID.String(), ts, ts)
		if err == nil {
			return
		}
		lastErr = err
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "no such table") || strings.Contains(msg, "does not exist") {
			return
		}
	}
	t.Fatal(lastErr)
}

func ptrStr(v string) *string { return &v }

func samePtrUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func createProject(t *testing.T, repo Repository, orgID, userID uuid.UUID, code, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	now := time.Now().UTC()
	if err := repo.CreateProject(context.Background(), driven.ProjectRow{
		ID: id, OrganisationID: orgID, Name: name, Code: code, CreatedAt: now, UpdatedAt: now,
	}, driven.ProjectMemberRow{
		ID: uuid.New(), ProjectID: id, UserID: userID, Role: "owner", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}
