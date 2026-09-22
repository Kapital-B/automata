package messages

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
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
	Resolve  interface {
		ResolveAfterSync(ctx context.Context, userID, accountID uuid.UUID, providerMessageIDs []string) error
		BackfillAccount(ctx context.Context, userID, accountID uuid.UUID) error
	}
	Assign interface {
		AssignAfterSync(ctx context.Context, userID, accountID uuid.UUID) error
	}
	// AssignEnqueuer, when set, takes assignment off the sync request path:
	// scoring a mailbox is real work and sync latency should not include it.
	// Falls back to the inline Assign hook when nil.
	AssignEnqueuer driven.JobEnqueuer
}

type SyncResult struct {
	JobRunID         uuid.UUID
	MessagesUpserted int
}

type SyncOptions struct {
	RunID   *uuid.UUID
	Trigger string
	// Force starts from an empty delta link instead of the stored one. Graph
	// only resends messages that changed, so a row whose content was lost
	// locally is never repaired by an ordinary run; a forced one refetches the
	// full payloads.
	Force bool
}

type SyncChunkResult struct {
	MessagesUpserted int
	Fetched          int
	DeltaReused      bool
	DeltaResetReason string
	NextCursor       *driven.JobCursor
	Done             bool
}

func (s *SyncService) SyncInbox(ctx context.Context, userID uuid.UUID, accountID uuid.UUID) (*SyncResult, error) {
	return s.SyncInboxWithOptions(ctx, userID, accountID, SyncOptions{})
}

func (s *SyncService) SyncChunk(ctx context.Context, run driven.RunContext) (*SyncChunkResult, error) {
	if s == nil || s.Accounts == nil || s.Messages == nil || s.OAuth == nil || s.Graph == nil || s.Vault == nil {
		return nil, fmt.Errorf("sync service not configured")
	}
	if run.AccountID == nil || *run.AccountID == uuid.Nil {
		return nil, fmt.Errorf("account_id is required")
	}
	accountID := *run.AccountID
	row, cipher, err := s.Accounts.GetAccount(ctx, run.UserID, accountID)
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
	if err := s.Accounts.UpdateAccountTokens(ctx, run.UserID, accountID, newCipher, row.PrimaryEmail, row.GraphTenantID, row.MsalHomeAccountID, "connected", nil); err != nil {
		return nil, err
	}

	cursor := ""
	deltaUsed := false
	deltaResetReason := ""
	switch {
	case run.Cursor != nil && strings.TrimSpace(run.Cursor.Value) != "":
		// Mid-run pagination: a forced reset applies to the start of a sync,
		// not to every chunk, or the run would restart on each page.
		cursor = strings.TrimSpace(run.Cursor.Value)
		deltaUsed = true
	case run.Payload.Force:
		deltaResetReason = "forced"
	default:
		prevDeltaLink, err := s.Accounts.GetSyncDeltaLink(ctx, run.UserID, accountID)
		if err != nil {
			return nil, err
		}
		cursor = strings.TrimSpace(strOrEmpty(prevDeltaLink))
		deltaUsed = cursor != ""
	}
	deltaRes, err := s.Graph.ListInboxDelta(ctx, tok.AccessToken, cursor, 100)
	if err != nil && deltaUsed && isInvalidDeltaError(err) {
		deltaResetReason = "invalid_delta_link"
		deltaRes, err = s.Graph.ListInboxDelta(ctx, tok.AccessToken, "", 100)
		deltaUsed = false
	}
	if err != nil {
		return nil, err
	}
	list := deltaRes.Messages
	n := 0
	for _, gm := range list {
		if gm.Removed {
			continue
		}
		rt, err := parseGraphTime(gm.ReceivedDateTime)
		if err != nil {
			// A delta row without a timestamp is not a full message payload.
			// Materialising one writes an empty subject and sender over a row
			// that already had both, and dates it now, so it floats to the top
			// of the inbox as if it had just arrived. Skip it and let a later
			// full payload carry the change.
			continue
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
		toJSON, _ := json.Marshal(graphRecipientsJSON(gm.ToRecipients))
		ccJSON, _ := json.Marshal(graphRecipientsJSON(gm.CcRecipients))
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
			ToJSON:            string(toJSON),
			CcJSON:            string(ccJSON),
			BodyText:          nullIfEmpty(body),
			BodyFetchedAt:     bodyFetched,
			HasAttachments:    gm.HasAttachments,
			RawEtag:           nullIfEmpty(etag),
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
		}
		if err := s.Messages.UpsertMessage(ctx, m); err != nil {
			return nil, err
		}
		n++
	}

	out := &SyncChunkResult{
		MessagesUpserted: n,
		Fetched:          len(list),
		DeltaReused:      deltaUsed,
		DeltaResetReason: deltaResetReason,
	}
	if strings.TrimSpace(deltaRes.NextLink) != "" {
		out.NextCursor = &driven.JobCursor{Kind: "graph_next_link", Value: strings.TrimSpace(deltaRes.NextLink)}
		return out, nil
	}
	if strings.TrimSpace(deltaRes.DeltaLink) == "" {
		return nil, fmt.Errorf("graph delta response missing cursor")
	}
	if err := s.Accounts.UpsertSyncState(ctx, run.UserID, accountID, &deltaRes.DeltaLink, time.Now().UTC()); err != nil {
		return nil, err
	}
	out.Done = true
	return out, nil
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
		if opts.RunID != nil {
			if err := s.JobRuns.PromoteJobRunToRunning(ctx, jobID, started); err != nil {
				return nil, err
			}
		} else {
			_ = s.JobRuns.InsertJobRun(ctx, jobID, accountID, "sync", trigger, "running", started, time.Time{}, nil, `{}`)
		}
	}

	var prevDeltaLink *string
	if !opts.Force {
		prevDeltaLink, err = s.Accounts.GetSyncDeltaLink(ctx, userID, accountID)
		if err != nil {
			if s.JobRuns != nil {
				msg := err.Error()
				_ = s.JobRuns.UpdateJobRunStatus(ctx, jobID, "failed", timePtrSync(time.Now().UTC()), &msg, `{}`)
			}
			return nil, err
		}
	}
	deltaUsed := strings.TrimSpace(strOrEmpty(prevDeltaLink)) != ""
	deltaResetReason := ""
	if opts.Force {
		deltaResetReason = "forced"
	}
	deltaRes, err := s.Graph.ListInboxDelta(ctx, tok.AccessToken, strOrEmpty(prevDeltaLink), 50)
	if err != nil && deltaUsed && isInvalidDeltaError(err) {
		deltaResetReason = "invalid_delta_link"
		deltaRes, err = s.Graph.ListInboxDelta(ctx, tok.AccessToken, "", 50)
		deltaUsed = false
	}
	if err != nil {
		if s.JobRuns != nil {
			msg := err.Error()
			_ = s.JobRuns.UpdateJobRunStatus(ctx, jobID, "failed", timePtrSync(time.Now().UTC()), &msg, syncMetaJSON(0, 0, deltaUsed, deltaResetReason))
		}
		return nil, err
	}
	list := deltaRes.Messages
	if s.JobRuns != nil {
		_ = s.JobRuns.UpdateJobRunMeta(ctx, jobID, syncProgressMetaJSON(len(list), 0, 0, deltaUsed, deltaResetReason))
	}

	n := 0
	providerIDs := make([]string, 0, len(list))
	for _, gm := range list {
		if gm.Removed {
			continue
		}
		rt, err := parseGraphTime(gm.ReceivedDateTime)
		if err != nil {
			// A delta row without a timestamp is not a full message payload.
			// Materialising one writes an empty subject and sender over a row
			// that already had both, and dates it now, so it floats to the top
			// of the inbox as if it had just arrived. Skip it and let a later
			// full payload carry the change.
			continue
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
		toJSON, _ := json.Marshal(graphRecipientsJSON(gm.ToRecipients))
		ccJSON, _ := json.Marshal(graphRecipientsJSON(gm.CcRecipients))
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
			ToJSON:            string(toJSON),
			CcJSON:            string(ccJSON),
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
		providerIDs = append(providerIDs, gm.ID)
		n++
		if s.JobRuns != nil {
			_ = s.JobRuns.UpdateJobRunMeta(ctx, jobID, syncProgressMetaJSON(len(list), n, n, deltaUsed, deltaResetReason))
		}
	}
	if s.Resolve != nil {
		if n > 0 {
			_ = s.Resolve.ResolveAfterSync(ctx, userID, accountID, providerIDs)
		}
		// Idempotent backfill so People is populated for mail synced before contacts existed.
		_ = s.Resolve.BackfillAccount(ctx, userID, accountID)
	}
	s.scheduleAssignment(ctx, userID, accountID)
	if err := s.Accounts.UpsertSyncState(ctx, userID, accountID, &deltaRes.DeltaLink, time.Now().UTC()); err != nil {
		return nil, err
	}
	finished := time.Now().UTC()
	if s.JobRuns != nil {
		meta := syncMetaJSON(len(list), n, deltaUsed, deltaResetReason)
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

func graphRecipientsJSON(recs []driven.GraphRecipient) []map[string]string {
	out := make([]map[string]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, map[string]string{"name": r.Name, "address": r.Address})
	}
	return out
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

func isInvalidDeltaError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "syncstatenotfound") ||
		strings.Contains(msg, "invaliddeltatoken") ||
		strings.Contains(msg, "invalid delta token") ||
		strings.Contains(msg, "resyncrequired") ||
		strings.Contains(msg, "410 gone")
}

func syncProgressMetaJSON(total, processed, upserted int, deltaReused bool, deltaResetReason string) string {
	return "{" +
		`"total_messages":` + strconv.Itoa(total) + "," +
		`"processed_messages":` + strconv.Itoa(processed) + "," +
		`"messages_upserted":` + strconv.Itoa(upserted) + "," +
		`"delta_reused":` + strconv.FormatBool(deltaReused) + "," +
		`"delta_reset_reason":"` + jsonEscape(deltaResetReason) + `"` +
		"}"
}

func syncMetaJSON(fetched, upserted int, deltaReused bool, deltaResetReason string) string {
	return "{" +
		`"fetched":` + strconv.Itoa(fetched) + "," +
		`"upserted":` + strconv.Itoa(upserted) + "," +
		`"delta_reused":` + strconv.FormatBool(deltaReused) + "," +
		`"delta_reset_reason":"` + jsonEscape(deltaResetReason) + `"` +
		"}"
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	if len(b) >= 2 && b[0] == '"' && b[len(b)-1] == '"' {
		return string(b[1 : len(b)-1])
	}
	return s
}

func timePtrSync(t time.Time) *time.Time {
	return &t
}

// scheduleAssignment hands project assignment to the job queue when one is
// configured, and otherwise runs it inline.
//
// Assignment scores every unassigned message on the account, which is too much
// work to sit inside a sync response. assign_projects is already a registered
// streamed job with its own chunking and retry policy.
func (s *SyncService) scheduleAssignment(ctx context.Context, userID, accountID uuid.UUID) {
	if s.AssignEnqueuer != nil {
		acc := accountID
		_, err := s.AssignEnqueuer.Enqueue(ctx, driven.CreateJobInput{
			JobType:     "assign_projects",
			UserID:      userID,
			AccountID:   &acc,
			TriggerKind: driven.JobTriggerAPI,
			Now:         time.Now().UTC(),
		})
		if err == nil {
			return
		}
		// Fall through to the inline path so a queue outage does not silently
		// stop correspondence being filed.
	}
	if s.Assign != nil {
		_ = s.Assign.AssignAfterSync(ctx, userID, accountID)
	}
}
