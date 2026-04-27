package messages

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

// SyncService pulls inbox messages into the store for one account.
type SyncService struct {
	Accounts driven.AccountRepository
	Messages driven.MessageRepository
	OAuth    driven.MicrosoftOAuth
	Graph    driven.MicrosoftGraph
	Vault    driven.TokenVault
	JobRuns  driven.JobRunRepository
}

type SyncResult struct {
	JobRunID           uuid.UUID
	MessagesUpserted   int
}

type SyncOptions struct {
	RunID   *uuid.UUID
	Trigger string
}

func (s *SyncService) SyncInbox(ctx context.Context, userID uuid.UUID, accountID uuid.UUID) (*SyncResult, error) {
	return s.SyncInboxWithOptions(ctx, userID, accountID, SyncOptions{})
}

func (s *SyncService) SyncInboxWithOptions(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, opts SyncOptions) (*SyncResult, error) {
	row, cipher, err := s.Accounts.GetAccount(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("account not found")
	}
	if len(cipher) == 0 {
		return nil, fmt.Errorf("no tokens for account")
	}
	raw, err := s.Vault.Decrypt(cipher)
	if err != nil {
		return nil, err
	}
	kind, refresh, err := appaccounts.DecodeRefreshPayload(raw)
	if err != nil {
		return nil, err
	}
	tok, err := s.OAuth.RefreshAccessToken(ctx, kind, refresh)
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	newRefresh := refresh
	if tok.RefreshToken != "" {
		newRefresh = tok.RefreshToken
	}
	payload, err := appaccounts.EncodeRefreshPayloadForStorage(kind, newRefresh)
	if err != nil {
		return nil, err
	}
	newCipher, err := s.Vault.Encrypt(payload)
	if err != nil {
		return nil, err
	}
	if err := s.Accounts.UpdateAccountTokens(ctx, userID, accountID, newCipher, row.PrimaryEmail, row.GraphTenantID, row.MsalHomeAccountID, "connected", nil); err != nil {
		return nil, err
	}

	jobID := uuid.New()
	if opts.RunID != nil {
		jobID = *opts.RunID
	}
	trigger := strings.TrimSpace(opts.Trigger)
	if trigger == "" {
		trigger = "api"
	}
	started := time.Now().UTC()
	if s.JobRuns != nil {
		_ = s.JobRuns.InsertJobRun(ctx, jobID, accountID, "sync", trigger, "running", started, time.Time{}, nil, `{}`)
	}

	prevDeltaLink, err := s.Accounts.GetSyncDeltaLink(ctx, userID, accountID)
	if err != nil {
		if s.JobRuns != nil {
			msg := err.Error()
			_ = s.JobRuns.UpdateJobRunStatus(ctx, jobID, "failed", timePtrSync(time.Now().UTC()), &msg, `{}`)
		}
		return nil, err
	}
	deltaRes, err := s.Graph.ListInboxDelta(ctx, tok.AccessToken, strOrEmpty(prevDeltaLink), 50)
	if err != nil {
		if s.JobRuns != nil {
			msg := err.Error()
			_ = s.JobRuns.UpdateJobRunStatus(ctx, jobID, "failed", timePtrSync(time.Now().UTC()), &msg, `{}`)
		}
		return nil, err
	}
	list := deltaRes.Messages
	if s.JobRuns != nil {
		_ = s.JobRuns.UpdateJobRunMeta(ctx, jobID, fmt.Sprintf(`{"total_messages":%d,"processed_messages":0,"messages_upserted":0}`, len(list)))
	}

	n := 0
	for _, gm := range list {
		rt, err := parseGraphTime(gm.ReceivedDateTime)
		if err != nil {
			rt = time.Now().UTC()
		}
		body := gm.BodyPreview
		var bodyFetched *time.Time
		if gm.BodyContent != "" {
			body = normalizeBody(gm.BodyContent, gm.BodyContentType)
			t := time.Now().UTC()
			bodyFetched = &t
		} else if gm.BodyPreview != "" {
			body = gm.BodyPreview
		}
		fromObj := map[string]string{"name": gm.FromName, "address": gm.FromAddress}
		fromJSON, _ := json.Marshal(fromObj)
		conv := gm.ConversationID
		etag := gm.ChangeKey
		m := driven.MessageRow{
			ID:                uuid.New(),
			AccountID:         accountID,
			ProviderMessageID: gm.ID,
			ConversationID:    nullIfEmpty(conv),
			ReceivedAt:        rt,
			Subject:           gm.Subject,
			FromJSON:          string(fromJSON),
			BodyText:          nullIfEmpty(body),
			BodyFetchedAt:     bodyFetched,
			HasAttachments:    gm.HasAttachments,
			RawEtag:           nullIfEmpty(etag),
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
		}
		if err := s.Messages.UpsertMessage(ctx, m); err != nil {
			if s.JobRuns != nil {
				em := err.Error()
				_ = s.JobRuns.UpdateJobRunStatus(ctx, jobID, "failed", timePtrSync(time.Now().UTC()), &em, `{}`)
			}
			return nil, err
		}
		n++
		if s.JobRuns != nil {
			_ = s.JobRuns.UpdateJobRunMeta(ctx, jobID, fmt.Sprintf(`{"total_messages":%d,"processed_messages":%d,"messages_upserted":%d}`, len(list), n, n))
		}
	}
	if err := s.Accounts.UpsertSyncState(ctx, userID, accountID, &deltaRes.DeltaLink, time.Now().UTC()); err != nil {
		return nil, err
	}
	finished := time.Now().UTC()
	if s.JobRuns != nil {
		meta := fmt.Sprintf(`{"messages_upserted":%d}`, n)
		_ = s.JobRuns.UpdateJobRunStatus(ctx, jobID, "success", timePtrSync(finished), nil, meta)
	}
	return &SyncResult{JobRunID: jobID, MessagesUpserted: n}, nil
}

func parseGraphTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	return time.Parse(time.RFC3339, s)
}

func normalizeBody(content, _ string) string {
	return content
}

func looksLikeHTML(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "<!doctype") ||
		strings.Contains(lower, "<html") ||
		strings.Contains(lower, "<head") ||
		strings.Contains(lower, "<body") ||
		strings.Contains(lower, "<div") ||
		strings.Contains(lower, "<span") ||
		strings.Contains(lower, "<p") ||
		strings.Contains(lower, "<a ") ||
		strings.Contains(lower, "<img") ||
		strings.Contains(lower, "<ul") ||
		strings.Contains(lower, "<li") ||
		strings.Contains(lower, "<table") ||
		strings.Contains(lower, "<br")
}

func stripHTML(s string) string {
	// Minimal HTML fallback for previews and LLM prompts.
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			if b.Len() > 0 {
				b.WriteRune(' ')
			}
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return s
	}
	return html.UnescapeString(out)
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func strOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func timePtrSync(t time.Time) *time.Time {
	return &t
}
