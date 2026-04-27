package messages

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type fakeSyncOAuth struct{}

func (f *fakeSyncOAuth) AuthorizationURL(ctx context.Context, kind domainacc.MsAccountKind, state string) (string, error) {
	return "", errors.New("not implemented")
}
func (f *fakeSyncOAuth) ExchangeCode(ctx context.Context, kind domainacc.MsAccountKind, code string) (driven.TokenPair, error) {
	return driven.TokenPair{}, errors.New("not implemented")
}
func (f *fakeSyncOAuth) RefreshAccessToken(ctx context.Context, kind domainacc.MsAccountKind, refreshToken string) (driven.TokenPair, error) {
	return driven.TokenPair{AccessToken: "access-token", RefreshToken: "refresh-next", ExpiresIn: 3600}, nil
}

type passthroughVault struct{}

func (v *passthroughVault) Encrypt(plaintext []byte) ([]byte, error) { return plaintext, nil }
func (v *passthroughVault) Decrypt(ciphertext []byte) ([]byte, error) { return ciphertext, nil }

type fakeDeltaGraph struct {
	calls      []string
	results    []*driven.GraphDeltaResult
	err        error
	resultIdx  int
}

func (f *fakeDeltaGraph) GetMe(ctx context.Context, accessToken string) (*driven.GraphProfile, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeDeltaGraph) ListInboxMessages(ctx context.Context, accessToken string, top int) ([]driven.GraphMessage, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeDeltaGraph) ListInboxDelta(ctx context.Context, accessToken string, deltaLink string, pageSize int) (*driven.GraphDeltaResult, error) {
	f.calls = append(f.calls, deltaLink)
	if f.err != nil {
		return nil, f.err
	}
	if f.resultIdx >= len(f.results) {
		return &driven.GraphDeltaResult{Messages: nil, DeltaLink: "delta-empty"}, nil
	}
	out := f.results[f.resultIdx]
	f.resultIdx++
	return out, nil
}
func (f *fakeDeltaGraph) GetMessageBody(ctx context.Context, accessToken string, providerMessageID string) (*driven.GraphMessage, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeDeltaGraph) SendMail(ctx context.Context, accessToken string, toEmail, subject, body string) error {
	return errors.New("not implemented")
}
func (f *fakeDeltaGraph) ForwardMessage(ctx context.Context, accessToken string, providerMessageID string, toEmail string, comment string) error {
	return errors.New("not implemented")
}

func setupSyncService(t *testing.T, graph *fakeDeltaGraph) (*SyncService, *sqlite.Repository, uuid.UUID, uuid.UUID) {
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
	payload, err := appaccounts.EncodeRefreshPayloadForStorage(domainacc.KindWork, "refresh-initial")
	if err != nil {
		t.Fatal(err)
	}
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
		Accounts: repo,
		Messages: repo,
		OAuth:    &fakeSyncOAuth{},
		Graph:    graph,
		Vault:    &passthroughVault{},
		JobRuns:  repo,
	}
	return svc, repo, userID, accountID
}

func TestSyncInboxUsesDeltaLinkAcrossRuns(t *testing.T) {
	graph := &fakeDeltaGraph{
		results: []*driven.GraphDeltaResult{
			{
				Messages: []driven.GraphMessage{
					{ID: "provider-1", Subject: "one", ReceivedDateTime: time.Now().UTC().Format(time.RFC3339), FromAddress: "a@example.com"},
				},
				DeltaLink: "delta-1",
			},
			{
				Messages: []driven.GraphMessage{
					{ID: "provider-2", Subject: "two", ReceivedDateTime: time.Now().UTC().Format(time.RFC3339), FromAddress: "b@example.com"},
				},
				DeltaLink: "delta-2",
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
	graph := &fakeDeltaGraph{
		results: []*driven.GraphDeltaResult{
			{
				Messages: []driven.GraphMessage{
					{ID: "provider-1", Subject: "one", ReceivedDateTime: time.Now().UTC().Format(time.RFC3339), FromAddress: "a@example.com"},
				},
				DeltaLink: "delta-1",
			},
		},
	}
	svc, repo, userID, accountID := setupSyncService(t, graph)
	if _, err := svc.SyncInbox(context.Background(), userID, accountID); err != nil {
		t.Fatal(err)
	}

	graph.err = errors.New("graph failure")
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
