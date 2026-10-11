package submissions

import (
	"context"
	"errors"
	accounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	jobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	"github.com/google/uuid"
	"testing"
	"time"
)

type scopedAccounts struct {
	accounts.AccountRepository
	owner uuid.UUID
}

func (f scopedAccounts) GetAccount(_ context.Context, user, id uuid.UUID) (*accounts.AccountRow, []byte, error) {
	if user != f.owner {
		return nil, nil, nil
	}
	return &accounts.AccountRow{ID: id}, nil, nil
}

type queueFailure struct {
	jobs.LegacyQueue
	calls int
	err   error
}

func (f *queueFailure) EnqueueCategorize(context.Context, jobs.LegacyJobRequest) error {
	f.calls++
	return f.err
}

type runStore struct {
	jobs.JobRunRepository
	id           uuid.UUID
	status, meta string
	insertError  error
}

func (f *runStore) InsertJobRun(_ context.Context, id, account uuid.UUID, kind, trigger, status string, start, end time.Time, msg *string, meta string) error {
	f.id = id
	f.status = status
	return f.insertError
}
func (f *runStore) UpdateJobRunStatus(_ context.Context, id uuid.UUID, status string, end *time.Time, msg *string, meta string) error {
	if id != f.id {
		panic("wrong run")
	}
	f.status = status
	f.meta = meta
	return nil
}
func TestLegacySubmissionScopesAndCompensatesFailures(t *testing.T) {
	owner, account := uuid.New(), uuid.New()
	queue := &queueFailure{err: errors.New("queue unavailable")}
	runs := &runStore{}
	s := &Service{Accounts: scopedAccounts{owner: owner}, Legacy: queue, Runs: runs}
	if _, err := s.SubmitAccount(context.Background(), uuid.New(), account, "categorize", jobs.JobPayload{}); err == nil || queue.calls != 0 {
		t.Fatalf("scope: %v", err)
	}
	_, err := s.SubmitAccount(context.Background(), owner, account, "categorize", jobs.JobPayload{Recategorize: true})
	if !errors.Is(err, queue.err) || queue.calls != 1 || runs.status != "failed" || runs.meta != `{"queued":false,"recategorize":true}` {
		t.Fatalf("%v %+v", err, runs)
	}
	runs.insertError = errors.New("store unavailable")
	_, err = s.SubmitAccount(context.Background(), owner, account, "categorize", jobs.JobPayload{})
	if !errors.Is(err, runs.insertError) || queue.calls != 1 {
		t.Fatalf("pending write failure: %v", err)
	}
}
