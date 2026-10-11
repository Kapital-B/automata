package connectors

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"

	"fmt"
	"strings"
	"time"

	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
	domainconnectors "github.com/Kapital-B/automata/svc/internal/domain/connectors"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	domainshared "github.com/Kapital-B/automata/svc/internal/domain/shared"
	"github.com/google/uuid"
)

const oauthFlowSlackConnector = "slack_connector"

// Service orchestrates Slack connector OAuth, bindings, and synchronization.
type Service struct {
	Connectors domainconnectors.ConnectorRepository
	OAuthState domainauth.OAuthStateRepository
	Users      domainauth.UserRepository
	Projects   domainprojects.ProjectRepository
	Slack      domainconnectors.SlackClient
	Vault      domainshared.TokenVault
	JobRuns    domainjobs.JobRunRepository
}

func (s *Service) StartConnect(ctx context.Context, userID uuid.UUID, in domainconnectors.ConnectorStartConnectInput) (*domainconnectors.ConnectorStartConnectOutput, error) {
	if s == nil || s.Slack == nil || s.OAuthState == nil {
		return nil, fmt.Errorf("Slack connector is not configured")
	}
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if provider != "" && provider != "slack" {
		return nil, fmt.Errorf("unsupported connector provider")
	}
	state, err := randomState()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{"label": in.LabelHint})
	if err != nil {
		return nil, err
	}
	if err := s.OAuthState.InsertOAuthState(ctx, state, oauthFlowSlackConnector, &userID, string(payload), time.Now().UTC()); err != nil {
		return nil, err
	}
	authorizationURL, err := s.Slack.AuthorizationURL(ctx, state)
	if err != nil {
		return nil, err
	}
	return &domainconnectors.ConnectorStartConnectOutput{AuthorizationURL: authorizationURL, State: state}, nil
}

func (s *Service) CompleteOAuth(ctx context.Context, code, state string) (*domainconnectors.ConnectorCompleteOAuthResult, error) {
	flow, userID, payloadJSON, ok, err := s.OAuthState.TakeOAuthState(ctx, state)
	if err != nil {
		return nil, err
	}
	if !ok || flow != oauthFlowSlackConnector || userID == nil {
		return nil, domainconnectors.ConnectorErrInvalidOAuthState
	}
	var statePayload struct {
		Label *string `json:"label"`
	}
	if err := json.Unmarshal([]byte(payloadJSON), &statePayload); err != nil {
		return nil, domainconnectors.ConnectorErrInvalidOAuthState
	}
	oauthResult, err := s.Slack.ExchangeCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange Slack code: %w", err)
	}
	tokenJSON, err := json.Marshal(slackTokenPayload{
		AccessToken: oauthResult.AccessToken, RefreshToken: oauthResult.RefreshToken,
	})
	if err != nil {
		return nil, err
	}
	cipher, err := s.Vault.Encrypt(tokenJSON)
	if err != nil {
		return nil, err
	}
	label := strings.TrimSpace(oauthResult.TeamName)
	if statePayload.Label != nil && strings.TrimSpace(*statePayload.Label) != "" {
		label = strings.TrimSpace(*statePayload.Label)
	}
	if label == "" {
		label = "Slack"
	}
	tenantID := strings.TrimSpace(oauthResult.TeamID)
	var externalTenantID *string
	if tenantID != "" {
		externalTenantID = &tenantID
	}
	now := time.Now().UTC()
	id := uuid.New()
	if err := s.Connectors.InsertConnectorAccount(ctx, domainconnectors.ConnectorAccountRow{
		ID: id, UserID: *userID, Provider: "slack", Label: label,
		ExternalTenantID: externalTenantID, ConnectionStatus: "connected",
		Scopes: oauthResult.Scopes, CreatedAt: now, UpdatedAt: now,
	}, cipher); err != nil {
		return nil, err
	}
	return &domainconnectors.ConnectorCompleteOAuthResult{ConnectorAccountID: id}, nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]domainconnectors.ConnectorAccountRow, error) {
	return s.Connectors.ListConnectorAccounts(ctx, userID)
}

func (s *Service) Disconnect(ctx context.Context, userID, connectorAccountID uuid.UUID) error {
	account, err := s.Connectors.GetConnectorAccount(ctx, userID, connectorAccountID)
	if err != nil {
		return err
	}
	if account == nil {
		return domainconnectors.ConnectorErrNotFound
	}
	return s.Connectors.DeleteConnectorAccount(ctx, userID, connectorAccountID)
}

func (s *Service) ListBindings(ctx context.Context, userID, connectorAccountID uuid.UUID) ([]domainconnectors.ConnectorBindingRow, error) {
	account, err := s.Connectors.GetConnectorAccount(ctx, userID, connectorAccountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, domainconnectors.ConnectorErrNotFound
	}
	return s.Connectors.ListConnectorBindings(ctx, userID, connectorAccountID)
}

func (s *Service) CreateBinding(ctx context.Context, userID, connectorAccountID uuid.UUID, in domainconnectors.ConnectorCreateBindingInput) (*domainconnectors.ConnectorBindingRow, error) {
	account, err := s.Connectors.GetConnectorAccount(ctx, userID, connectorAccountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, domainconnectors.ConnectorErrNotFound
	}
	channelID := strings.TrimSpace(in.ExternalChannelID)
	if channelID == "" {
		return nil, fmt.Errorf("external_channel_id is required")
	}
	homeOrgID, err := s.Users.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		return nil, err
	}
	organisationID := homeOrgID
	if in.OrganisationID != nil {
		if *in.OrganisationID != homeOrgID {
			return nil, domainconnectors.ConnectorErrNotFound
		}
		organisationID = *in.OrganisationID
	}
	if in.ProjectID != nil {
		project, err := s.Projects.GetProject(ctx, organisationID, *in.ProjectID)
		if err != nil {
			return nil, err
		}
		member, memberErr := s.Projects.GetProjectMember(ctx, *in.ProjectID, userID)
		if memberErr != nil {
			return nil, memberErr
		}
		if project == nil || project.ArchivedAt != nil || member == nil {
			return nil, domainconnectors.ConnectorErrNotFound
		}
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = channelID
		if cipher, err := s.Connectors.GetConnectorTokenCipher(ctx, userID, connectorAccountID); err == nil {
			if token, err := s.decryptToken(cipher); err == nil {
				if channels, err := s.Slack.ListConversations(ctx, token.AccessToken); err == nil {
					for _, channel := range channels {
						if channel.ID == channelID && strings.TrimSpace(channel.Name) != "" {
							label = channel.Name
							break
						}
					}
				}
			}
		}
	}
	now := time.Now().UTC()
	row := &domainconnectors.ConnectorBindingRow{
		ID: uuid.New(), ConnectorAccountID: connectorAccountID, OrganisationID: organisationID,
		ExternalChannelID: channelID, ProjectID: in.ProjectID, Label: label,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Connectors.CreateConnectorBinding(ctx, *row); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *Service) Sync(ctx context.Context, userID, connectorAccountID uuid.UUID) (*domainconnectors.ConnectorSyncResult, error) {
	return s.SyncConnector(ctx, userID, connectorAccountID)
}

type slackTokenPayload struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (s *Service) decryptToken(cipher []byte) (*slackTokenPayload, error) {
	if len(cipher) == 0 {
		return nil, fmt.Errorf("connector has no token")
	}
	plaintext, err := s.Vault.Decrypt(cipher)
	if err != nil {
		return nil, err
	}
	var token slackTokenPayload
	if err := json.Unmarshal(plaintext, &token); err != nil {
		return nil, err
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, fmt.Errorf("connector token missing access_token")
	}
	return &token, nil
}

func randomState() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
