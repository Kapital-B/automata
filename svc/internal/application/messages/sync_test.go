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

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type passthroughVault struct{}

func (v *passthroughVault) Encrypt(plaintext []byte) ([]byte, error)  { return plaintext, nil }
func (v *passthroughVault) Decrypt(ciphertext []byte) ([]byte, error) { return ciphertext, nil }

// fakeMailbox scripts ListChanges pages and records outbound calls. It stands
// in for any provider: services only ever see the port.
type fakeMailbox struct {
	caps      driven.MailboxCapabilities
	calls     []string
	results   []*driven.MailChangePage
	errByCall map[int]error
	resultIdx int

	forwardCalls int
	forwardErr   error
	replyCalls   int
	replyErr     error
}

func (f *fakeMailbox) Capabilities() driven.MailboxCapabilities { return f.caps }

func (f *fakeMailbox) ListChanges(ctx context.Context, cursor string, pageSize int) (*driven.MailChangePage, error) {
	f.calls = append(f.calls, cursor)
	call := len(f.calls) - 1
	if f.errByCall != nil {
		if err, ok := f.errByCall[call]; ok {
			return nil, err
		}
	}
	if f.resultIdx >= len(f.results) {
		return &driven.MailChangePage{FinalCursor: "delta-empty"}, nil
	}
	out := f.results[f.resultIdx]
	f.resultIdx++
	return out, nil
}

func (f *fakeMailbox) GetRawMessage(ctx context.Context, providerMessageID string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeMailbox) Reply(ctx context.Context, providerMessageID, body string) error {
	f.replyCalls++
	return f.replyErr
}

func (f *fakeMailbox) Forward(ctx context.Context, providerMessageID, to, comment string) error {
	f.forwardCalls++
	return f.forwardErr
}

// fakeProvider hands out one mailbox, so tests drive the real opener.
type fakeProvider struct {
	box     *fakeMailbox
	openErr error
	opens   int
}

func (p *fakeProvider) Capabilities() driven.MailboxCapabilities { return p.box.Capabilities() }

func (p *fakeProvider) Open(ctx context.Context, account driven.AccountRow, credential []byte) (driven.Mailbox, []byte, error) {
	p.opens++
	if p.openErr != nil {
		return nil, nil, p.openErr
	}
	return p.box, nil, nil
}

func testOpener(accounts driven.AccountRepository, box *fakeMailbox) *appaccounts.MailboxOpener {
	return &appaccounts.MailboxOpener{
		Accounts:  accounts,
		Vault:     &passthroughVault{},
		Providers: map[string]driven.MailProvider{appaccounts.ProviderM365: &fakeProvider{box: box}},
	}
}

func setupSyncService(t *testing.T, graph *fakeMailbox) (*SyncService, *sqlite.Repository, uuid.UUID, uuid.UUID) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	userID := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	accountID := uuid.New()
	payload := []byte("credential")
	if err := repo.InsertAccount(context.Background(), driven.AccountRow{
		UserID:           userID,
		ID:               accountID,
		Label:            "Work",
		Provider:         "m365",
		MsAccountKind:    domainacc.KindWork,
		PrimaryEmail:     "work@example.com",
		ConnectionStatus: "connected",
	}, payload); err != nil {
		t.Fatal(err)
	}
	svc := &SyncService{
		Accounts:  repo,
		Messages:  repo,
		Mailboxes: testOpener(repo, graph),
		JobRuns:   repo,
	}
	return svc, repo, userID, accountID
}

func TestSyncInboxUsesDeltaLinkAcrossRuns(t *testing.T) {
	graph := &fakeMailbox{
		results: []*driven.MailChangePage{
			{
				Messages: []driven.MailMessage{
					{ID: "provider-1", Subject: "one", ReceivedDateTime: time.Now().UTC().Format(time.RFC3339), FromAddress: "a@example.com"},
				},
				FinalCursor: "delta-1",
			},
			{
				Messages: []driven.MailMessage{
					{ID: "provider-2", Subject: "two", ReceivedDateTime: time.Now().UTC().Format(time.RFC3339), FromAddress: "b@example.com"},
				},
				FinalCursor: "delta-2",
			},
		},
	}
	svc, repo, userID, accountID := setupSyncService(t, graph)

	if _, err := svc.SyncInbox(context.Background(), userID, accountID); err != nil {
		t.Fatal(err)
	}
	if got := len(graph.calls); got != 1 || graph.calls[0] != "" {
		t.Fatalf("expected first delta call with empty link, got %#v", graph.calls)
	}
	storedAfterFirst, err := repo.GetSyncDeltaLink(context.Background(), userID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if storedAfterFirst == nil || *storedAfterFirst != "delta-1" {
		t.Fatalf("expected stored delta-1, got %v", storedAfterFirst)
	}

	if _, err := svc.SyncInbox(context.Background(), userID, accountID); err != nil {
		t.Fatal(err)
	}
	if got := len(graph.calls); got != 2 || graph.calls[1] != "delta-1" {
		t.Fatalf("expected second delta call with prior link, got %#v", graph.calls)
	}
	storedAfterSecond, err := repo.GetSyncDeltaLink(context.Background(), userID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if storedAfterSecond == nil || *storedAfterSecond != "delta-2" {
		t.Fatalf("expected stored delta-2, got %v", storedAfterSecond)
	}
}

func TestSyncInboxFailsAndDoesNotOverwriteDeltaLink(t *testing.T) {
	graph := &fakeMailbox{
		results: []*driven.MailChangePage{
			{
				Messages: []driven.MailMessage{
					{ID: "provider-1", Subject: "one", ReceivedDateTime: time.Now().UTC().Format(time.RFC3339), FromAddress: "a@example.com"},
				},
				FinalCursor: "delta-1",
			},
		},
	}
	svc, repo, userID, accountID := setupSyncService(t, graph)
	if _, err := svc.SyncInbox(context.Background(), userID, accountID); err != nil {
		t.Fatal(err)
	}

	graph.errByCall = map[int]error{1: errors.New("graph failure")}
	if _, err := svc.SyncInbox(context.Background(), userID, accountID); err == nil {
		t.Fatal("expected sync failure")
	}
	stored, err := repo.GetSyncDeltaLink(context.Background(), userID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || *stored != "delta-1" {
		t.Fatalf("expected old delta link to be preserved, got %v", stored)
	}
}

func TestSyncInboxResetsInvalidDeltaAndContinues(t *testing.T) {
	graph := &fakeMailbox{
		results: []*driven.MailChangePage{
			{
				Messages:    []driven.MailMessage{{ID: "provider-1", Subject: "one", ReceivedDateTime: time.Now().UTC().Format(time.RFC3339), FromAddress: "a@example.com"}},
				FinalCursor: "delta-1",
			},
			{
				Messages:    []driven.MailMessage{{ID: "provider-2", Subject: "two", ReceivedDateTime: time.Now().UTC().Format(time.RFC3339), FromAddress: "b@example.com"}},
				FinalCursor: "delta-2",
			},
		},
	}
	svc, repo, userID, accountID := setupSyncService(t, graph)
	if _, err := svc.SyncInbox(context.Background(), userID, accountID); err != nil {
		t.Fatal(err)
	}
	graph.errByCall = map[int]error{1: fmt.Errorf("%w: graph 410 Gone: SyncStateNotFound", driven.ErrCursorExpired)}

	res, err := svc.SyncInbox(context.Background(), userID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(graph.calls); got != 3 {
		t.Fatalf("expected fallback to bootstrap delta after invalid cursor; got %d calls (%#v)", got, graph.calls)
	}
	if graph.calls[1] != "delta-1" || graph.calls[2] != "" {
		t.Fatalf("unexpected delta sequence: %#v", graph.calls)
	}
	stored, err := repo.GetSyncDeltaLink(context.Background(), userID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || *stored != "delta-2" {
		t.Fatalf("expected updated delta-2 after fallback, got %v", stored)
	}
	run, err := repo.GetJobRun(context.Background(), userID, res.JobRunID)
	if err != nil {
		t.Fatal(err)
	}
	if run == nil {
		t.Fatal("expected job run")
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(run.MetaJSON), &meta); err != nil {
		t.Fatal(err)
	}
	if got, _ := meta["delta_reused"].(bool); got {
		t.Fatalf("expected delta_reused=false after fallback, got %+v", meta["delta_reused"])
	}
	if got, _ := meta["delta_reset_reason"].(string); got != "invalid_delta_link" {
		t.Fatalf("expected delta_reset_reason=invalid_delta_link, got %q", got)
	}
}

func TestSyncInboxWritesObservabilityMeta(t *testing.T) {
	graph := &fakeMailbox{
		results: []*driven.MailChangePage{
			{
				Messages: []driven.MailMessage{
					{ID: "provider-1", Subject: "one", ReceivedDateTime: time.Now().UTC().Format(time.RFC3339), FromAddress: "a@example.com"},
					{ID: "provider-2", Subject: "two", ReceivedDateTime: time.Now().UTC().Format(time.RFC3339), FromAddress: "b@example.com"},
				},
				FinalCursor: "delta-1",
			},
		},
	}
	svc, repo, userID, accountID := setupSyncService(t, graph)
	res, err := svc.SyncInbox(context.Background(), userID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	run, err := repo.GetJobRun(context.Background(), userID, res.JobRunID)
	if err != nil {
		t.Fatal(err)
	}
	if run == nil {
		t.Fatal("expected job run")
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(run.MetaJSON), &meta); err != nil {
		t.Fatal(err)
	}
	if got, _ := meta["fetched"].(float64); got != 2 {
		t.Fatalf("expected fetched=2, got %v", got)
	}
	if got, _ := meta["upserted"].(float64); got != 2 {
		t.Fatalf("expected upserted=2, got %v", got)
	}
	if got, _ := meta["delta_reused"].(bool); got {
		t.Fatalf("expected delta_reused=false on initial sync, got %v", got)
	}
}

// A Graph delta page carries tombstones for messages that left the folder, and
// can carry rows with only the properties that changed. Neither is a full
// message payload, and upserting one blanks the subject and sender of a row
// that already had both and redates it to now, so it floats to the top of the
// inbox looking like a freshly arrived message from "Unknown sender".
func TestSyncInboxDoesNotOverwriteAMessageWithAPartialDeltaRow(t *testing.T) {
	received := time.Now().UTC().Add(-72 * time.Hour)
	graph := &fakeMailbox{
		results: []*driven.MailChangePage{
			{
				Messages: []driven.MailMessage{{
					ID:               "provider-1",
					Subject:          "Your online bill",
					ReceivedDateTime: received.Format(time.RFC3339),
					FromName:         "Microsoft",
					FromAddress:      "billing@microsoft.com",
				}},
				FinalCursor: "delta-1",
			},
			{
				// A tombstone for the message synced above.
				Removed: []string{"provider-1"},
				Messages: []driven.MailMessage{
					// A changed-properties-only row: no timestamp.
					{ID: "provider-2"},
				},
				FinalCursor: "delta-2",
			},
		},
	}
	svc, repo, userID, accountID := setupSyncService(t, graph)
	ctx := context.Background()

	if _, err := svc.SyncInbox(ctx, userID, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SyncInbox(ctx, userID, accountID); err != nil {
		t.Fatal(err)
	}

	rows, err := repo.ListMessagesByAccount(ctx, userID, accountID, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The contentless rows must not have become messages of their own.
	if len(rows) != 1 {
		t.Fatalf("stored %d messages, want 1", len(rows))
	}
	got := rows[0]
	if got.Subject != "Your online bill" {
		t.Errorf("subject = %q, want %q", got.Subject, "Your online bill")
	}
	if !strings.Contains(got.FromJSON, "billing@microsoft.com") {
		t.Errorf("from_json = %q, want the original sender", got.FromJSON)
	}
	// Redating is what pushed these to the top of the inbox.
	if drift := got.ReceivedAt.Sub(received); drift > time.Minute || drift < -time.Minute {
		t.Errorf("received_at = %v, want it left at %v", got.ReceivedAt, received)
	}
}

// Graph only resends messages that changed, so an ordinary delta run can never
// repair a row whose stored content was lost locally. A forced run ignores the
// stored delta link and refetches full payloads.
func TestSyncInboxForceIgnoresTheStoredDeltaLink(t *testing.T) {
	graph := &fakeMailbox{
		results: []*driven.MailChangePage{
			{
				Messages: []driven.MailMessage{{
					ID: "provider-1", Subject: "one",
					ReceivedDateTime: time.Now().UTC().Format(time.RFC3339),
					FromAddress:      "a@example.com",
				}},
				FinalCursor: "delta-1",
			},
			{Messages: nil, FinalCursor: "delta-2"},
			{Messages: nil, FinalCursor: "delta-3"},
		},
	}
	svc, repo, userID, accountID := setupSyncService(t, graph)
	ctx := context.Background()

	if _, err := svc.SyncInbox(ctx, userID, accountID); err != nil {
		t.Fatal(err)
	}
	// An ordinary run resumes from the stored link.
	if _, err := svc.SyncInbox(ctx, userID, accountID); err != nil {
		t.Fatal(err)
	}
	if got := graph.calls[1]; got != "delta-1" {
		t.Fatalf("second call used %q, want the stored delta link", got)
	}

	if _, err := svc.SyncInboxWithOptions(ctx, userID, accountID, SyncOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	if got := graph.calls[2]; got != "" {
		t.Fatalf("forced call used %q, want an empty delta link", got)
	}
	// The forced run still stores the link it ends on, so the next ordinary
	// run resumes rather than refetching everything again.
	stored, err := repo.GetSyncDeltaLink(ctx, userID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || *stored != "delta-3" {
		t.Errorf("stored delta link = %v, want delta-3", stored)
	}
}

// The chunked job path is the one that runs in a deployed environment, and it
// has a second cursor to respect: a forced reset applies to the start of a
// sync, not to every page, or a multi-page run would restart on each chunk.
func TestSyncChunkForceResetsOnlyAtTheStart(t *testing.T) {
	graph := &fakeMailbox{
		results: []*driven.MailChangePage{
			{Messages: nil, FinalCursor: "delta-1"},
			{Messages: nil, FinalCursor: "delta-2"},
			{Messages: nil, FinalCursor: "delta-3"},
		},
	}
	svc, _, userID, accountID := setupSyncService(t, graph)
	ctx := context.Background()

	// Seed a stored delta link.
	if _, err := svc.SyncInbox(ctx, userID, accountID); err != nil {
		t.Fatal(err)
	}

	// No cursor yet and Force set: start from scratch.
	if _, err := svc.SyncChunk(ctx, driven.RunContext{
		UserID: userID, AccountID: &accountID, JobType: "sync",
		Payload: driven.JobPayload{Force: true},
	}); err != nil {
		t.Fatal(err)
	}
	if got := graph.calls[1]; got != "" {
		t.Fatalf("forced first chunk used %q, want an empty delta link", got)
	}

	// Mid-pagination the cursor wins, so the run advances instead of looping.
	if _, err := svc.SyncChunk(ctx, driven.RunContext{
		UserID: userID, AccountID: &accountID, JobType: "sync",
		Payload: driven.JobPayload{Force: true},
		Cursor:  &driven.JobCursor{Value: "page-2"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := graph.calls[2]; got != "page-2" {
		t.Fatalf("chunk with a cursor used %q, want page-2", got)
	}

	// Run detail is merged per chunk, so the last chunk is what the run
	// reports. A forced run has to stay identifiable through pagination.
	res, err := svc.SyncChunk(ctx, driven.RunContext{
		UserID: userID, AccountID: &accountID, JobType: "sync",
		Payload: driven.JobPayload{Force: true},
		Cursor:  &driven.JobCursor{Value: "page-3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.DeltaResetReason != "forced" {
		t.Errorf("delta reset reason = %q, want forced on every chunk of a forced run", res.DeltaResetReason)
	}
	if res.DeltaReused {
		t.Error("a forced run should not report the delta as reused")
	}
}

// recordingEnqueuer captures what sync queues as follow-up work.
type recordingEnqueuer struct {
	jobs []driven.CreateJobInput
}

func (r *recordingEnqueuer) Enqueue(ctx context.Context, in driven.CreateJobInput) (*driven.JobRecord, error) {
	r.jobs = append(r.jobs, in)
	return &driven.JobRecord{ID: uuid.New(), JobType: in.JobType}, nil
}

func (r *recordingEnqueuer) EnqueueChain(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, trigger string, chain []string, payload driven.JobPayload, scheduleID *uuid.UUID, scheduledFor *time.Time) (*driven.JobRecord, error) {
	return nil, errors.New("not implemented")
}

func (r *recordingEnqueuer) countOf(jobType string) int {
	n := 0
	for _, j := range r.jobs {
		if j.JobType == jobType {
			n++
		}
	}
	return n
}

// Contacts are extracted from synced mail by a job. Nothing enqueued it, and
// the chunked path — the only one a deployed worker runs — did not resolve
// inline either, so contacts were never extracted at all.
func TestSyncChunkQueuesContactResolutionOnceTheMailboxIsCurrent(t *testing.T) {
	graph := &fakeMailbox{
		results: []*driven.MailChangePage{
			{
				Messages: []driven.MailMessage{{
					ID: "provider-1", Subject: "one",
					ReceivedDateTime: time.Now().UTC().Format(time.RFC3339),
					FromAddress:      "a@example.com",
				}},
				NextCursor: "page-2",
			},
			{Messages: nil, FinalCursor: "delta-final"},
		},
	}
	svc, _, userID, accountID := setupSyncService(t, graph)
	enq := &recordingEnqueuer{}
	svc.ContactsEnqueuer = enq
	ctx := context.Background()

	// First chunk has more pages to fetch, so the mailbox is not current yet.
	first, err := svc.SyncChunk(ctx, driven.RunContext{UserID: userID, AccountID: &accountID, JobType: "sync"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Done {
		t.Fatal("expected the first chunk to report more pages")
	}
	if got := enq.countOf("resolve_contacts"); got != 0 {
		t.Errorf("queued resolution mid-pagination %d times, want 0", got)
	}

	// Final chunk: now it is worth reading the mailbox.
	last, err := svc.SyncChunk(ctx, driven.RunContext{
		UserID: userID, AccountID: &accountID, JobType: "sync",
		Cursor: &driven.JobCursor{Value: "page-2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !last.Done {
		t.Fatal("expected the last chunk to be done")
	}
	if got := enq.countOf("resolve_contacts"); got != 1 {
		t.Fatalf("queued resolution %d times, want exactly 1", got)
	}
	job := enq.jobs[0]
	if job.AccountID == nil || *job.AccountID != accountID {
		t.Errorf("job account = %v, want %s", job.AccountID, accountID)
	}
	if job.UserID != userID {
		t.Errorf("job user = %s, want %s", job.UserID, userID)
	}
}
