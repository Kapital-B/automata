package projects

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

// MaxBatchAssignItems caps one batch request. Triage selections are operator
// sized; a larger request is a client bug, not a workload.
const MaxBatchAssignItems = 200

// ErrTooManyItems is returned when a batch exceeds MaxBatchAssignItems.
var ErrTooManyItems = fmt.Errorf("too_many_items")

// BatchAssignItem is one assignment in a batch. A nil ProjectID clears the
// assignment, matching the single-item endpoints.
type BatchAssignItem struct {
	Kind      string // "message" | "manual"
	ID        uuid.UUID
	ProjectID *uuid.UUID
	Scope     domainprojects.AssignScope // messages only; defaults to thread
	// NotRelevant marks (true) or restores (false) correspondence that is not
	// project work. Nil leaves the flag untouched, so existing callers are
	// unaffected.
	NotRelevant *bool
}

// BatchAssignResult reports the outcome of one item. Errors are machine
// readable so the UI can explain a partial failure without parsing prose.
type BatchAssignResult struct {
	ID    uuid.UUID
	OK    bool
	Error string
}

// AssignBatch assigns or clears many items in one call.
//
// Items succeed or fail independently: one bad item must not sink the batch.
// Unlike a loop over AssignMessage, project validation is hoisted out of the
// per-item path and the post-commit interpret/reconcile hook fires once per
// distinct project rather than once per item.
func (s *Service) AssignBatch(ctx context.Context, userID uuid.UUID, items []BatchAssignItem) ([]BatchAssignResult, error) {
	if len(items) > MaxBatchAssignItems {
		return nil, ErrTooManyItems
	}
	results := make([]BatchAssignResult, len(items))
	for i, it := range items {
		results[i] = BatchAssignResult{ID: it.ID, OK: true}
	}
	if len(items) == 0 {
		return results, nil
	}

	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Resolve every distinct project once instead of per item.
	projects := map[uuid.UUID]*driven.ProjectRow{}
	for _, it := range items {
		if it.ProjectID == nil {
			continue
		}
		if _, done := projects[*it.ProjectID]; done {
			continue
		}
		p, err := s.Projects.GetProject(ctx, orgID, *it.ProjectID)
		if err != nil {
			return nil, err
		}
		if p != nil && p.ArchivedAt != nil {
			p = nil
		}
		projects[*it.ProjectID] = p
	}

	now := time.Now().UTC()
	uid := userID

	var (
		threadRows   []driven.AssignmentRow
		overrideRows []driven.AssignmentRow
		threadClears []threadClear
		// Index of the result each pending write belongs to, so a chunk failure
		// can be reported against the right items.
		threadIdx   []int
		overrideIdx []int
	)
	var commits []batchCommit

	fail := func(i int, code string) {
		results[i].OK = false
		results[i].Error = code
	}

	for i, it := range items {
		if it.ProjectID != nil && it.NotRelevant != nil && *it.NotRelevant {
			fail(i, "project_and_not_relevant")
			continue
		}
		if it.ProjectID != nil && projects[*it.ProjectID] == nil {
			fail(i, "not_found")
			continue
		}
		switch strings.TrimSpace(strings.ToLower(it.Kind)) {
		case "manual":
			if err := s.batchAssignManual(ctx, orgID, it); err != nil {
				fail(i, batchErrorCode(err))
				continue
			}
			if it.ProjectID != nil && !marksNotRelevant(it) {
				mid := it.ID
				commits = append(commits, batchCommit{projectID: *it.ProjectID, manualItemID: &mid})
			}
		case "message":
			msg, err := s.Messages.GetMessage(ctx, userID, it.ID)
			if err != nil {
				return nil, err
			}
			if msg == nil {
				fail(i, "not_found")
				continue
			}
			scope := it.Scope
			if scope == "" {
				scope = domainprojects.ScopeThread
			}
			if !scope.Valid() {
				fail(i, "invalid_scope")
				continue
			}
			switch scope {
			case domainprojects.ScopeThread:
				if msg.ConversationID == nil || strings.TrimSpace(*msg.ConversationID) == "" {
					fail(i, "conversation_required")
					continue
				}
				if marksNotRelevant(it) {
					// A settled decision that there is no project: committed,
					// with no project id and the time it was decided.
					threadRows = append(threadRows, driven.AssignmentRow{
						ID: uuid.New(), OrganisationID: orgID, AccountID: msg.AccountID,
						ConversationID: *msg.ConversationID,
						Status:         string(domainprojects.StatusCommitted), Reason: notRelevantReason,
						Source: string(domainprojects.SourceUser), AssignedByUserID: &uid,
						CreatedAt: now, UpdatedAt: now, NotRelevantAt: &now,
					})
					threadIdx = append(threadIdx, i)
					continue
				}
				if it.ProjectID == nil {
					// Covers both an explicit clear and a restore: removing the
					// row returns the thread to the queue.
					threadClears = append(threadClears, threadClear{index: i, accountID: msg.AccountID, conversationID: *msg.ConversationID})
					continue
				}
				threadRows = append(threadRows, driven.AssignmentRow{
					ID: uuid.New(), OrganisationID: orgID, AccountID: msg.AccountID,
					ConversationID: *msg.ConversationID, ProjectID: it.ProjectID,
					Status: string(domainprojects.StatusCommitted), Reason: "user_assign",
					Source: string(domainprojects.SourceUser), AssignedByUserID: &uid,
					CreatedAt: now, UpdatedAt: now,
				})
				threadIdx = append(threadIdx, i)
			case domainprojects.ScopeMessage:
				mid := it.ID
				row := driven.AssignmentRow{
					OrganisationID: orgID, AccountID: msg.AccountID, MessageID: &mid,
					ProjectID: it.ProjectID, Status: string(domainprojects.StatusCommitted),
					Reason: "user_assign", Source: string(domainprojects.SourceUser),
					AssignedByUserID: &uid, CreatedAt: now, UpdatedAt: now,
				}
				if marksNotRelevant(it) {
					row.ProjectID = nil
					row.Reason = notRelevantReason
					row.NotRelevantAt = &now
				}
				overrideRows = append(overrideRows, row)
				overrideIdx = append(overrideIdx, i)
			}
			if it.ProjectID != nil && !marksNotRelevant(it) {
				mid := it.ID
				commits = append(commits, batchCommit{projectID: *it.ProjectID, messageID: &mid, msg: msg})
			}
		default:
			fail(i, "invalid_kind")
		}
	}

	// A chunk failure part-way through leaves earlier chunks committed. We report
	// every row in the failed write as failed rather than probing which landed:
	// assignment upserts are idempotent, so a client retry is free, and
	// over-reporting failure is safer than claiming a write that did not happen.
	if len(threadRows) > 0 {
		if err := s.Assignments.UpsertThreadAssignments(ctx, threadRows); err != nil {
			markFailed(results, threadIdx, batchErrorCode(err))
			commits = dropCommitsFor(commits, results)
		}
	}
	if len(overrideRows) > 0 {
		if err := s.Assignments.UpsertMessageOverrides(ctx, overrideRows); err != nil {
			markFailed(results, overrideIdx, batchErrorCode(err))
			commits = dropCommitsFor(commits, results)
		}
	}
	for _, c := range threadClears {
		if err := s.Assignments.DeleteThreadAssignment(ctx, c.accountID, c.conversationID); err != nil {
			fail(c.index, batchErrorCode(err))
		}
	}

	// Participants are deduped across the batch, then the downstream hook runs
	// once per distinct project rather than once per item.
	touchedProjects := map[uuid.UUID]batchCommit{}
	for _, c := range commits {
		if c.messageID != nil && c.msg != nil {
			_ = s.upsertParticipantsFromMessage(ctx, orgID, c.projectID, c.msg)
		}
		if c.manualItemID != nil {
			ids, _ := s.Contacts.ListContactIDsForManualItem(ctx, orgID, *c.manualItemID)
			for _, cid := range ids {
				_ = s.Projects.UpsertProjectParticipant(ctx, c.projectID, cid, now)
			}
		}
		if _, seen := touchedProjects[c.projectID]; !seen {
			touchedProjects[c.projectID] = c
		}
	}
	if s.AfterProjectCorrespondence != nil {
		for pid, c := range touchedProjects {
			projectID := pid
			s.AfterProjectCorrespondence(ctx, userID, projectID, c.messageID, c.manualItemID)
		}
	}

	return results, nil
}

// batchCommit is post-write work deferred until every row has landed.
type batchCommit struct {
	projectID    uuid.UUID
	messageID    *uuid.UUID
	manualItemID *uuid.UUID
	msg          *driven.MessageRow
}

// notRelevantReason labels an assignment row that records "no project".
const notRelevantReason = "not_relevant"

// marksNotRelevant reports whether the item dismisses correspondence. A nil
// flag leaves the existing decision alone.
func marksNotRelevant(it BatchAssignItem) bool {
	return it.NotRelevant != nil && *it.NotRelevant
}

type threadClear struct {
	index          int
	accountID      uuid.UUID
	conversationID string
}

func (s *Service) batchAssignManual(ctx context.Context, orgID uuid.UUID, it BatchAssignItem) error {
	if s.Manuals == nil {
		return fmt.Errorf("manuals not configured")
	}
	item, err := s.Manuals.GetManualItem(ctx, orgID, it.ID)
	if err != nil {
		return err
	}
	if item == nil {
		return ErrNotFound
	}
	status := "unassigned"
	reason := "user_assign"
	var notRelevantAt *time.Time
	if marksNotRelevant(it) {
		status = string(domainprojects.StatusCommitted)
		reason = notRelevantReason
		at := time.Now().UTC()
		notRelevantAt = &at
	} else if it.ProjectID != nil {
		status = string(domainprojects.StatusCommitted)
	}
	projectID := it.ProjectID
	if notRelevantAt != nil {
		projectID = nil
	}
	return s.Manuals.UpdateManualItemAssignment(ctx, orgID, it.ID, projectID, status,
		reason, string(domainprojects.SourceUser), notRelevantAt)
}

func markFailed(results []BatchAssignResult, idx []int, code string) {
	for _, i := range idx {
		results[i].OK = false
		results[i].Error = code
	}
}

// dropCommitsFor removes post-commit work for items whose write failed.
func dropCommitsFor(commits []batchCommit, results []BatchAssignResult) []batchCommit {
	ok := map[uuid.UUID]bool{}
	for _, r := range results {
		ok[r.ID] = r.OK
	}
	out := commits[:0]
	for _, c := range commits {
		id := uuid.Nil
		switch {
		case c.messageID != nil:
			id = *c.messageID
		case c.manualItemID != nil:
			id = *c.manualItemID
		}
		if ok[id] {
			out = append(out, c)
		}
	}
	return out
}

func batchErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case strings.Contains(err.Error(), "conversation_required"):
		return "conversation_required"
	case strings.Contains(strings.ToLower(err.Error()), "not found"):
		return "not_found"
	default:
		return "write_failed"
	}
}
