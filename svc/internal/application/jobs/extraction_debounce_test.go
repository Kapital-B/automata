package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/memoryjobs"
	appjobs "github.com/Kapital-B/automata/svc/internal/application/jobs"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

// stubDueProjects reports a fixed due list and records the debounce windows it
// was asked for.
type stubDueProjects struct {
	driven.ProjectRepository
	due       []driven.DueProject
	gotQuiet  time.Duration
	gotCeil   time.Duration
	extracted []uuid.UUID
}

func (s *stubDueProjects) ListProjectsDueForExtraction(ctx context.Context, now time.Time, quietFor, ceiling time.Duration, limit int) ([]driven.DueProject, error) {
	s.gotQuiet = quietFor
	s.gotCeil = ceiling
	return s.due, nil
}

func (s *stubDueProjects) MarkProjectExtracted(ctx context.Context, projectID uuid.UUID, at time.Time) error {
	s.extracted = append(s.extracted, projectID)
	return nil
}

func TestSchedulerEnqueuesExtractionForDueProjects(t *testing.T) {
	store := memoryjobs.NewStore()
	projectA, projectB := uuid.New(), uuid.New()
	owner := uuid.New()
	projects := &stubDueProjects{due: []driven.DueProject{
		{ProjectID: projectA, OwnerUserID: owner},
		{ProjectID: projectB, OwnerUserID: owner},
	}}
	sched := &appjobs.SchedulerService{
		Store:    store,
		Projects: projects,
		Enqueuer: &appjobs.Enqueuer{Store: store, Registry: appjobs.DefaultRegistry()},
		Registry: appjobs.DefaultRegistry(),
	}

	if err := sched.Tick(context.Background(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	// The operator's chosen debounce: a minute of quiet, five minute ceiling.
	if projects.gotQuiet != time.Minute {
		t.Errorf("quiet window = %v, want 1m", projects.gotQuiet)
	}
	if projects.gotCeil != 5*time.Minute {
		t.Errorf("ceiling = %v, want 5m", projects.gotCeil)
	}

	page, err := store.List(context.Background(), driven.JobListFilter{UserID: owner, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[uuid.UUID]string{}
	for _, job := range page.Jobs {
		if job.Payload.ProjectID != nil {
			seen[*job.Payload.ProjectID] = job.JobType
		}
	}
	for _, id := range []uuid.UUID{projectA, projectB} {
		if seen[id] != appjobs.TypeInterpretProject {
			t.Errorf("project %s enqueued as %q, want interpret_project", id, seen[id])
		}
	}
}

// A project already being extracted must not be enqueued again, and a held
// lock must not fail the whole tick.
func TestSchedulerCoalescesExtractionAndSurvivesLockConflicts(t *testing.T) {
	store := memoryjobs.NewStore()
	projectID := uuid.New()
	owner := uuid.New()
	projects := &stubDueProjects{due: []driven.DueProject{{ProjectID: projectID, OwnerUserID: owner}}}
	enq := &appjobs.Enqueuer{Store: store, Registry: appjobs.DefaultRegistry()}
	sched := &appjobs.SchedulerService{
		Store: store, Projects: projects, Enqueuer: enq, Registry: appjobs.DefaultRegistry(),
	}
	ctx := context.Background()

	if err := sched.Tick(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	// The project stays due until extraction completes, so the next tick sees
	// it again and must coalesce rather than error.
	if err := sched.Tick(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("a held extraction lock must not fail the tick: %v", err)
	}

	page, err := store.List(ctx, driven.JobListFilter{UserID: owner, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, job := range page.Jobs {
		if job.Payload.ProjectID != nil && *job.Payload.ProjectID == projectID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("project enqueued %d times across two ticks, want 1", count)
	}
}

func TestSchedulerSkipsExtractionWithoutProjects(t *testing.T) {
	store := memoryjobs.NewStore()
	sched := &appjobs.SchedulerService{
		Store:    store,
		Enqueuer: &appjobs.Enqueuer{Store: store, Registry: appjobs.DefaultRegistry()},
		Registry: appjobs.DefaultRegistry(),
	}
	if err := sched.Tick(context.Background(), time.Now().UTC()); err != nil {
		t.Fatalf("the pass must be optional: %v", err)
	}
}
