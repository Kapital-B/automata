package messages

import (
	"context"
	"encoding/json"
	"fmt"
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

func (s *SyncService) SyncInbox(ctx context.Context, userID uuid.UUID, accountID uuid.UUID) (*SyncResult, error) {
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
	started := time.Now().UTC()

	list, err := s.Graph.ListInboxMessages(ctx, tok.AccessToken, 50)
	if err != nil {
		if s.JobRuns != nil {
			msg := err.Error()
			_ = s.JobRuns.InsertJobRun(ctx, jobID, accountID, "sync", "api", "failed", started, time.Now().UTC(), &msg, `{}`)
		}
		return nil, err
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
				_ = s.JobRuns.InsertJobRun(ctx, jobID, accountID, "sync", "api", "failed", started, time.Now().UTC(), &em, `{}`)
			}
			return nil, err
		}
		n++
	}
	if err := s.Accounts.UpsertSyncStateTime(ctx, userID, accountID, time.Now().UTC()); err != nil {
		return nil, err
	}
	finished := time.Now().UTC()
	if s.JobRuns != nil {
		meta := fmt.Sprintf(`{"messages_upserted":%d}`, n)
		_ = s.JobRuns.InsertJobRun(ctx, jobID, accountID, "sync", "api", "success", started, finished, nil, meta)
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

func normalizeBody(content, contentType string) string {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "html") {
		return stripHTML(content)
	}
	return content
}

func stripHTML(s string) string {
	// minimal: drop tags for Phase 1 preview
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return s
	}
	return out
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
