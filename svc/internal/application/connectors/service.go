package connectors

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

const oauthFlowSlackConnector = "slack_connector"

var (
	ErrInvalidOAuthState = errors.New("invalid oauth state")
	ErrNotFound          = errors.New("connector not found")
)

// Service orchestrates Slack connector OAuth, bindings, and synchronization.
type Service struct {
	Connectors driven.ConnectorRepository
	OAuthState driven.OAuthStateRepository
	Users      driven.UserRepository
	Projects   driven.ProjectRepository
	Slack      driven.SlackClient
	Vault      driven.TokenVault
	JobRuns    driven.JobRunRepository
}

type StartConnectInput struct {
	Provider  string
	LabelHint *string
}

type StartConnectOutput struct {
	AuthorizationURL string
	State            string
}

func (s *Service) StartConnect(ctx context.Context, userID uuid.UUID, in StartConnectInput) (*StartConnectOutput, error) {
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
	return &StartConnectOutput{AuthorizationURL: authorizationURL, State: state}, nil
}

type CompleteOAuthResult struct {
	ConnectorAccountID uuid.UUID
}

func (s *Service) CompleteOAuth(ctx context.Context, code, state string) (*CompleteOAuthResult, error) {
	flow, userID, payloadJSON, ok, err := s.OAuthState.TakeOAuthState(ctx, state)
	if err != nil {
		return nil, err
	}
	if !ok || flow != oauthFlowSlackConnector || userID == nil {
		return nil, ErrInvalidOAuthState
	}
	var statePayload struct {
		Label *string `json:"label"`
	}
	if err := json.Unmarshal([]byte(payloadJSON), &statePayload); err != nil {
		return nil, ErrInvalidOAuthState
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
	if err := s.Connectors.InsertConnectorAccount(ctx, driven.ConnectorAccountRow{
		ID: id, UserID: *userID, Provider: "slack", Label: label,
		ExternalTenantID: externalTenantID, ConnectionStatus: "connected",
		Scopes: oauthResult.Scopes, CreatedAt: now, UpdatedAt: now,
	}, cipher); err != nil {
		return nil, err
	}
	return &CompleteOAuthResult{ConnectorAccountID: id}, nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]driven.ConnectorAccountRow, error) {
	return s.Connectors.ListConnectorAccounts(ctx, userID)
}

func (s *Service) Disconnect(ctx context.Context, userID, connectorAccountID uuid.UUID) error {
	account, err := s.Connectors.GetConnectorAccount(ctx, userID, connectorAccountID)
	if err != nil {
		return err
	}
	if account == nil {
		return ErrNotFound
	}
	return s.Connectors.DeleteConnectorAccount(ctx, userID, connectorAccountID)
}

func (s *Service) ListBindings(ctx context.Context, userID, connectorAccountID uuid.UUID) ([]driven.ConnectorBindingRow, error) {
	account, err := s.Connectors.GetConnectorAccount(ctx, userID, connectorAccountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrNotFound
	}
	return s.Connectors.ListConnectorBindings(ctx, userID, connectorAccountID)
}

type CreateBindingInput struct {
	ExternalChannelID string
	OrganisationID    *uuid.UUID
	ProjectID         *uuid.UUID
	Label             string
}

func (s *Service) CreateBinding(ctx context.Context, userID, connectorAccountID uuid.UUID, in CreateBindingInput) (*driven.ConnectorBindingRow, error) {
	account, err := s.Connectors.GetConnectorAccount(ctx, userID, connectorAccountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrNotFound
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
			return nil, ErrNotFound
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
			return nil, ErrNotFound
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
	row := &driven.ConnectorBindingRow{
		ID: uuid.New(), ConnectorAccountID: connectorAccountID, OrganisationID: organisationID,
		ExternalChannelID: channelID, ProjectID: in.ProjectID, Label: label,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Connectors.CreateConnectorBinding(ctx, *row); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *Service) Sync(ctx context.Context, userID, connectorAccountID uuid.UUID) (*SyncResult, error) {
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
