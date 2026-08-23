package connectors_test

import (
	"context"
	"database/sql"
	"net/url"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
	slackadapter "github.com/Kapital-B/automata/svc/internal/adapters/outbound/slack"
	"github.com/Kapital-B/automata/svc/internal/application/connectors"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestFakeSlackConnectBindSyncTimelineIdempotent(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+uuid.NewString()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	userID := uuid.New()
	if _, err := repo.CreateUserWithHomeOrg(
		ctx, userID, "slack@example.test", nil, time.Now().UTC(),
		"password", userID.String(), "slack@example.test",
	); err != nil {
		t.Fatal(err)
	}
	projectService := &appprojects.Service{
		Users: repo, Projects: repo, Assignments: repo, Manuals: repo,
		Timeline: repo, Contacts: repo, Messages: repo,
	}
	project, err := projectService.Create(ctx, userID, appprojects.CreateProjectInput{
		Name: "Data Centre 01", Code: "DC01",
	})
	if err != nil {
		t.Fatal(err)
	}
	vault, err := security.NewAESGCMVault([]byte("12345678901234567890123456789012"))
	if err != nil {
		t.Fatal(err)
	}
	slackClient := &slackadapter.Client{
		Mode: "fake", RedirectURI: "http://localhost:8080/api/connectors/callback",
	}
	service := &connectors.Service{
		Connectors: repo, OAuthState: repo, Users: repo, Projects: repo,
		Slack: slackClient, Vault: vault, JobRuns: repo,
	}

	start, err := service.StartConnect(ctx, userID, connectors.StartConnectInput{Provider: "slack"})
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, err := url.Parse(start.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	connected, err := service.CompleteOAuth(
		ctx, authorizationURL.Query().Get("code"), authorizationURL.Query().Get("state"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateBinding(ctx, userID, connected.ConnectorAccountID, connectors.CreateBindingInput{
		ExternalChannelID: "C_FAKE_DC01", ProjectID: &project.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Sync(ctx, userID, connected.ConnectorAccountID); err != nil {
		t.Fatal(err)
	}
	first := slackTimeline(t, ctx, repo, userID, project.OrganisationID, project.ID)
	if len(first) != 2 {
		t.Fatalf("first Slack timeline count = %d, want 2", len(first))
	}
	firstIDs := map[uuid.UUID]struct{}{}
	for _, item := range first {
		if item.ConnectorMessageID == nil || item.ConnectorAccountID == nil {
			t.Fatalf("Slack item missing connector ids: %+v", item)
		}
		firstIDs[*item.ConnectorMessageID] = struct{}{}
	}

	if _, err := service.Sync(ctx, userID, connected.ConnectorAccountID); err != nil {
		t.Fatal(err)
	}
	second := slackTimeline(t, ctx, repo, userID, project.OrganisationID, project.ID)
	if len(second) != 2 {
		t.Fatalf("second Slack timeline count = %d, want 2", len(second))
	}
	for _, item := range second {
		if item.ConnectorMessageID == nil {
			t.Fatal("Slack item missing connector_message_id")
		}
		if _, ok := firstIDs[*item.ConnectorMessageID]; !ok {
			t.Fatalf("re-sync changed persisted connector message id: %s", item.ConnectorMessageID)
		}
	}
}

func slackTimeline(
	t *testing.T,
	ctx context.Context,
	repo *sqlite.Repository,
	userID, organisationID, projectID uuid.UUID,
) []driven.TimelineItem {
	t.Helper()
	items, err := repo.ListProjectTimeline(ctx, userID, organisationID, projectID, driven.TimelineFilter{
		Source: "slack",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Source != "slack" {
			t.Fatalf("unexpected timeline source %q", item.Source)
		}
	}
	return items
}
