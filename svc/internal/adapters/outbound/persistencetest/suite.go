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
	driven.IssueRepository
	driven.FactRepository
	driven.DecisionRepository
	driven.ContradictionRepository
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
		// The merged contact owns both addresses; either is a fair label.
		if e := list[0].PrimaryEmail; e != "sarah@acme.com" && e != "other@acme.com" {
			t.Fatalf("merged contact primary email = %q", e)
		}
		// Search takes a different query and must fill it too.
		found, err := h.Repo.ListContacts(ctx, org2, driven.ContactListFilter{Query: "acme"})
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 1 || found[0].PrimaryEmail != "sarah@acme.com" {
			t.Fatalf("search result = %+v, want sarah@acme.com as the primary email", found)
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
		if _, err := h.Repo.ListMessagesNeedingAssign(ctx, userID, accountID, driven.AssignCandidateFilter{Limit: 500}); err != nil {
			t.Fatal(err)
		}
		if n := h.Counter.Count(); n > 3 {
			t.Errorf("ListMessagesNeedingAssign issued %d statements over 60 messages, want <= 3", n)
		}
	})

	t.Run("triage_read_path_never_reads_bodies", func(t *testing.T) {
		h := factory(t)
		if h.Counter == nil {
			t.Skip("handle has no query counter")
		}
		ctx := context.Background()
		now := time.Now().UTC()
		userID, _, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		for i := 0; i < 5; i++ {
			insertMsg(t, h.Repo, accountID, "subject", fmt.Sprintf("conv-%d", i), "body", now)
		}

		// The queue renders subject, sender and timestamp. Pulling mail bodies
		// to decide an assignment status is what made this path so expensive,
		// and on DSQL it eats the 128 MiB query-memory budget.
		for name, call := range map[string]func() error{
			"ListUnassigned": func() error {
				_, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 50})
				return err
			},
			"CountUnassignedSummary": func() error {
				_, err := h.Repo.CountUnassignedSummary(ctx, userID)
				return err
			},
		} {
			h.Counter.Reset()
			if err := call(); err != nil {
				t.Fatal(err)
			}
			for _, stmt := range h.Counter.Statements() {
				if strings.Contains(strings.ToLower(stmt), "body_text") {
					t.Errorf("%s reads body_text:\n%s", name, stmt)
				}
			}
		}
	})

	runHomeOverviewTimestampTests(t, factory)
	runActivityFeedTests(t, factory)
	runAttentionTests(t, factory)
	runNotRelevantTests(t, factory)
	runExtractionDebounceTests(t, factory)

	t.Run("timeline_hydration_is_batched_and_correct", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC05", "Timeline")

		// A contact who participates in both a mail thread and a paste.
		contactID := uuid.New()
		if err := h.Repo.CreateContact(ctx, driven.ContactRow{
			ID: contactID, OrganisationID: orgID, DisplayName: "Dana Reed", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		const mailCount = 12
		messageIDs := make([]uuid.UUID, 0, mailCount)
		for i := 0; i < mailCount; i++ {
			conv := fmt.Sprintf("tl-conv-%d", i)
			msgID := insertMsg(t, h.Repo, accountID, "project mail", conv, "body", now.Add(-time.Duration(i)*time.Minute))
			messageIDs = append(messageIDs, msgID)
			if err := h.Repo.UpsertThreadAssignment(ctx, driven.AssignmentRow{
				ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, ConversationID: conv,
				ProjectID: &projectID, Status: "committed", Reason: "seed",
				Source: string(domainprojects.SourceUser), CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
			mid := msgID
			if err := h.Repo.UpsertParticipant(ctx, driven.CorrespondenceParticipantRow{
				ID: uuid.New(), OrganisationID: orgID, ContactID: contactID, Role: "to", MessageID: &mid,
			}); err != nil {
				t.Fatal(err)
			}
		}

		manualID := uuid.New()
		reason, source := "user_paste", string(domainprojects.SourceUser)
		if err := h.Repo.CreateManualItem(ctx, driven.ManualItemRow{
			ID: manualID, OrganisationID: orgID, Channel: "note", OccurredAt: now,
			Title: "Site note", BodyText: "pasted", ProjectID: &projectID,
			AssignmentStatus: "committed", AssignmentReason: &reason, AssignmentSource: &source,
			CreatedByUserID: userID, CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.UpsertParticipant(ctx, driven.CorrespondenceParticipantRow{
			ID: uuid.New(), OrganisationID: orgID, ContactID: contactID, Role: "to", ManualItemID: &manualID,
		}); err != nil {
			t.Fatal(err)
		}

		// One message and the paste belong to an issue.
		issueID := uuid.New()
		if err := h.Repo.CreateIssue(ctx, driven.IssueRow{
			ID: issueID, OrganisationID: orgID, ProjectID: projectID, Title: "Leak",
			Status: "open", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		linkedMsg := messageIDs[0]
		if err := h.Repo.AddIssueItem(ctx, driven.IssueItemRow{
			ID: uuid.New(), IssueID: issueID, MessageID: &linkedMsg, AddedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		linkedManual := manualID
		if err := h.Repo.AddIssueItem(ctx, driven.IssueItemRow{
			ID: uuid.New(), IssueID: issueID, ManualItemID: &linkedManual, AddedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		if h.Counter != nil {
			h.Counter.Reset()
		}
		items, err := h.Repo.ListProjectTimeline(ctx, userID, orgID, projectID, driven.TimelineFilter{Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != mailCount+1 {
			t.Fatalf("timeline returned %d items, want %d", len(items), mailCount+1)
		}

		// Hydration is batched, not per row. Before this it cost two extra
		// statements for every item on the timeline.
		if h.Counter != nil {
			if n := h.Counter.Count(); n > 12 {
				t.Errorf("ListProjectTimeline issued %d statements for %d items, want a bounded handful", n, len(items))
			}
		}

		var withIssue, withContact int
		for _, it := range items {
			if len(it.Contacts) > 0 {
				withContact++
				if it.Contacts[0].ID != contactID || it.Contacts[0].DisplayName != "Dana Reed" {
					t.Errorf("contact hydrated wrongly: %+v", it.Contacts[0])
				}
			}
			if it.IssueID != nil {
				withIssue++
				if *it.IssueID != issueID {
					t.Errorf("issue id = %s, want %s", it.IssueID, issueID)
				}
			}
		}
		if withContact != mailCount+1 {
			t.Errorf("%d items carry contacts, want %d", withContact, mailCount+1)
		}
		if withIssue != 2 {
			t.Errorf("%d items carry an issue link, want 2", withIssue)
		}

		// The unassigned-to-issue filter depends on hydration having happened.
		unlinked, err := h.Repo.ListProjectTimeline(ctx, userID, orgID, projectID,
			driven.TimelineFilter{Limit: 100, UnassignedToIssue: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(unlinked) != mailCount+1-2 {
			t.Fatalf("unassigned-to-issue returned %d, want %d", len(unlinked), mailCount+1-2)
		}
		for _, it := range unlinked {
			if it.IssueID != nil {
				t.Error("unassigned-to-issue returned an item already on an issue")
			}
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

		msgs, err := h.Repo.ListMessagesNeedingAssign(ctx, userID, accountID, driven.AssignCandidateFilter{Limit: 500})
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

// runHomeOverviewTimestampTests covers the two columns added for the Home
// activity feed, whose whole value is dating events correctly.
func runHomeOverviewTimestampTests(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("fact_activation_is_dated_when_confirmed", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, _ := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC10", "Timestamps")

		factID := uuid.New()
		if err := h.Repo.CreateFact(ctx, driven.FactRow{
			ID: factID, OrganisationID: orgID, ProjectID: projectID,
			SubjectKey: "pump.duty", Label: "Pump duty", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		// Proposed a week ago, with no activation time.
		proposed := now.Add(-7 * 24 * time.Hour)
		verID := uuid.New()
		if err := h.Repo.CreateFactVersion(ctx, driven.FactVersionRow{
			ID: verID, FactID: factID, Status: "proposed", ValueJSON: `{"amount":90}`,
			ValueText: "90 kW", Source: "llm", CreatedAt: proposed,
		}); err != nil {
			t.Fatal(err)
		}
		got, err := h.Repo.GetFactVersion(ctx, orgID, verID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ActivatedAt != nil {
			t.Fatalf("proposed version should have no activation time, got %v", got.ActivatedAt)
		}

		// Confirmed today.
		got.Status = "active"
		activated := now
		got.ActivatedAt = &activated
		if err := h.Repo.UpdateFactVersion(ctx, *got); err != nil {
			t.Fatal(err)
		}
		reloaded, err := h.Repo.GetFactVersion(ctx, orgID, verID)
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.ActivatedAt == nil {
			t.Fatal("expected an activation time after confirm")
		}
		if !reloaded.ActivatedAt.Truncate(time.Second).Equal(activated.Truncate(time.Second)) {
			t.Errorf("activated_at = %v, want %v", reloaded.ActivatedAt, activated)
		}
		// The point of the column: activation is days after creation.
		if !reloaded.ActivatedAt.After(reloaded.CreatedAt) {
			t.Errorf("activated_at %v should be after created_at %v", reloaded.ActivatedAt, reloaded.CreatedAt)
		}
	})

	t.Run("issue_resolution_is_dated_and_cleared_on_reopen", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, _ := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC11", "Timestamps")

		issueID := uuid.New()
		if err := h.Repo.CreateIssue(ctx, driven.IssueRow{
			ID: issueID, OrganisationID: orgID, ProjectID: projectID, Title: "Leak",
			Status: "open", CreatedAt: now.Add(-48 * time.Hour), UpdatedAt: now.Add(-48 * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
		open, err := h.Repo.GetIssue(ctx, orgID, issueID)
		if err != nil {
			t.Fatal(err)
		}
		if open.ResolvedAt != nil {
			t.Fatalf("open issue should have no resolution time, got %v", open.ResolvedAt)
		}

		resolved := now
		open.Status = "resolved"
		open.ResolvedAt = &resolved
		open.UpdatedAt = resolved
		if err := h.Repo.UpdateIssue(ctx, *open); err != nil {
			t.Fatal(err)
		}
		got, err := h.Repo.GetIssue(ctx, orgID, issueID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ResolvedAt == nil {
			t.Fatal("expected a resolution time")
		}

		// Editing the issue afterwards must not move the resolution time.
		got.Title = "Leak (renamed)"
		got.UpdatedAt = now.Add(time.Hour)
		if err := h.Repo.UpdateIssue(ctx, *got); err != nil {
			t.Fatal(err)
		}
		after, err := h.Repo.GetIssue(ctx, orgID, issueID)
		if err != nil {
			t.Fatal(err)
		}
		if after.ResolvedAt == nil || !after.ResolvedAt.Truncate(time.Second).Equal(resolved.Truncate(time.Second)) {
			t.Errorf("resolved_at moved on edit: %v, want %v", after.ResolvedAt, resolved)
		}
		if !after.UpdatedAt.After(*after.ResolvedAt) {
			t.Error("updated_at should have moved past resolved_at, which is why updated_at cannot date the resolution")
		}
	})

	t.Run("issue_source_round_trips_and_defaults_empty", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, _ := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC12", "Provenance")

		extracted := uuid.New()
		if err := h.Repo.CreateIssue(ctx, driven.IssueRow{
			ID: extracted, OrganisationID: orgID, ProjectID: projectID, Title: "Seal leak",
			Status: "open", Source: "llm", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		// A row written before the column existed reads as empty, not as an
		// error, so the API can default it to human.
		legacy := uuid.New()
		if err := h.Repo.CreateIssue(ctx, driven.IssueRow{
			ID: legacy, OrganisationID: orgID, ProjectID: projectID, Title: "Raised by hand",
			Status: "open", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		got, err := h.Repo.GetIssue(ctx, orgID, extracted)
		if err != nil {
			t.Fatal(err)
		}
		if got.Source != "llm" {
			t.Errorf("source = %q, want llm", got.Source)
		}
		old, err := h.Repo.GetIssue(ctx, orgID, legacy)
		if err != nil {
			t.Fatal(err)
		}
		if old.Source != "" {
			t.Errorf("source = %q, want empty", old.Source)
		}

		// Source survives an unrelated edit.
		got.Title = "Seal leak (renamed)"
		got.UpdatedAt = now.Add(time.Hour)
		if err := h.Repo.UpdateIssue(ctx, *got); err != nil {
			t.Fatal(err)
		}
		after, err := h.Repo.GetIssue(ctx, orgID, extracted)
		if err != nil {
			t.Fatal(err)
		}
		if after.Source != "llm" {
			t.Errorf("source = %q after edit, want llm", after.Source)
		}
	})

	t.Run("issue_item_counts_resolve_per_project_in_one_query", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, _ := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC13", "Counts")
		otherProjectID := createProject(t, h.Repo, orgID, userID, "DC14", "Other")

		withEvidence := uuid.New()
		withoutEvidence := uuid.New()
		elsewhere := uuid.New()
		for id, pid := range map[uuid.UUID]uuid.UUID{
			withEvidence: projectID, withoutEvidence: projectID, elsewhere: otherProjectID,
		} {
			if err := h.Repo.CreateIssue(ctx, driven.IssueRow{
				ID: id, OrganisationID: orgID, ProjectID: pid, Title: "Issue",
				Status: "open", CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 2; i++ {
			manualID := uuid.New()
			if err := h.Repo.CreateManualItem(ctx, driven.ManualItemRow{
				ID: manualID, OrganisationID: orgID, Channel: "note", OccurredAt: now,
				Title: "Note", BodyText: "text", ProjectID: &projectID,
				AssignmentStatus: "committed", CreatedByUserID: userID, CreatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
			if err := h.Repo.AddIssueItem(ctx, driven.IssueItemRow{
				ID: uuid.New(), IssueID: withEvidence, ManualItemID: &manualID, AddedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
		}

		counts, err := h.Repo.CountIssueItemsByProject(ctx, orgID, projectID)
		if err != nil {
			t.Fatal(err)
		}
		if counts[withEvidence] != 2 {
			t.Errorf("count = %d, want 2", counts[withEvidence])
		}
		// An issue with no evidence is absent rather than zero; callers read
		// the zero value from the map.
		if _, ok := counts[withoutEvidence]; ok {
			t.Error("an issue with no evidence should not appear in the counts")
		}
		// Scoped to the project, so another project's issues cannot inflate it.
		if _, ok := counts[elsewhere]; ok {
			t.Error("counts must not reach outside the project")
		}
	})
}

// runActivityFeedTests covers the cross-project Home feed.
func runActivityFeedTests(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("activity_shows_each_change_and_scopes_by_membership", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		mine := createProject(t, h.Repo, orgID, userID, "DC20", "Mine")

		// A project in the same org that the caller is NOT a member of.
		stranger := uuid.New()
		if _, err := h.Repo.CreateUserWithHomeOrg(ctx, stranger, "stranger-"+stranger.String()+"@ex.com", nil, now,
			"password", stranger.String(), "stranger-"+stranger.String()+"@ex.com"); err != nil {
			t.Fatal(err)
		}
		theirs := uuid.New()
		if err := h.Repo.CreateProject(ctx, driven.ProjectRow{
			ID: theirs, OrganisationID: orgID, Name: "Theirs", Code: "DC21",
			CreatedAt: now, UpdatedAt: now,
		}, driven.ProjectMemberRow{
			ID: uuid.New(), ProjectID: theirs, UserID: stranger, Role: "owner",
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		// A decision proposed, then accepted: two events from one row.
		decisionID := uuid.New()
		decided := now.Add(-1 * time.Hour)
		if err := h.Repo.CreateDecision(ctx, driven.DecisionRow{
			ID: decisionID, OrganisationID: orgID, ProjectID: mine,
			Statement: "Proceed with 90 kW", Status: "accepted", Source: "llm",
			DecidedAt: &decided, CreatedAt: now.Add(-6 * time.Hour), UpdatedAt: decided,
		}); err != nil {
			t.Fatal(err)
		}
		// The same shape on the project the caller cannot see.
		if err := h.Repo.CreateDecision(ctx, driven.DecisionRow{
			ID: uuid.New(), OrganisationID: orgID, ProjectID: theirs,
			Statement: "Invisible decision", Status: "accepted", Source: "user",
			DecidedAt: &decided, CreatedAt: now.Add(-6 * time.Hour), UpdatedAt: decided,
		}); err != nil {
			t.Fatal(err)
		}

		// An issue opened and resolved.
		issueID := uuid.New()
		resolved := now.Add(-30 * time.Minute)
		if err := h.Repo.CreateIssue(ctx, driven.IssueRow{
			ID: issueID, OrganisationID: orgID, ProjectID: mine, Title: "Leak",
			Status: "resolved", CreatedAt: now.Add(-5 * time.Hour),
			UpdatedAt: resolved, ResolvedAt: &resolved,
		}); err != nil {
			t.Fatal(err)
		}

		// A contradiction, still open.
		if err := h.Repo.CreateContradiction(ctx, driven.ContradictionRow{
			ID: uuid.New(), OrganisationID: orgID, ProjectID: mine, Status: "open",
			Summary: "Duty stated twice", CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}

		// Correspondence filed onto the project: must NOT appear in the feed.
		conv := "conv-activity"
		insertMsg(t, h.Repo, accountID, "filed mail", conv, "body", now)
		if err := h.Repo.UpsertThreadAssignment(ctx, driven.AssignmentRow{
			ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, ConversationID: conv,
			ProjectID: &mine, Status: "provisional", Reason: "llm", Source: "llm",
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		items, err := h.Repo.ListActivity(ctx, userID, orgID, driven.ActivityFilter{Limit: 50})
		if err != nil {
			t.Fatal(err)
		}

		byKind := map[string]int{}
		for _, it := range items {
			byKind[it.Kind]++
			if it.ProjectID != mine {
				t.Errorf("feed leaked a project the caller is not a member of: %s", it.ProjectCode)
			}
			if it.ProjectCode != "DC20" {
				t.Errorf("project code = %q, want DC20", it.ProjectCode)
			}
			if strings.Contains(strings.ToLower(it.Kind), "assign") {
				t.Errorf("provisional assignments must not appear: %+v", it)
			}
		}
		// One decision row yields both a proposed and an accepted event.
		for _, want := range []string{"decision_proposed", "decision_accepted", "issue_opened", "issue_resolved", "contradiction_opened"} {
			if byKind[want] == 0 {
				t.Errorf("missing %s; got %v", want, byKind)
			}
		}
		if byKind["contradiction_resolved"] != 0 {
			t.Errorf("open contradiction must not report a resolution event")
		}

		// Newest first.
		for i := 1; i < len(items); i++ {
			if items[i].OccurredAt.After(items[i-1].OccurredAt) {
				t.Fatalf("feed out of order at %d: %v after %v", i, items[i].OccurredAt, items[i-1].OccurredAt)
			}
		}
	})

	t.Run("activity_pages_without_repeats_or_gaps", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, _ := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC22", "Paging")

		// Several events share a timestamp, which is what makes a naive
		// timestamp-only cursor loop.
		shared := now.Add(-time.Hour)
		const total = 12
		for i := 0; i < total; i++ {
			at := shared
			if i%3 == 0 {
				at = now.Add(-time.Duration(i) * time.Minute)
			}
			if err := h.Repo.CreateIssue(ctx, driven.IssueRow{
				ID: uuid.New(), OrganisationID: orgID, ProjectID: projectID,
				Title: fmt.Sprintf("issue %d", i), Status: "open",
				CreatedAt: at, UpdatedAt: at,
			}); err != nil {
				t.Fatal(err)
			}
		}

		seen := map[string]int{}
		var before *time.Time
		var beforeID *uuid.UUID
		for page := 0; page < 10; page++ {
			got, err := h.Repo.ListActivity(ctx, userID, orgID, driven.ActivityFilter{
				Limit: 5, Before: before, BeforeID: beforeID,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) == 0 {
				break
			}
			for _, it := range got {
				seen[it.Kind+":"+it.RefID.String()]++
			}
			last := got[len(got)-1]
			at := last.OccurredAt
			id := last.RefID
			before = &at
			beforeID = &id
		}
		if len(seen) != total {
			t.Fatalf("paged through %d distinct events, want %d", len(seen), total)
		}
		for key, n := range seen {
			if n != 1 {
				t.Errorf("event %s returned %d times across pages", key, n)
			}
		}
	})

	t.Run("overview_counts_and_last_activity", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, _ := seedUserAccount(t, h.Repo, uuid.New(), now)
		busy := createProject(t, h.Repo, orgID, userID, "DC23", "Busy")
		createProject(t, h.Repo, orgID, userID, "DC24", "Quiet")

		if err := h.Repo.CreateContradiction(ctx, driven.ContradictionRow{
			ID: uuid.New(), OrganisationID: orgID, ProjectID: busy, Status: "open",
			Summary: "conflict", CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.CreateDecision(ctx, driven.DecisionRow{
			ID: uuid.New(), OrganisationID: orgID, ProjectID: busy,
			Statement: "Maybe", Status: "proposed", Source: "llm",
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		decided := now
		if err := h.Repo.CreateDecision(ctx, driven.DecisionRow{
			ID: uuid.New(), OrganisationID: orgID, ProjectID: busy,
			Statement: "Proceed with 90 kW", Status: "accepted", Source: "user",
			DecidedAt: &decided, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		counts, err := h.Repo.CountOverview(ctx, userID, orgID)
		if err != nil {
			t.Fatal(err)
		}
		if counts.OpenContradictions != 1 {
			t.Errorf("open contradictions = %d, want 1", counts.OpenContradictions)
		}
		if counts.ProposedDecisions != 1 {
			t.Errorf("proposed decisions = %d, want 1", counts.ProposedDecisions)
		}
		if counts.ActiveProjects != 2 {
			t.Errorf("active projects = %d, want 2", counts.ActiveProjects)
		}

		projects, err := h.Repo.ListOverviewProjects(ctx, userID, orgID, 8)
		if err != nil {
			t.Fatal(err)
		}
		if len(projects) != 2 {
			t.Fatalf("overview projects = %d, want 2", len(projects))
		}
		// The project with activity sorts first; the quiet one still appears,
		// with no activity time rather than being dropped.
		if projects[0].Code != "DC23" {
			t.Errorf("first project = %s, want DC23 (it has activity)", projects[0].Code)
		}
		if projects[0].LastActivityAt == nil {
			t.Error("busy project should report a last activity time")
		}
		// The teaser comes from the overview payload, so Home needs no
		// per-project request to render it.
		if projects[0].Teaser == "" {
			t.Error("busy project should carry a position teaser from its accepted decision or active fact")
		}
		if projects[1].Code != "DC24" || projects[1].LastActivityAt != nil {
			t.Errorf("quiet project = %+v, want DC24 with no activity", projects[1])
		}
	})

	t.Run("overview_and_activity_are_set_based", func(t *testing.T) {
		h := factory(t)
		if h.Counter == nil {
			t.Skip("handle has no query counter")
		}
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, _ := seedUserAccount(t, h.Repo, uuid.New(), now)
		for i := 0; i < 12; i++ {
			pid := createProject(t, h.Repo, orgID, userID, fmt.Sprintf("P%02d", i), "Project")
			for j := 0; j < 4; j++ {
				if err := h.Repo.CreateIssue(ctx, driven.IssueRow{
					ID: uuid.New(), OrganisationID: orgID, ProjectID: pid,
					Title: "issue", Status: "open",
					CreatedAt: now.Add(-time.Duration(i*10+j) * time.Minute),
					UpdatedAt: now,
				}); err != nil {
					t.Fatal(err)
				}
			}
		}

		h.Counter.Reset()
		if _, err := h.Repo.ListActivity(ctx, userID, orgID, driven.ActivityFilter{Limit: 25}); err != nil {
			t.Fatal(err)
		}
		if n := h.Counter.Count(); n != 1 {
			t.Errorf("ListActivity issued %d statements over 12 projects, want 1", n)
		}

		h.Counter.Reset()
		if _, err := h.Repo.CountOverview(ctx, userID, orgID); err != nil {
			t.Fatal(err)
		}
		if n := h.Counter.Count(); n != 1 {
			t.Errorf("CountOverview issued %d statements, want 1", n)
		}

		h.Counter.Reset()
		if _, err := h.Repo.ListOverviewProjects(ctx, userID, orgID, 8); err != nil {
			t.Fatal(err)
		}
		if n := h.Counter.Count(); n != 1 {
			t.Errorf("ListOverviewProjects issued %d statements, want 1", n)
		}
	})
}

// runAttentionTests covers the set-based rewrite of /api/attention.
func runAttentionTests(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("attention_is_set_based_and_membership_scoped", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, _ := seedUserAccount(t, h.Repo, uuid.New(), now)

		// A project the caller belongs to, with one of each pending kind.
		mine := createProject(t, h.Repo, orgID, userID, "DC30", "Mine")
		assigned := userID
		if err := h.Repo.CreateIssue(ctx, driven.IssueRow{
			ID: uuid.New(), OrganisationID: orgID, ProjectID: mine, Title: "Assigned to me",
			Status: "open", AssigneeUserID: &assigned, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.CreateIssue(ctx, driven.IssueRow{
			ID: uuid.New(), OrganisationID: orgID, ProjectID: mine, Title: "Awaiting input",
			Status: "awaiting_input", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.CreateIssue(ctx, driven.IssueRow{
			ID: uuid.New(), OrganisationID: orgID, ProjectID: mine, Title: "Already resolved",
			Status: "resolved", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.CreateContradiction(ctx, driven.ContradictionRow{
			ID: uuid.New(), OrganisationID: orgID, ProjectID: mine, Status: "open",
			Summary: "conflict", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.CreateDecision(ctx, driven.DecisionRow{
			ID: uuid.New(), OrganisationID: orgID, ProjectID: mine, Statement: "Maybe",
			Status: "proposed", Source: "llm", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		factID := uuid.New()
		if err := h.Repo.CreateFact(ctx, driven.FactRow{
			ID: factID, OrganisationID: orgID, ProjectID: mine,
			SubjectKey: "duty", Label: "Duty", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.CreateFactVersion(ctx, driven.FactVersionRow{
			ID: uuid.New(), FactID: factID, Status: "proposed", ValueJSON: "{}",
			ValueText: "90 kW", Source: "llm", CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		// A project in the same org the caller is not a member of.
		stranger := uuid.New()
		if _, err := h.Repo.CreateUserWithHomeOrg(ctx, stranger, "s-"+stranger.String()+"@ex.com", nil, now,
			"password", stranger.String(), "s-"+stranger.String()+"@ex.com"); err != nil {
			t.Fatal(err)
		}
		theirs := uuid.New()
		if err := h.Repo.CreateProject(ctx, driven.ProjectRow{
			ID: theirs, OrganisationID: orgID, Name: "Theirs", Code: "DC31", CreatedAt: now, UpdatedAt: now,
		}, driven.ProjectMemberRow{
			ID: uuid.New(), ProjectID: theirs, UserID: stranger, Role: "owner", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.CreateContradiction(ctx, driven.ContradictionRow{
			ID: uuid.New(), OrganisationID: orgID, ProjectID: theirs, Status: "open",
			Summary: "invisible", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		if h.Counter != nil {
			h.Counter.Reset()
		}
		rows, err := h.Repo.ListAttention(ctx, userID, orgID)
		if err != nil {
			t.Fatal(err)
		}
		if h.Counter != nil {
			if n := h.Counter.Count(); n != 1 {
				t.Errorf("ListAttention issued %d statements, want 1", n)
			}
		}

		byKind := map[string]int{}
		for _, row := range rows {
			byKind[row.Kind]++
			if row.ProjectID != mine {
				t.Errorf("attention leaked a non-member project: %s", row.ProjectName)
			}
			if row.OccurredAt.IsZero() {
				t.Errorf("row %s has no timestamp, so it cannot be ordered by recency", row.Kind)
			}
		}
		for kind, want := range map[string]int{
			"issue_assignee":       1,
			"member_role":          1,
			"provisional_fact":     1,
			"provisional_decision": 1,
			"open_contradiction":   1,
		} {
			if byKind[kind] != want {
				t.Errorf("%s = %d, want %d (all kinds: %v)", kind, byKind[kind], want, byKind)
			}
		}
		// A resolved issue is not pending, and an issue assigned to the caller
		// must not also appear under member_role.
		if len(rows) != 5 {
			t.Errorf("attention rows = %d, want 5: %v", len(rows), byKind)
		}
	})
}

// runNotRelevantTests covers dismissing correspondence that is not project work.
func runNotRelevantTests(t *testing.T, factory Factory) {
	t.Helper()

	markThread := func(t *testing.T, h Handle, orgID, accountID uuid.UUID, conv string, at time.Time) {
		t.Helper()
		if err := h.Repo.UpsertThreadAssignment(context.Background(), driven.AssignmentRow{
			ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, ConversationID: conv,
			Status: "committed", Reason: "not_relevant", Source: string(domainprojects.SourceUser),
			CreatedAt: at, UpdatedAt: at, NotRelevantAt: &at,
		}); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("not_relevant_leaves_the_queue_and_is_listed_separately", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)

		insertMsg(t, h.Repo, accountID, "keep me", "conv-keep", "body", now)
		insertMsg(t, h.Repo, accountID, "newsletter", "conv-dismiss", "body", now.Add(-time.Minute))
		markThread(t, h, orgID, accountID, "conv-dismiss", now)

		queue, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		if len(queue) != 1 || queue[0].Subject != "keep me" {
			t.Fatalf("queue = %d rows, want only the undismissed one: %+v", len(queue), queue)
		}

		dismissed, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "not_relevant", Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		if len(dismissed) != 1 || dismissed[0].Subject != "newsletter" {
			t.Fatalf("dismissed = %d rows, want the newsletter: %+v", len(dismissed), dismissed)
		}
		if dismissed[0].NotRelevantAt == nil {
			t.Error("dismissed row should carry when it was marked")
		}

		// The two lists partition the rows; neither leaks into the other.
		for _, q := range queue {
			for _, d := range dismissed {
				if q.MessageID != nil && d.MessageID != nil && *q.MessageID == *d.MessageID {
					t.Error("a row appears in both the queue and the dismissed list")
				}
			}
		}

		sum, err := h.Repo.CountUnassignedSummary(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		if sum.Unassigned != 1 || sum.NotRelevant != 1 {
			t.Errorf("summary = %+v, want 1 unassigned and 1 not-relevant", sum)
		}
	})

	t.Run("later_replies_in_a_marked_thread_stay_out", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)

		insertMsg(t, h.Repo, accountID, "first", "conv-news", "body", now.Add(-time.Hour))
		markThread(t, h, orgID, accountID, "conv-news", now.Add(-30*time.Minute))
		// A reply arrives after the decision.
		insertMsg(t, h.Repo, accountID, "reply", "conv-news", "body", now)

		queue, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		if len(queue) != 0 {
			t.Fatalf("a reply to a dismissed thread must not re-enter the queue: %+v", queue)
		}

		// And the scorer must not re-propose it either.
		needing, err := h.Repo.ListMessagesNeedingAssign(ctx, userID, accountID, driven.AssignCandidateFilter{Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		if len(needing) != 0 {
			t.Fatalf("dismissed thread offered to the scorer: %d messages", len(needing))
		}
	})

	// Forward rules keep one verdict per (message, rule), and a run reads only
	// messages some rule has not finished with, within that rule's scope.
	t.Run("forward_candidates_follow_scope_and_verdicts", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC().Truncate(time.Millisecond)
		userID, _, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		if err := h.Repo.ReplaceForwardAllowlist(ctx, userID, []string{"dest@example.com"}); err != nil {
			t.Fatal(err)
		}
		at := func(d time.Duration) time.Time { return now.Add(-d) }
		old := insertMsg(t, h.Repo, accountID, "old", "c1", "b", at(72*time.Hour))
		mid := insertMsg(t, h.Repo, accountID, "mid", "c2", "b", at(48*time.Hour))
		recent := insertMsg(t, h.Repo, accountID, "recent", "c3", "b", at(time.Hour))

		epoch := time.Unix(0, 0).UTC()
		since := at(50 * time.Hour)
		rule := func(applyFrom *time.Time, enabled bool) uuid.UUID {
			id := uuid.New()
			if err := h.Repo.CreateForwardRule(ctx, driven.ForwardRuleRow{
				ID: id, UserID: userID, AccountID: accountID, Name: "r", Mode: "logic",
				ConditionJSON: `{"all":[{"field":"subject","op":"contains","value":"x"}]}`, ForwardTo: "dest@example.com",
				Enabled: enabled, ApplyFrom: applyFrom, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
			return id
		}
		everything := rule(&epoch, true)
		scoped := rule(&since, true)

		ids := func(msgs []driven.MessageRow) []uuid.UUID {
			out := []uuid.UUID{}
			for _, m := range msgs {
				out = append(out, m.ID)
			}
			return out
		}
		list := func(ruleIDs []uuid.UUID, after *driven.ForwardCandidateCursor, limit int) []driven.MessageRow {
			msgs, err := h.Repo.ListForwardCandidates(ctx, userID, accountID, ruleIDs, after, limit)
			if err != nil {
				t.Fatal(err)
			}
			return msgs
		}
		// Scope: the scoped rule alone does not reach the old message.
		if got := ids(list([]uuid.UUID{scoped}, nil, 10)); fmt.Sprint(got) != fmt.Sprint([]uuid.UUID{mid, recent}) {
			t.Fatalf("scoped candidates = %v, want mid, recent", got)
		}
		// Oldest first, and the keyset continues after the cursor.
		first := list([]uuid.UUID{everything, scoped}, nil, 1)
		if len(first) != 1 || first[0].ID != old {
			t.Fatalf("first page = %v, want old", ids(first))
		}
		rest := list([]uuid.UUID{everything, scoped}, &driven.ForwardCandidateCursor{ReceivedAt: first[0].ReceivedAt, MessageID: first[0].ID}, 10)
		if fmt.Sprint(ids(rest)) != fmt.Sprint([]uuid.UUID{mid, recent}) {
			t.Fatalf("after cursor = %v, want mid, recent", ids(rest))
		}

		// A final verdict takes a (message, rule) out; a pending one does not.
		// SQLite ties a verdict to a recorded run.
		runID := uuid.New()
		ensureLegacyJobRunIfPresent(t, h.DB, runID, accountID, now)
		verdict := func(msg, rule uuid.UUID, status string, pending bool, attempts int) {
			reason := status
			if err := h.Repo.InsertForwardAudit(ctx, driven.ForwardAuditRow{
				ID: uuid.New(), UserID: userID, AccountID: accountID, MessageID: msg, RuleID: rule, RunID: runID,
				Status: status, Reason: &reason, Pending: pending, Attempts: attempts, CreatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
		}
		verdict(old, everything, "skipped", false, 0)
		verdict(recent, everything, "forwarded", false, 0)
		verdict(recent, scoped, "failed", true, 1)
		verdict(mid, everything, "skipped", false, 0)
		verdict(mid, scoped, "skipped", false, 0)
		if got := ids(list([]uuid.UUID{everything, scoped}, nil, 10)); fmt.Sprint(got) != fmt.Sprint([]uuid.UUID{recent}) {
			t.Fatalf("candidates = %v, want only recent (pending for scoped)", got)
		}
		// Upsert: the same pair updates in place.
		verdict(recent, scoped, "forwarded", false, 1)
		if got := list([]uuid.UUID{everything, scoped}, nil, 10); len(got) != 0 {
			t.Fatalf("candidates = %v, want none once every verdict is final", ids(got))
		}
		// A disabled rule contributes nothing, even if named.
		disabled := rule(&epoch, false)
		if got := list([]uuid.UUID{disabled}, nil, 10); len(got) != 0 {
			t.Fatalf("a disabled rule produced candidates: %v", ids(got))
		}

		rows, err := h.Repo.ListForwardAuditForMessages(ctx, userID, []uuid.UUID{recent})
		if err != nil || len(rows) != 2 {
			t.Fatalf("verdicts for recent = %+v, %v", rows, err)
		}
		for _, r := range rows {
			if r.RuleID == scoped && (r.Status != "forwarded" || r.Pending || r.Attempts != 1) {
				t.Fatalf("scoped verdict = %+v", r)
			}
		}
		stats, err := h.Repo.ForwardRuleStats(ctx, userID, accountID)
		if err != nil {
			t.Fatal(err)
		}
		byRule := map[uuid.UUID]driven.ForwardRuleStats{}
		for _, s := range stats {
			byRule[s.RuleID] = s
		}
		if s := byRule[everything]; s.Forwarded != 1 || s.Failed != 0 || s.LastForwardedAt == nil {
			t.Fatalf("stats(everything) = %+v", s)
		}
		activity, err := h.Repo.ListForwardActivity(ctx, userID, everything, 10)
		if err != nil || len(activity) != 1 || activity[0].Subject != "recent" {
			t.Fatalf("activity = %+v, %v; want only the forward, not the skips", activity, err)
		}
		rules, err := h.Repo.ListForwardRules(ctx, userID, accountID)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rules {
			if r.ID == scoped && (r.ApplyFrom == nil || !r.ApplyFrom.Equal(since)) {
				t.Fatalf("apply_from round trip = %v, want %v", r.ApplyFrom, since)
			}
		}
		// Updating without a start keeps it.
		for _, r := range rules {
			if r.ID == scoped {
				r.ApplyFrom = nil
				r.Name = "renamed"
				if err := h.Repo.UpdateForwardRule(ctx, r); err != nil {
					t.Fatal(err)
				}
			}
		}
		rules, _ = h.Repo.ListForwardRules(ctx, userID, accountID)
		for _, r := range rules {
			if r.ID == scoped && (r.ApplyFrom == nil || !r.ApplyFrom.Equal(since) || r.Name != "renamed") {
				t.Fatalf("after update = %+v", r)
			}
		}
	})

	// Automatic assignment may place what nobody decided and refresh its own
	// provisional guesses, but a user's decision is final: it re-scored
	// dismissed and filed threads and put them back in triage.
	t.Run("automatic_assignment_never_touches_user_decisions", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC().Truncate(time.Second)
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		svc := &appprojects.Service{
			Users: h.Repo, Projects: h.Repo, Assignments: h.Repo, Manuals: h.Repo, Timeline: h.Repo, Contacts: h.Repo, Messages: h.Repo,
		}
		project, err := svc.Create(ctx, userID, appprojects.CreateProjectInput{Name: "Cooling", Code: "DC01"})
		if err != nil {
			t.Fatal(err)
		}
		pid := project.ID
		thread := func(conv, status, source string, projectID *uuid.UUID, dismissed bool) driven.AssignmentRow {
			row := driven.AssignmentRow{
				ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, ConversationID: conv,
				ProjectID: projectID, Status: status, Reason: "test", Source: source, CreatedAt: now, UpdatedAt: now,
			}
			if dismissed {
				row.NotRelevantAt = &now
			}
			return row
		}
		at := func(i int) time.Time { return now.Add(-time.Duration(i) * time.Minute) }

		open := insertMsg(t, h.Repo, accountID, "open", "c-open", "b", at(1))
		guessed := insertMsg(t, h.Repo, accountID, "guessed", "c-guessed", "b", at(2))
		filed := insertMsg(t, h.Repo, accountID, "filed", "c-filed", "b", at(3))
		dismissed := insertMsg(t, h.Repo, accountID, "dismissed", "c-dismissed", "b", at(4))
		insertMsg(t, h.Repo, accountID, "ruled", "c-ruled", "b", at(5))
		overGuess := insertMsg(t, h.Repo, accountID, "override guess", "c-over-guess", "b", at(6))
		overUser := insertMsg(t, h.Repo, accountID, "override user", "c-over-user", "b", at(7))
		for _, row := range []driven.AssignmentRow{
			thread("c-guessed", "provisional", "llm", &pid, false),
			thread("c-filed", "committed", "user", &pid, false),
			thread("c-dismissed", "committed", "user", nil, true),
			thread("c-ruled", "committed", "rule", &pid, false),
		} {
			if err := h.Repo.UpsertThreadAssignment(ctx, row); err != nil {
				t.Fatal(err)
			}
		}
		override := func(id uuid.UUID, source string) driven.AssignmentRow {
			mid := id
			return driven.AssignmentRow{
				OrganisationID: orgID, AccountID: accountID, MessageID: &mid, ProjectID: &pid,
				Status: "provisional", Reason: "test", Source: source, CreatedAt: now, UpdatedAt: now,
			}
		}
		if err := h.Repo.UpsertMessageOverride(ctx, override(overGuess, "llm")); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.UpsertMessageOverride(ctx, override(overUser, "user")); err != nil {
			t.Fatal(err)
		}

		// Eligible: nothing decided, or only a machine's guess; paged two at a
		// time, each exactly once, newest first.
		var got []uuid.UUID
		filter := driven.AssignCandidateFilter{Limit: 2}
		for page := 0; page < 10; page++ {
			msgs, err := h.Repo.ListMessagesNeedingAssign(ctx, userID, accountID, filter)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range msgs {
				got = append(got, m.ID)
			}
			if len(msgs) < 2 {
				break
			}
			last := msgs[len(msgs)-1]
			filter.BeforeReceivedAt, filter.BeforeID = &last.ReceivedAt, &last.ID
		}
		want := []uuid.UUID{open, guessed, overGuess}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("eligible = %v, want %v (open, guessed, override guess)", got, want)
		}

		// A machine write cannot replace a user's row, at either scope...
		if err := h.Repo.UpsertThreadAssignment(ctx, thread("c-dismissed", "provisional", "llm", &pid, false)); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.UpsertThreadAssignment(ctx, thread("c-filed", "provisional", "llm", &pid, false)); err != nil {
			t.Fatal(err)
		}
		if err := h.Repo.UpsertMessageOverride(ctx, override(overUser, "llm")); err != nil {
			t.Fatal(err)
		}
		if row, _ := h.Repo.GetThreadAssignment(ctx, accountID, "c-dismissed"); row == nil || row.Source != "user" || row.NotRelevantAt == nil || row.ProjectID != nil {
			t.Fatalf("dismissal overwritten: %+v", row)
		}
		if row, _ := h.Repo.GetThreadAssignment(ctx, accountID, "c-filed"); row == nil || row.Source != "user" || row.Status != "committed" {
			t.Fatalf("filed thread overwritten: %+v", row)
		}
		if row, _ := h.Repo.GetMessageOverride(ctx, overUser); row == nil || row.Source != "user" {
			t.Fatalf("user override overwritten: %+v", row)
		}
		queue, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range queue {
			if it.MessageID != nil && (*it.MessageID == dismissed || *it.MessageID == filed) {
				t.Fatalf("a decided thread is back in triage: %+v", it)
			}
		}

		// ...but a user can still change their mind, and a machine can
		// refresh its own guess.
		if err := h.Repo.UpsertThreadAssignment(ctx, thread("c-dismissed", "committed", "user", &pid, false)); err != nil {
			t.Fatal(err)
		}
		if row, _ := h.Repo.GetThreadAssignment(ctx, accountID, "c-dismissed"); row == nil || row.ProjectID == nil || row.NotRelevantAt != nil {
			t.Fatalf("user could not refile a dismissed thread: %+v", row)
		}
		fresh := thread("c-guessed", "provisional", "llm", &pid, false)
		fresh.Reason = "refreshed"
		if err := h.Repo.UpsertThreadAssignment(ctx, fresh); err != nil {
			t.Fatal(err)
		}
		if row, _ := h.Repo.GetThreadAssignment(ctx, accountID, "c-guessed"); row == nil || row.Reason != "refreshed" {
			t.Fatalf("machine guess not refreshed: %+v", row)
		}
	})

	// A Graph delta row need not repeat fields that did not change, and a
	// tombstone carries none at all. Upserting one must not blank a message
	// that already has a sender and a subject.
	// Contact resolution is driven off a watermark rather than a rescan, so
	// the queue has to shrink as it is worked and never hand back a message
	// twice.
	t.Run("contact_resolution_queue_drains_and_is_user_scoped", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, _, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)

		ids := make([]uuid.UUID, 0, 3)
		for i := 0; i < 3; i++ {
			id := uuid.New()
			ids = append(ids, id)
			if err := h.Repo.UpsertMessage(ctx, driven.MessageRow{
				ID: id, AccountID: accountID, ProviderMessageID: id.String(),
				ReceivedAt: now.Add(-time.Duration(i) * time.Hour), Subject: "Hello",
				FromJSON: `{"address":"a@example.com"}`, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
		}

		pending, err := h.Repo.ListMessagesNeedingContactResolution(ctx, userID, accountID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 3 {
			t.Fatalf("pending = %d, want 3", len(pending))
		}
		// Oldest first, so a backlog drains in the order it arrived.
		for i := 1; i < len(pending); i++ {
			if pending[i].ReceivedAt.Before(pending[i-1].ReceivedAt) {
				t.Fatalf("results are not oldest first: %v", pending)
			}
		}

		if err := h.Repo.MarkContactsResolved(ctx, userID, ids[:2], now); err != nil {
			t.Fatal(err)
		}
		left, err := h.Repo.ListMessagesNeedingContactResolution(ctx, userID, accountID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(left) != 1 {
			t.Fatalf("pending after marking two = %d, want 1", len(left))
		}

		// Another user cannot mark this account's mail as done.
		otherUser, _, _ := seedUserAccount(t, h.Repo, uuid.New(), now)
		if err := h.Repo.MarkContactsResolved(ctx, otherUser, []uuid.UUID{left[0].ID}, now); err != nil {
			t.Fatal(err)
		}
		still, err := h.Repo.ListMessagesNeedingContactResolution(ctx, userID, accountID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(still) != 1 {
			t.Errorf("another user's mark took effect: pending = %d, want 1", len(still))
		}
	})

	t.Run("a_partial_upsert_does_not_blank_a_populated_message", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, _, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		body := "You have a past due balance."
		received := now.Add(-72 * time.Hour)
		msgID := uuid.New()
		if err := h.Repo.UpsertMessage(ctx, driven.MessageRow{
			ID: msgID, AccountID: accountID, ProviderMessageID: "provider-1",
			ReceivedAt: received, Subject: "Your online bill",
			FromJSON: `{"name":"Microsoft","address":"billing@microsoft.com"}`,
			BodyText: &body, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		// The same message arriving with nothing in it.
		if err := h.Repo.UpsertMessage(ctx, driven.MessageRow{
			ID: uuid.New(), AccountID: accountID, ProviderMessageID: "provider-1",
			ReceivedAt: now, Subject: "", FromJSON: "",
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		got, err := h.Repo.GetMessage(ctx, userID, msgID)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil {
			t.Fatal("message disappeared")
		}
		if got.Subject != "Your online bill" {
			t.Errorf("subject = %q, want it kept", got.Subject)
		}
		if !strings.Contains(got.FromJSON, "billing@microsoft.com") {
			t.Errorf("from_json = %q, want the sender kept", got.FromJSON)
		}
		if got.BodyText == nil || *got.BodyText != body {
			t.Errorf("body_text = %v, want it kept", got.BodyText)
		}
	})

	t.Run("message_scope_marking_leaves_the_rest_of_the_thread", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)

		a := insertMsg(t, h.Repo, accountID, "dismiss just me", "conv-mixed", "body", now)
		insertMsg(t, h.Repo, accountID, "still queued", "conv-mixed", "body", now.Add(-time.Minute))

		mid := a
		at := now
		if err := h.Repo.UpsertMessageOverride(ctx, driven.AssignmentRow{
			OrganisationID: orgID, AccountID: accountID, MessageID: &mid,
			Status: "committed", Reason: "not_relevant", Source: string(domainprojects.SourceUser),
			CreatedAt: now, UpdatedAt: now, NotRelevantAt: &at,
		}); err != nil {
			t.Fatal(err)
		}

		queue, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		if len(queue) != 1 {
			t.Fatalf("queue = %d rows, want the rest of the thread: %+v", len(queue), queue)
		}
		if queue[0].MessageID != nil && *queue[0].MessageID == a {
			t.Error("the dismissed message is still in the queue")
		}

		// The message-scope decision overrides the thread, as Wave 1 §7 requires.
		eff, err := h.Repo.EffectiveAssignment(ctx, userID, a)
		if err != nil {
			t.Fatal(err)
		}
		if eff.NotRelevantAt == nil {
			t.Error("effective assignment should report the dismissal")
		}
		if eff.Scope != "message" {
			t.Errorf("scope = %q, want message", eff.Scope)
		}
	})

	t.Run("assigning_a_project_clears_the_dismissal", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC40", "Later")

		msgID := insertMsg(t, h.Repo, accountID, "turns out relevant", "conv-turn", "body", now)
		markThread(t, h, orgID, accountID, "conv-turn", now)

		// Assigning writes the whole row, so the dismissal must not survive.
		if err := h.Repo.UpsertThreadAssignment(ctx, driven.AssignmentRow{
			ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, ConversationID: "conv-turn",
			ProjectID: &projectID, Status: "committed", Reason: "user_assign",
			Source: string(domainprojects.SourceUser), CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		eff, err := h.Repo.EffectiveAssignment(ctx, userID, msgID)
		if err != nil {
			t.Fatal(err)
		}
		if eff.ProjectID == nil || *eff.ProjectID != projectID {
			t.Fatalf("expected the project to be assigned, got %+v", eff)
		}
		if eff.NotRelevantAt != nil {
			t.Error("assigning a project must clear the dismissal; an item cannot be both")
		}
		dismissed, err := h.Repo.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "not_relevant", Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		if len(dismissed) != 0 {
			t.Errorf("assigned item still listed as not-relevant: %+v", dismissed)
		}
	})
}

// runExtractionDebounceTests covers when a project becomes due for extraction.
func runExtractionDebounceTests(t *testing.T, factory Factory) {
	t.Helper()
	const quiet = time.Minute
	const ceiling = 5 * time.Minute

	assign := func(t *testing.T, h Handle, orgID, accountID, projectID uuid.UUID, conv string, at time.Time) {
		t.Helper()
		if err := h.Repo.UpsertThreadAssignment(context.Background(), driven.AssignmentRow{
			ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, ConversationID: conv,
			ProjectID: &projectID, Status: "committed", Reason: "user_assign",
			Source: string(domainprojects.SourceUser), CreatedAt: at, UpdatedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	isDue := func(t *testing.T, h Handle, now time.Time, projectID uuid.UUID) bool {
		t.Helper()
		due, err := h.Repo.ListProjectsDueForExtraction(context.Background(), now, quiet, ceiling, 50)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range due {
			if d.ProjectID == projectID {
				return true
			}
		}
		return false
	}

	t.Run("extraction_waits_for_quiet_then_fires", func(t *testing.T) {
		h := factory(t)
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC50", "Debounce")

		// Correspondence just arrived: still inside the quiet window.
		assign(t, h, orgID, accountID, projectID, "conv-1", now.Add(-10*time.Second))
		if isDue(t, h, now, projectID) {
			t.Error("project should not be due while correspondence is still arriving")
		}

		// A minute of quiet makes it due.
		if !isDue(t, h, now.Add(quiet), projectID) {
			t.Error("project should be due after a minute of quiet")
		}
	})

	t.Run("extraction_fires_at_the_ceiling_under_continuous_assignment", func(t *testing.T) {
		h := factory(t)
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC51", "Ceiling")

		// A steady trickle: something arrives every 20 seconds for six minutes,
		// so the quiet window never closes.
		for i := 0; i < 18; i++ {
			assign(t, h, orgID, accountID, projectID, fmt.Sprintf("conv-%d", i),
				now.Add(-6*time.Minute).Add(time.Duration(i*20)*time.Second))
		}
		// Newest is 20s old, so quiet alone would keep deferring forever.
		if !isDue(t, h, now, projectID) {
			t.Error("the ceiling should force extraction under continuous assignment")
		}
	})

	t.Run("extraction_not_due_without_new_correspondence", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC52", "Quiet")

		// A project with no correspondence is never due.
		if isDue(t, h, now, projectID) {
			t.Error("a project with no correspondence should not be due")
		}

		assign(t, h, orgID, accountID, projectID, "conv-1", now.Add(-10*time.Minute))
		if !isDue(t, h, now, projectID) {
			t.Fatal("expected the project to be due before extraction")
		}

		// After extraction it settles.
		if err := h.Repo.MarkProjectExtracted(ctx, projectID, now); err != nil {
			t.Fatal(err)
		}
		if isDue(t, h, now.Add(time.Hour), projectID) {
			t.Error("a project with nothing newer than its watermark should not be due again")
		}

		// New correspondence after the watermark makes it due once more.
		assign(t, h, orgID, accountID, projectID, "conv-2", now.Add(time.Minute))
		if !isDue(t, h, now.Add(10*time.Minute), projectID) {
			t.Error("correspondence newer than the watermark should make the project due")
		}
	})

	t.Run("extraction_ignores_not_project_related", func(t *testing.T) {
		h := factory(t)
		now := time.Now().UTC()
		userID, orgID, accountID := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC53", "Dismissed")

		// A dismissal carries no project, so it cannot make one due.
		at := now.Add(-10 * time.Minute)
		if err := h.Repo.UpsertThreadAssignment(context.Background(), driven.AssignmentRow{
			ID: uuid.New(), OrganisationID: orgID, AccountID: accountID, ConversationID: "conv-dismissed",
			Status: "committed", Reason: "not_relevant", Source: string(domainprojects.SourceUser),
			CreatedAt: at, UpdatedAt: at, NotRelevantAt: &at,
		}); err != nil {
			t.Fatal(err)
		}
		if isDue(t, h, now, projectID) {
			t.Error("dismissed correspondence must not trigger extraction")
		}
	})

	t.Run("extraction_watermark_is_readable_on_the_project", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		now := time.Now().UTC().Truncate(time.Second)
		userID, orgID, _ := seedUserAccount(t, h.Repo, uuid.New(), now)
		projectID := createProject(t, h.Repo, orgID, userID, "DC54", "Watermark")

		// The UI reports "reviewed N minutes ago" from this, so the watermark
		// has to survive the round trip, not just drive the due query.
		before, err := h.Repo.GetProject(ctx, orgID, projectID)
		if err != nil {
			t.Fatal(err)
		}
		if before.LastExtractedAt != nil {
			t.Errorf("a project that never ran reports %v, want nil", before.LastExtractedAt)
		}

		if err := h.Repo.MarkProjectExtracted(ctx, projectID, now); err != nil {
			t.Fatal(err)
		}
		after, err := h.Repo.GetProject(ctx, orgID, projectID)
		if err != nil {
			t.Fatal(err)
		}
		if after.LastExtractedAt == nil {
			t.Fatal("watermark did not survive the round trip")
		}
		if got := after.LastExtractedAt.UTC(); !got.Equal(now) {
			t.Errorf("watermark is %s, want %s", got, now)
		}
	})

}
