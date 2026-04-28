package messages

import (
	"context"
	"fmt"
	"strings"
	"time"

	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

type DraftLifecycleService struct {
	Summaries driven.SummaryRepository
	Messages  driven.MessageRepository
	Accounts  driven.AccountRepository
	OAuth     driven.MicrosoftOAuth
	Graph     driven.MicrosoftGraph
	Vault     driven.TokenVault
}

func (s *DraftLifecycleService) SaveDraft(ctx context.Context, userID, draftID uuid.UUID, subject, body string) error {
	if s == nil || s.Summaries == nil {
		return fmt.Errorf("draft service not configured")
	}
	return s.Summaries.UpdateDraftSuggestion(ctx, userID, draftID, strings.TrimSpace(subject), strings.TrimSpace(body), time.Now().UTC())
}

func (s *DraftLifecycleService) DiscardDraft(ctx context.Context, userID, draftID uuid.UUID) error {
	if s == nil || s.Summaries == nil {
		return fmt.Errorf("draft service not configured")
	}
	return s.Summaries.MarkDraftSuggestionStatus(ctx, userID, draftID, "discarded", time.Now().UTC())
}

func (s *DraftLifecycleService) SendDraft(ctx context.Context, userID, draftID uuid.UUID) error {
	if s == nil || s.Summaries == nil || s.Messages == nil || s.Accounts == nil || s.OAuth == nil || s.Graph == nil || s.Vault == nil {
		return fmt.Errorf("draft send service not configured")
	}
	draft, err := s.Summaries.GetDraftSuggestion(ctx, userID, draftID)
	if err != nil {
		return err
	}
	if draft == nil {
		return fmt.Errorf("draft not found")
	}
	if draft.Status != "ready" {
		return fmt.Errorf("draft is not in ready state")
	}
	account, cipher, err := s.Accounts.GetAccount(ctx, userID, draft.AccountID)
	if err != nil {
		return err
	}
	if account == nil || len(cipher) == 0 {
		return fmt.Errorf("account not found")
	}
	raw, err := s.Vault.Decrypt(cipher)
	if err != nil {
		return err
	}
	kind, refresh, err := appaccounts.DecodeRefreshPayload(raw)
	if err != nil {
		return err
	}
	tok, err := s.OAuth.RefreshAccessToken(ctx, kind, refresh)
	if err != nil {
		return fmt.Errorf("refresh token: %w", err)
	}
	newRefresh := refresh
	if tok.RefreshToken != "" {
		newRefresh = tok.RefreshToken
	}
	payload, err := appaccounts.EncodeRefreshPayloadForStorage(kind, newRefresh)
	if err != nil {
		return err
	}
	newCipher, err := s.Vault.Encrypt(payload)
	if err != nil {
		return err
	}
	if err := s.Accounts.UpdateAccountTokens(ctx, userID, draft.AccountID, newCipher, account.PrimaryEmail, account.GraphTenantID, account.MsalHomeAccountID, "connected", nil); err != nil {
		return err
	}
	msg, err := s.Messages.GetMessage(ctx, userID, draft.MessageID)
	if err != nil {
		return err
	}
	if msg == nil || strings.TrimSpace(msg.ProviderMessageID) == "" {
		return fmt.Errorf("source message unavailable")
	}
	now := time.Now().UTC()
	attempt := driven.SendAttemptRow{
		ID:        uuid.New(),
		UserID:    userID,
		AccountID: draft.AccountID,
		DraftID:   draft.ID,
		MessageID: draft.MessageID,
		Status:    "failed",
		CreatedAt: now,
	}
	if err := s.Graph.ReplyToMessage(ctx, tok.AccessToken, msg.ProviderMessageID, draft.Body); err != nil {
		msg := err.Error()
		attempt.ErrorMessage = &msg
		_ = s.Summaries.InsertSendAttempt(ctx, attempt)
		return err
	}
	attempt.Status = "success"
	if err := s.Summaries.InsertSendAttempt(ctx, attempt); err != nil {
		return err
	}
	return s.Summaries.MarkDraftSuggestionStatus(ctx, userID, draftID, "sent", now)
}
