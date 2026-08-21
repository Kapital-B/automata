package messages

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

const manualForwardCommentMaxRunes = 2000

func destEmailInForwardAllowlist(dest string, rows []driven.ForwardAllowlistRow) bool {
	key := strings.ToLower(strings.TrimSpace(dest))
	if key == "" {
		return false
	}
	for _, row := range rows {
		if strings.ToLower(strings.TrimSpace(row.Email)) == key {
			return true
		}
	}
	return false
}

// ManualForwardMessage forwards a single message via Microsoft Graph to an allowlisted address.
// Writes manual_forward_audit on success and failure.
func (s *ForwardRulesService) ManualForwardMessage(ctx context.Context, userID, messageID uuid.UUID, toEmail, comment string) error {
	if s == nil || s.Messages == nil || s.Forwards == nil || s.Accounts == nil || s.OAuth == nil || s.Graph == nil || s.Vault == nil {
		return fmt.Errorf("forward service not configured")
	}
	to := strings.TrimSpace(toEmail)
	if to == "" {
		return fmt.Errorf("to_email required")
	}
	if !strings.Contains(to, "@") {
		return fmt.Errorf("invalid to_email")
	}
	allowRows, err := s.Forwards.ListForwardAllowlist(ctx, userID)
	if err != nil {
		return err
	}
	if !destEmailInForwardAllowlist(to, allowRows) {
		return fmt.Errorf("forward_to not in allowlist")
	}

	msg, err := s.Messages.GetMessage(ctx, userID, messageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return fmt.Errorf("message not found")
	}

	account, cipher, err := s.Accounts.GetAccount(ctx, userID, msg.AccountID)
	if err != nil {
		return err
	}
	if account == nil {
		return fmt.Errorf("account not found")
	}

	accessToken, err := s.refreshToken(ctx, userID, msg.AccountID, account, cipher)
	if err != nil {
		return err
	}

	c := strings.TrimSpace(comment)
	if c != "" {
		if utf8.RuneCountInString(c) > manualForwardCommentMaxRunes {
			c = string([]rune(c)[:manualForwardCommentMaxRunes])
		}
	}
	var commentPtr *string
	if c != "" {
		commentPtr = &c
	}

	normalizedTo := strings.ToLower(to)
	now := time.Now().UTC()

	graphID, err := s.Graph.ResolveGraphMessageID(ctx, accessToken, msg.ProviderMessageID)
	if err != nil {
		msgErr := err.Error()
		_ = s.Forwards.InsertManualForwardAudit(ctx, driven.ManualForwardAuditRow{
			ID:        uuid.New(),
			UserID:    userID,
			AccountID: msg.AccountID,
			MessageID: messageID,
			ToEmail:   normalizedTo,
			Comment:   commentPtr,
			Status:    "failed",
			Reason:    &msgErr,
			CreatedAt: now,
		})
		return err
	}
	if err := s.Graph.ForwardMessage(ctx, accessToken, graphID, normalizedTo, c); err != nil {
		msgErr := err.Error()
		_ = s.Forwards.InsertManualForwardAudit(ctx, driven.ManualForwardAuditRow{
			ID:        uuid.New(),
			UserID:    userID,
			AccountID: msg.AccountID,
			MessageID: messageID,
			ToEmail:   normalizedTo,
			Comment:   commentPtr,
			Status:    "failed",
			Reason:    &msgErr,
			CreatedAt: now,
		})
		return err
	}
	okReason := "manual forward via Graph"
	_ = s.Forwards.InsertManualForwardAudit(ctx, driven.ManualForwardAuditRow{
		ID:        uuid.New(),
		UserID:    userID,
		AccountID: msg.AccountID,
		MessageID: messageID,
		ToEmail:   normalizedTo,
		Comment:   commentPtr,
		Status:    "forwarded",
		Reason:    &okReason,
		CreatedAt: now,
	})
	return nil
}
