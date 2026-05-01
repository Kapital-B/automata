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

type ForwardRulesService struct {
	Messages  driven.MessageRepository
	Forwards  driven.ForwardRepository
	Accounts  driven.AccountRepository
	OAuth     driven.MicrosoftOAuth
	Graph     driven.MicrosoftGraph
	Vault     driven.TokenVault
	LLM       driven.LLMClient
	JobRuns   driven.JobRunRepository
	ModelName string
}

type ForwardRulesOptions struct {
	RunID   *uuid.UUID
	Trigger string
	Since   *time.Time
}

type forwardLogicCondition struct {
	All []forwardPredicate `json:"all"`
}

type forwardPredicate struct {
	Field string      `json:"field"`
	Op    string      `json:"op"`
	Value interface{} `json:"value"`
}

type forwardLLMCondition struct {
	Prompt string `json:"prompt"`
}

func (s *ForwardRulesService) RunAccount(ctx context.Context, userID, accountID uuid.UUID, opts ForwardRulesOptions) (uuid.UUID, error) {
	if s == nil || s.Messages == nil || s.Forwards == nil || s.Accounts == nil || s.OAuth == nil || s.Graph == nil || s.Vault == nil || s.JobRuns == nil {
		return uuid.Nil, fmt.Errorf("forward rules service not configured")
	}
	jobID := uuid.New()
	if opts.RunID != nil {
		jobID = *opts.RunID
	}
	trigger := strings.TrimSpace(opts.Trigger)
	if trigger == "" {
		trigger = "api"
	}
	_ = s.JobRuns.InsertJobRun(ctx, jobID, accountID, "forward_rules", trigger, "running", time.Now().UTC(), time.Time{}, nil, `{}`)

	allowlistRows, err := s.Forwards.ListForwardAllowlist(ctx, userID)
	if err != nil {
		return s.failRun(ctx, jobID, err)
	}
	allowed := map[string]struct{}{}
	for _, row := range allowlistRows {
		allowed[strings.ToLower(strings.TrimSpace(row.Email))] = struct{}{}
	}
	rules, err := s.Forwards.ListForwardRules(ctx, userID, accountID)
	if err != nil {
		return s.failRun(ctx, jobID, err)
	}
	msgFilter := driven.MessageListFilter{
		AccountID:         &accountID,
		Limit:             200,
		OnlyForwardUnseen: true,
	}
	rows, err := s.Messages.ListMessages(ctx, userID, msgFilter)
	if err != nil {
		return s.failRun(ctx, jobID, err)
	}
	if len(rows) == 0 {
		meta := fmt.Sprintf(`{"total_messages":0,"forward_candidates":0,"total_rules":%d,"forwarded":0,"skipped":0,"failed":0}`, len(rules))
		_ = s.JobRuns.UpdateJobRunStatus(ctx, jobID, "success", timePtrForward(time.Now().UTC()), nil, meta)
		return jobID, nil
	}
	account, cipher, err := s.Accounts.GetAccount(ctx, userID, accountID)
	if err != nil || account == nil {
		if err == nil {
			err = fmt.Errorf("account not found")
		}
		return s.failRun(ctx, jobID, err)
	}
	accessToken, err := s.refreshToken(ctx, userID, accountID, account, cipher)
	if err != nil {
		return s.failRun(ctx, jobID, err)
	}

	forwarded, skipped, failed := 0, 0, 0
	seenMessageIDs := make([]uuid.UUID, 0, len(rows))
	for _, msg := range rows {
		messageForwarded := false
		messageHadFailure := false
		for _, rule := range rules {
			if !rule.Enabled {
				continue
			}
			if _, ok := allowed[strings.ToLower(strings.TrimSpace(rule.ForwardTo))]; !ok {
				reason := "forward_to not in allowlist"
				_ = s.insertAudit(ctx, userID, accountID, msg.ID, rule.ID, jobID, "failed", &reason)
				failed++
				messageHadFailure = true
				continue
			}
			match, reason, err := s.ruleMatches(ctx, rule, msg)
			if err != nil {
				msgErr := err.Error()
				_ = s.insertAudit(ctx, userID, accountID, msg.ID, rule.ID, jobID, "failed", &msgErr)
				failed++
				messageHadFailure = true
				continue
			}
			if !match {
				_ = s.insertAudit(ctx, userID, accountID, msg.ID, rule.ID, jobID, "skipped", &reason)
				skipped++
				continue
			}
			if err := s.Graph.ForwardMessage(ctx, accessToken, msg.ProviderMessageID, rule.ForwardTo, ""); err != nil {
				e := err.Error()
				_ = s.insertAudit(ctx, userID, accountID, msg.ID, rule.ID, jobID, "failed", &e)
				failed++
				messageHadFailure = true
				continue
			}
			okReason := "rule matched and message forwarded"
			_ = s.insertAudit(ctx, userID, accountID, msg.ID, rule.ID, jobID, "forwarded", &okReason)
			forwarded++
			messageForwarded = true
		}
		if messageForwarded || !messageHadFailure {
			seenMessageIDs = append(seenMessageIDs, msg.ID)
		}
	}
	if err := s.Messages.MarkMessagesForwardSeen(ctx, userID, seenMessageIDs, time.Now().UTC()); err != nil {
		return s.failRun(ctx, jobID, err)
	}
	meta := fmt.Sprintf(`{"total_messages":%d,"forward_candidates":%d,"total_rules":%d,"forwarded":%d,"skipped":%d,"failed":%d,"marked_seen":%d}`, len(rows), len(rows), len(rules), forwarded, skipped, failed, len(seenMessageIDs))
	_ = s.JobRuns.UpdateJobRunStatus(ctx, jobID, "success", timePtrForward(time.Now().UTC()), nil, meta)
	return jobID, nil
}

func (s *ForwardRulesService) failRun(ctx context.Context, runID uuid.UUID, err error) (uuid.UUID, error) {
	msg := err.Error()
	_ = s.JobRuns.UpdateJobRunStatus(ctx, runID, "failed", timePtrForward(time.Now().UTC()), &msg, `{}`)
	return uuid.Nil, err
}

func (s *ForwardRulesService) insertAudit(ctx context.Context, userID, accountID, messageID, ruleID, runID uuid.UUID, status string, reason *string) error {
	err := s.Forwards.InsertForwardAudit(ctx, driven.ForwardAuditRow{
		ID:        uuid.New(),
		UserID:    userID,
		AccountID: accountID,
		MessageID: messageID,
		RuleID:    ruleID,
		RunID:     runID,
		Status:    status,
		Reason:    reason,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return nil
	}
	return err
}

func (s *ForwardRulesService) ruleMatches(ctx context.Context, rule driven.ForwardRuleRow, msg driven.MessageRow) (bool, string, error) {
	switch strings.TrimSpace(strings.ToLower(rule.Mode)) {
	case "logic":
		var c forwardLogicCondition
		if err := json.Unmarshal([]byte(rule.ConditionJSON), &c); err != nil {
			return false, "invalid logic condition", err
		}
		for _, p := range c.All {
			if !predicateMatches(msg, p) {
				return false, "logic condition did not match", nil
			}
		}
		return true, "logic condition matched", nil
	case "llm":
		if s.LLM == nil {
			return false, "llm not configured", fmt.Errorf("llm not configured")
		}
		var c forwardLLMCondition
		if err := json.Unmarshal([]byte(rule.ConditionJSON), &c); err != nil {
			return false, "invalid llm condition", err
		}
		resp, err := s.LLM.ChatCompletion(ctx, []driven.LLMMessage{
			{Role: "system", Content: "Evaluate if this email should be forwarded. Reply JSON only: {\"forward\":true|false,\"reason\":\"...\"}"},
			{Role: "user", Content: "Rule: " + c.Prompt + "\n\nSubject: " + msg.Subject + "\nFrom: " + msg.FromJSON + "\nBody: " + deref(msg.BodyText)},
		})
		if err != nil {
			return false, "llm evaluation failed", err
		}
		var out struct {
			Forward bool   `json:"forward"`
			Reason  string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(normalizeJSONContent(resp.Content)), &out); err != nil {
			return false, "llm returned invalid json", err
		}
		return out.Forward, out.Reason, nil
	default:
		return false, "unsupported rule mode", fmt.Errorf("unsupported rule mode: %s", rule.Mode)
	}
}

func predicateMatches(msg driven.MessageRow, p forwardPredicate) bool {
	field := strings.TrimSpace(strings.ToLower(p.Field))
	op := strings.TrimSpace(strings.ToLower(p.Op))
	switch field {
	case "has_attachments":
		want, ok := p.Value.(bool)
		if !ok {
			return false
		}
		if op == "equals" || op == "eq" {
			return msg.HasAttachments == want
		}
	case "category_slug":
		want := strings.TrimSpace(strings.ToLower(fmt.Sprint(p.Value)))
		got := ""
		if msg.CategorySlug != nil {
			got = strings.ToLower(strings.TrimSpace(*msg.CategorySlug))
		}
		return (op == "equals" || op == "eq") && got == want
	}
	return false
}

func (s *ForwardRulesService) refreshToken(ctx context.Context, userID, accountID uuid.UUID, account *driven.AccountRow, cipher []byte) (string, error) {
	raw, err := s.Vault.Decrypt(cipher)
	if err != nil {
		return "", err
	}
	kind, refresh, err := appaccounts.DecodeRefreshPayload(raw)
	if err != nil {
		return "", err
	}
	tok, err := s.OAuth.RefreshAccessToken(ctx, kind, refresh)
	if err != nil {
		return "", err
	}
	nextRefresh := refresh
	if tok.RefreshToken != "" {
		nextRefresh = tok.RefreshToken
	}
	payload, err := appaccounts.EncodeRefreshPayloadForStorage(kind, nextRefresh)
	if err != nil {
		return "", err
	}
	newCipher, err := s.Vault.Encrypt(payload)
	if err != nil {
		return "", err
	}
	if err := s.Accounts.UpdateAccountTokens(ctx, userID, accountID, newCipher, account.PrimaryEmail, account.GraphTenantID, account.MsalHomeAccountID, "connected", nil); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func timePtrForward(t time.Time) *time.Time {
	return &t
}
