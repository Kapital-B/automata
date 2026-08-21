package messages

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type fakeForwardGraph struct {
	forwardCalls int
	forwardErr   error
}

func (f *fakeForwardGraph) GetMe(ctx context.Context, accessToken string) (*driven.GraphProfile, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeForwardGraph) ListInboxMessages(ctx context.Context, accessToken string, top int) ([]driven.GraphMessage, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeForwardGraph) ListInboxDelta(ctx context.Context, accessToken string, deltaLink string, pageSize int) (*driven.GraphDeltaResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeForwardGraph) GetMessageBody(ctx context.Context, accessToken string, providerMessageID string) (*driven.GraphMessage, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeForwardGraph) ResolveGraphMessageID(ctx context.Context, accessToken string, providerMessageID string) (string, error) {
	return strings.TrimSpace(providerMessageID), nil
}
func (f *fakeForwardGraph) SendMail(ctx context.Context, accessToken string, toEmail, subject, body string) error {
	return errors.New("not implemented")
}
func (f *fakeForwardGraph) ReplyToMessage(ctx context.Context, accessToken string, providerMessageID string, body string) error {
	return errors.New("not implemented")
}
func (f *fakeForwardGraph) ForwardMessage(ctx context.Context, accessToken string, providerMessageID string, toEmail string, comment string) error {
	f.forwardCalls++
	return f.forwardErr
}

func setupForwardRulesService(t *testing.T, graph *fakeForwardGraph) (*sql.DB, *ForwardRulesService, *sqlite.Repository, uuid.UUID, uuid.UUID, uuid.UUID) {
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
	messageID := uuid.New()

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
	body := "Invoice attached"
	if err := repo.UpsertMessage(context.Background(), driven.MessageRow{
		ID:                messageID,
		AccountID:         accountID,
		ProviderMessageID: "provider-invoice-1",
		ReceivedAt:        time.Date(2026, 5, 1, 9, 12, 20, 0, time.UTC),
		Subject:           "Invoice - INV0018315",
		FromJSON:          `{"name":"Vendor","address":"billing@example.com"}`,
		BodyText:          &body,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	invoiceDef, err := repo.GetCategoryDefinitionBySlug(context.Background(), userID, "invoice")
	if err != nil {
		t.Fatal(err)
	}
	if invoiceDef == nil {
		def := driven.CategoryDefinitionRow{
			ID:          uuid.New(),
			UserID:      userID,
			Slug:        "invoice",
			DisplayName: "Invoice",
			Definition:  "Invoices and billing",
			SortOrder:   99,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}
		if err := repo.CreateCategoryDefinition(context.Background(), def); err != nil {
			t.Fatal(err)
		}
		invoiceDef = &def
	}
	conf := 0.98
	if err := repo.UpsertMessageCategory(context.Background(), driven.MessageCategoryRow{
		ID:         uuid.New(),
		MessageID:  messageID,
		AccountID:  accountID,
		CategoryID: invoiceDef.ID,
		Source:     "llm",
		Confidence: &conf,
		RunID:      uuid.New(),
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	forwardTo := "bills@example.com"
	if err := repo.ReplaceForwardAllowlist(context.Background(), userID, []string{forwardTo}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateForwardRule(context.Background(), driven.ForwardRuleRow{
		ID:            uuid.New(),
		UserID:        userID,
		AccountID:     accountID,
		Name:          "Invoice forward",
		Mode:          "logic",
		ConditionJSON: `{"all":[{"field":"category_slug","op":"equals","value":"invoice"}]}`,
		ForwardTo:     forwardTo,
		Enabled:       true,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	svc := &ForwardRulesService{
		Messages: repo,
		Forwards: repo,
		Accounts: repo,
		OAuth:    &fakeSyncOAuth{},
		Graph:    graph,
		Vault:    &passthroughVault{},
		JobRuns:  repo,
	}
	return db, svc, repo, userID, accountID, messageID
}

func TestForwardRulesUsesForwardSeenMarkerNotReceivedAt(t *testing.T) {
	db, svc, repo, userID, accountID, messageID := setupForwardRulesService(t, &fakeForwardGraph{})
	_, err := svc.RunAccount(context.Background(), userID, accountID, ForwardRulesOptions{
		Trigger: "schedule",
		Since:   ptrTime(time.Date(2026, 5, 1, 10, 17, 48, 0, time.UTC)),
	})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := repo.GetMessage(context.Background(), userID, messageID)
	if err != nil {
		t.Fatal(err)
	}
	if msg == nil || msg.ForwardSeenAt == nil {
		t.Fatalf("expected forward_seen_at to be set, got %+v", msg)
	}
	var forwarded int
	if err := db.QueryRow(`SELECT COUNT(1) FROM forward_audit WHERE message_id = ? AND status = 'forwarded'`, messageID.String()).Scan(&forwarded); err != nil {
		t.Fatal(err)
	}
	if forwarded != 1 {
		t.Fatalf("expected one forwarded audit row, got %d", forwarded)
	}
}

func TestForwardRulesSecondRunDoesNotCallGraphAgain(t *testing.T) {
	graph := &fakeForwardGraph{}
	_, svc, _, userID, accountID, _ := setupForwardRulesService(t, graph)
	_, err := svc.RunAccount(context.Background(), userID, accountID, ForwardRulesOptions{Trigger: "schedule"})
	if err != nil {
		t.Fatal(err)
	}
	if graph.forwardCalls != 1 {
		t.Fatalf("want 1 forward call after first run, got %d", graph.forwardCalls)
	}
	_, err = svc.RunAccount(context.Background(), userID, accountID, ForwardRulesOptions{Trigger: "schedule"})
	if err != nil {
		t.Fatal(err)
	}
	if graph.forwardCalls != 1 {
		t.Fatalf("want 1 forward call total after second run (message already seen), got %d", graph.forwardCalls)
	}
}

func TestForwardRulesSkippedAuditWhenRuleDoesNotMatch(t *testing.T) {
	graph := &fakeForwardGraph{}
	db, svc, repo, userID, accountID, messageID := setupForwardRulesService(t, graph)
	ruleNoMatchID := uuid.New()
	forwardTo := "bills@example.com"
	if err := repo.CreateForwardRule(context.Background(), driven.ForwardRuleRow{
		ID:            ruleNoMatchID,
		UserID:        userID,
		AccountID:     accountID,
		Name:          "Newsletter only",
		Mode:          "logic",
		ConditionJSON: `{"all":[{"field":"category_slug","op":"equals","value":"newsletter"}]}`,
		ForwardTo:     forwardTo,
		Enabled:       true,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.RunAccount(context.Background(), userID, accountID, ForwardRulesOptions{Trigger: "schedule"})
	if err != nil {
		t.Fatal(err)
	}
	if graph.forwardCalls != 1 {
		t.Fatalf("expected exactly one Graph forward, got %d", graph.forwardCalls)
	}
	var skipped, forwarded int
	if err := db.QueryRow(`SELECT COUNT(1) FROM forward_audit WHERE message_id = ? AND status = 'skipped' AND rule_id = ?`,
		messageID.String(), ruleNoMatchID.String()).Scan(&skipped); err != nil {
		t.Fatal(err)
	}
	if skipped != 1 {
		t.Fatalf("want 1 skipped audit for non-matching rule, got %d", skipped)
	}
	if err := db.QueryRow(`SELECT COUNT(1) FROM forward_audit WHERE message_id = ? AND status = 'forwarded'`,
		messageID.String()).Scan(&forwarded); err != nil {
		t.Fatal(err)
	}
	if forwarded != 1 {
		t.Fatalf("want 1 forwarded audit, got %d", forwarded)
	}
}

func TestForwardRulesKeepsMessageUnseenWhenForwardFails(t *testing.T) {
	db, svc, repo, userID, accountID, messageID := setupForwardRulesService(t, &fakeForwardGraph{forwardErr: errors.New("graph failed")})
	_, err := svc.RunAccount(context.Background(), userID, accountID, ForwardRulesOptions{Trigger: "schedule"})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := repo.GetMessage(context.Background(), userID, messageID)
	if err != nil {
		t.Fatal(err)
	}
	if msg == nil {
		t.Fatal("expected message")
	}
	if msg.ForwardSeenAt != nil {
		t.Fatalf("expected forward_seen_at to remain nil on forward failure, got %v", *msg.ForwardSeenAt)
	}
	rows, err := repo.ListMessages(context.Background(), userID, driven.MessageListFilter{
		AccountID:         &accountID,
		OnlyForwardUnseen: true,
		Limit:             50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected message to remain in forward-unseen set, got %d rows", len(rows))
	}
	var failed int
	if err := db.QueryRow(`SELECT COUNT(1) FROM forward_audit WHERE message_id = ? AND status = 'failed'`, messageID.String()).Scan(&failed); err != nil {
		t.Fatal(err)
	}
	if failed == 0 {
		t.Fatal("expected failed forward audit row")
	}
}
