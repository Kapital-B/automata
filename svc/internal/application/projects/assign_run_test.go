package projects

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

type stubUsers struct {
	driven.UserRepository
	orgID uuid.UUID
}

func (s stubUsers) GetHomeOrganisationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return s.orgID, nil
}

type stubProjects struct {
	driven.ProjectRepository
	rows []driven.ProjectRow
}

func (s stubProjects) ListProjects(ctx context.Context, orgID uuid.UUID, f driven.ProjectListFilter) ([]driven.ProjectRow, error) {
	return s.rows, nil
}

func (s stubProjects) UpsertProjectParticipant(ctx context.Context, projectID, contactID uuid.UUID, at time.Time) error {
	return nil
}

// failingAssignments accepts reads but rejects every write.
type failingAssignments struct {
	driven.AssignmentRepository
	msgs []driven.MessageRow
}

func (f *failingAssignments) ListMessagesNeedingAssign(ctx context.Context, userID, accountID uuid.UUID, filter driven.AssignCandidateFilter) ([]driven.MessageRow, error) {
	return f.msgs, nil
}

func (f *failingAssignments) FindCommittedSiblingProject(ctx context.Context, userID, accountID uuid.UUID, conversationID string, exclude uuid.UUID) (*uuid.UUID, error) {
	return nil, nil
}

func (f *failingAssignments) UpsertThreadAssignment(ctx context.Context, row driven.AssignmentRow) error {
	return errors.New("write rejected")
}

func (f *failingAssignments) UpsertMessageOverride(ctx context.Context, row driven.AssignmentRow) error {
	return errors.New("write rejected")
}

type recordingJobRuns struct {
	driven.JobRunRepository
	status string
	errMsg string
	meta   string
}

func (r *recordingJobRuns) InsertJobRun(ctx context.Context, id, accountID uuid.UUID, jobType, trigger, status string, startedAt, finishedAt time.Time, errMsg *string, metaJSON string) error {
	return nil
}

func (r *recordingJobRuns) UpdateJobRunStatus(ctx context.Context, id uuid.UUID, status string, finishedAt *time.Time, errMsg *string, metaJSON string) error {
	r.status = status
	r.meta = metaJSON
	if errMsg != nil {
		r.errMsg = *errMsg
	}
	return nil
}

type stubContacts struct {
	driven.ContactRepository
}

func (stubContacts) ListContactIDsForMessage(ctx context.Context, orgID, messageID uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

// TestAssignAfterSyncReportsFailure covers the case that made a broken
// assignment pass invisible: errors were swallowed per message and the run was
// recorded "success" regardless, so "the scorer is down" looked exactly like
// "nothing matched".
func TestAssignAfterSyncReportsFailure(t *testing.T) {
	orgID := uuid.New()
	dc01 := proj("DC01", "Cooling")
	body := "please review"
	conv := "conv-1"
	msgs := []driven.MessageRow{{
		ID: uuid.New(), AccountID: uuid.New(), Subject: "Regarding DC01",
		BodyText: &body, ConversationID: &conv, FromJSON: `{"address":"a@b.com"}`,
	}}

	runs := &recordingJobRuns{}
	svc := &AssignService{
		Users:       stubUsers{orgID: orgID},
		Projects:    stubProjects{rows: []driven.ProjectRow{dc01}},
		Assignments: &failingAssignments{msgs: msgs},
		Contacts:    stubContacts{},
		JobRuns:     runs,
	}

	err := svc.AssignAfterSync(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected AssignAfterSync to surface the write failure")
	}
	if runs.status != "failed" {
		t.Errorf("run status = %q, want failed", runs.status)
	}
	if !strings.Contains(runs.errMsg, "failed to assign") {
		t.Errorf("run error = %q, want it to name the failure", runs.errMsg)
	}
	if !strings.Contains(runs.meta, `"errors":1`) {
		t.Errorf("run meta = %q, want an error count", runs.meta)
	}
}

// TestAssignAfterSyncRecordsOutcomeBreakdown makes a run that scored nothing
// distinguishable from a run that failed.
func TestAssignAfterSyncRecordsOutcomeBreakdown(t *testing.T) {
	orgID := uuid.New()
	body := "nothing relevant here"
	conv := "conv-1"
	msgs := []driven.MessageRow{{
		ID: uuid.New(), AccountID: uuid.New(), Subject: "lunch?",
		BodyText: &body, ConversationID: &conv, FromJSON: `{"address":"a@b.com"}`,
	}}

	runs := &recordingJobRuns{}
	svc := &AssignService{
		Users:       stubUsers{orgID: orgID},
		Projects:    stubProjects{rows: []driven.ProjectRow{proj("DC01", "Cooling")}},
		Assignments: &failingAssignments{msgs: msgs},
		Contacts:    stubContacts{},
		JobRuns:     runs,
	}

	if err := svc.AssignAfterSync(context.Background(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("nothing matched, so there is nothing to fail: %v", err)
	}
	if runs.status != "success" {
		t.Errorf("run status = %q, want success", runs.status)
	}
	for _, want := range []string{`"unscored":1`, `"errors":0`, `"committed_rule":0`, `"provisional_llm":0`} {
		if !strings.Contains(runs.meta, want) {
			t.Errorf("run meta %q missing %s", runs.meta, want)
		}
	}
}
