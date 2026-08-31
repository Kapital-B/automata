package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/jobkit"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

type SyncResult struct {
	JobRunID         uuid.UUID
	MessagesUpserted int
}

type SyncOptions struct {
	RunID   *uuid.UUID
	Trigger string
}

type ConnectorSyncChunkResult struct {
	MessagesUpserted int
	NextCursor       *driven.JobCursor
	Done             bool
}

func (s *Service) SyncConnector(ctx context.Context, userID, connectorAccountID uuid.UUID) (*SyncResult, error) {
	return s.SyncConnectorWithOptions(ctx, userID, connectorAccountID, SyncOptions{})
}

func (s *Service) SyncConnectorChunk(ctx context.Context, run driven.RunContext) (*ConnectorSyncChunkResult, error) {
	if s == nil || s.Connectors == nil || s.Slack == nil || s.Vault == nil {
		return nil, fmt.Errorf("connector sync not configured")
	}
	connectorAccountID := uuid.Nil
	if run.Payload.ConnectorAccountID != nil {
		connectorAccountID = *run.Payload.ConnectorAccountID
	}
	if connectorAccountID == uuid.Nil {
		return nil, fmt.Errorf("connector_account_id is required")
	}
	account, err := s.Connectors.GetConnectorAccount(ctx, run.UserID, connectorAccountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrNotFound
	}
	if account.Provider != "slack" {
		return nil, fmt.Errorf("unsupported connector provider")
	}
	cipher, err := s.Connectors.GetConnectorTokenCipher(ctx, run.UserID, connectorAccountID)
	if err != nil {
		return nil, err
	}
	token, err := s.decryptToken(cipher)
	if err != nil {
		return nil, err
	}
	bindings, err := s.Connectors.ListConnectorBindings(ctx, run.UserID, connectorAccountID)
	if err != nil {
		return nil, err
	}
	offset := jobkit.DecodeOffsetCursor(run.Cursor)
	if offset >= len(bindings) {
		return &ConnectorSyncChunkResult{Done: true}, nil
	}
	binding := bindings[offset]
	pageCursor := ""
	if run.Cursor != nil && strings.Contains(run.Cursor.Value, "|") {
		parts := strings.SplitN(run.Cursor.Value, "|", 2)
		pageCursor = strings.TrimSpace(parts[1])
	}
	page, err := s.Slack.FetchHistory(ctx, token.AccessToken, binding.ExternalChannelID, pageCursor)
	if err != nil {
		return nil, err
	}
	upserted := 0
	now := time.Now().UTC()
	for _, message := range page.Messages {
		title := binding.Label
		if title == "" {
			title = binding.ExternalChannelID
		}
		messageMeta, _ := json.Marshal(map[string]any{
			"provider":   "slack",
			"channel_id": binding.ExternalChannelID,
		})
		if err := s.Connectors.UpsertConnectorMessage(ctx, driven.ConnectorMessageRow{
			ID:                 uuid.New(),
			ConnectorAccountID: connectorAccountID,
			OrganisationID:     binding.OrganisationID,
			ProjectID:          binding.ProjectID,
			ProviderEventID:    message.ProviderEventID,
			ExternalChannelID:  binding.ExternalChannelID,
			Title:              title,
			BodyText:           message.Text,
			AuthorLabel:        message.AuthorLabel,
			OccurredAt:         message.OccurredAt,
			MetaJSON:           string(messageMeta),
			CreatedAt:          now,
			UpdatedAt:          now,
		}); err != nil {
			return nil, err
		}
		upserted++
	}
	out := &ConnectorSyncChunkResult{MessagesUpserted: upserted}
	if strings.TrimSpace(page.NextCursor) != "" {
		out.NextCursor = &driven.JobCursor{Kind: "slack_history", Value: fmt.Sprintf("%d|%s", offset, strings.TrimSpace(page.NextCursor))}
		return out, nil
	}
	nextOffset := offset + 1
	if nextOffset >= len(bindings) {
		finishedAt := time.Now().UTC()
		if err := s.Connectors.UpdateConnectorToken(
			ctx, run.UserID, connectorAccountID, cipher, account.Scopes, "connected", nil, &finishedAt,
		); err != nil {
			return nil, err
		}
		out.Done = true
		return out, nil
	}
	out.NextCursor = &driven.JobCursor{Kind: "slack_history", Value: fmt.Sprintf("%d|", nextOffset)}
	return out, nil
}

func (s *Service) SyncConnectorWithOptions(
	ctx context.Context,
	userID, connectorAccountID uuid.UUID,
	options SyncOptions,
) (*SyncResult, error) {
	account, err := s.Connectors.GetConnectorAccount(ctx, userID, connectorAccountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrNotFound
	}
	if account.Provider != "slack" {
		return nil, fmt.Errorf("unsupported connector provider")
	}
	cipher, err := s.Connectors.GetConnectorTokenCipher(ctx, userID, connectorAccountID)
	if err != nil {
		return nil, err
	}
	token, err := s.decryptToken(cipher)
	if err != nil {
		return nil, err
	}

	runID := uuid.New()
	if options.RunID != nil {
		runID = *options.RunID
	}
	trigger := strings.TrimSpace(options.Trigger)
	if trigger == "" {
		trigger = "api"
	}
	startedAt := time.Now().UTC()
	meta := connectorSyncMeta(connectorAccountID, 0, false)
	if s.JobRuns != nil {
		if options.RunID != nil {
			if err := s.JobRuns.PromoteJobRunToRunning(ctx, runID, startedAt); err != nil {
				return nil, err
			}
		} else if err := s.JobRuns.InsertJobRun(
			ctx, runID, uuid.Nil, "sync_slack", trigger, "running",
			startedAt, time.Time{}, nil, meta,
		); err != nil {
			return nil, err
		}
	}

	bindings, err := s.Connectors.ListConnectorBindings(ctx, userID, connectorAccountID)
	if err != nil {
		s.failSync(ctx, runID, account, cipher, err)
		return nil, err
	}
	upserted := 0
	for _, binding := range bindings {
		cursor := ""
		if binding.SyncCursor != nil {
			cursor = strings.TrimSpace(*binding.SyncCursor)
		}
		for {
			page, err := s.Slack.FetchHistory(ctx, token.AccessToken, binding.ExternalChannelID, cursor)
			if err != nil {
				s.failSync(ctx, runID, account, cipher, err)
				return nil, err
			}
			now := time.Now().UTC()
			for _, message := range page.Messages {
				title := binding.Label
				if title == "" {
					title = binding.ExternalChannelID
				}
				messageMeta, _ := json.Marshal(map[string]any{
					"provider":   "slack",
					"channel_id": binding.ExternalChannelID,
				})
				if err := s.Connectors.UpsertConnectorMessage(ctx, driven.ConnectorMessageRow{
					ID: uuid.New(), ConnectorAccountID: connectorAccountID,
					OrganisationID: binding.OrganisationID, ProjectID: binding.ProjectID,
					ProviderEventID: message.ProviderEventID, ExternalChannelID: binding.ExternalChannelID,
					Title: title, BodyText: message.Text, AuthorLabel: message.AuthorLabel,
					OccurredAt: message.OccurredAt, MetaJSON: string(messageMeta),
					CreatedAt: now, UpdatedAt: now,
				}); err != nil {
					s.failSync(ctx, runID, account, cipher, err)
					return nil, err
				}
				upserted++
			}
			nextCursor := strings.TrimSpace(page.NextCursor)
			var next *string
			if nextCursor != "" {
				next = &nextCursor
			}
			if err := s.Connectors.UpdateConnectorBindingCursor(ctx, userID, binding.ID, next, now); err != nil {
				s.failSync(ctx, runID, account, cipher, err)
				return nil, err
			}
			if nextCursor == "" {
				break
			}
			cursor = nextCursor
		}
	}

	finishedAt := time.Now().UTC()
	if err := s.Connectors.UpdateConnectorToken(
		ctx, userID, connectorAccountID, cipher, account.Scopes, "connected", nil, &finishedAt,
	); err != nil {
		s.failSync(ctx, runID, account, cipher, err)
		return nil, err
	}
	meta = connectorSyncMeta(connectorAccountID, upserted, true)
	if s.JobRuns != nil {
		_ = s.JobRuns.UpdateJobRunStatus(ctx, runID, "success", &finishedAt, nil, meta)
	}
	return &SyncResult{JobRunID: runID, MessagesUpserted: upserted}, nil
}

func (s *Service) failSync(
	ctx context.Context,
	runID uuid.UUID,
	account *driven.ConnectorAccountRow,
	cipher []byte,
	syncErr error,
) {
	message := syncErr.Error()
	finishedAt := time.Now().UTC()
	_ = s.Connectors.UpdateConnectorToken(
		ctx, account.UserID, account.ID, cipher, account.Scopes, "error", &message, nil,
	)
	if s.JobRuns != nil {
		_ = s.JobRuns.UpdateJobRunStatus(
			ctx, runID, "failed", &finishedAt, &message,
			connectorSyncMeta(account.ID, 0, false),
		)
	}
}

func connectorSyncMeta(connectorAccountID uuid.UUID, messagesUpserted int, complete bool) string {
	raw, _ := json.Marshal(map[string]any{
		"connector_account_id": connectorAccountID.String(),
		"messages_upserted":    messagesUpserted,
		"complete":             complete,
	})
	return string(raw)
}
