package messages

import (
	context "context"
	json "encoding/json"
	fmt "fmt"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	uuid "github.com/google/uuid"
	mail "net/mail"
	strings "strings"
	time "time"
	utf8 "unicode/utf8"
)

func MessageRuleError(format string, args ...any) error {
	return &MessageForwardRuleError{Reason: fmt.Sprintf(format, args...)}
}

// MessageValidForwardAddress reports whether s is a bare email address.
func MessageValidForwardAddress(s string) bool {
	s = strings.TrimSpace(s)
	a, err := mail.ParseAddress(s)
	return err == nil && strings.EqualFold(a.Address, s)
}

// MessageForwardPreview is what switching a rule on for existing mail would do.
type MessageForwardPreview struct {
	// InScope is how many messages the rule would look at.
	InScope int
	// Matched is how many of them a logic rule matches; nil for an AI rule,
	// which cannot be counted without asking the model about each one.
	Matched *int
	// Capped means the mailbox is bigger than the preview reads, so both
	// counts are lower bounds.
	Capped  bool
	Samples []MessageRow
}

// Condition fields and operators a logic rule may use. Anything else is
// refused when the rule is saved: an unknown field used to be accepted and
// then silently never match.
const (
	MessageForwardFieldHasAttachments = "has_attachments"
	MessageForwardFieldCategory       = "category_slug"
	MessageForwardFieldFrom           = "from"
	MessageForwardFieldSubject        = "subject"

	MessageForwardOpEquals   = "equals"
	MessageForwardOpContains = "contains"
	MessageForwardOpDomain   = "domain"
)

// MessageAllowlistSet builds the lookup MessageForwardRuleBlock uses.
func MessageAllowlistSet(rows []ForwardAllowlistRow) map[string]struct{} {
	out := map[string]struct{}{}
	for _, row := range rows {
		out[strings.ToLower(strings.TrimSpace(row.Email))] = struct{}{}
	}
	return out
}

// MessageForwardChunkResult reports one page of a forward run.
type MessageForwardChunkResult struct {
	Processed int
	Forwarded int
	Skipped   int
	Failed    int
	Pending   int
	Next      *ForwardCandidateCursor
	Done      bool
}

type MessageSyncResult struct {
	JobRunID         uuid.UUID
	MessagesUpserted int
}

const (
	MessageMaxForwardPredicates = 10
	MessageMaxForwardPromptLen  = 1000
	// MessageCategoryWait is how long a category rule waits for a message to be
	// categorised before judging it uncategorised. Forwarding can run before
	// categorisation reaches new mail; judging then used to skip the message
	// for good.
	MessageCategoryWait = 72 * time.Hour
)

type MessageSummarizeOptions struct {
	RunID     *uuid.UUID
	Trigger   string
	Since     *time.Time
	AccountID *uuid.UUID
}

// MessageRuleScopeStart is the earliest received time a rule considers.
func MessageRuleScopeStart(rule ForwardRuleRow) time.Time {
	if rule.ApplyFrom != nil {
		return rule.ApplyFrom.UTC()
	}
	return rule.CreatedAt.UTC()
}

// MessageForwardRuleBlock says why an enabled rule is not running, or "" if it is.
func MessageForwardRuleBlock(rule ForwardRuleRow, allowed map[string]struct{}) string {
	if _, ok := allowed[strings.ToLower(strings.TrimSpace(rule.ForwardTo))]; !ok {
		return "its destination is no longer on the allowlist"
	}
	if _, _, err := MessageNormalizeForwardRule(rule.Mode, json.RawMessage(rule.ConditionJSON)); err != nil {
		return "its condition is invalid: " + err.Error()
	}
	return ""
}

func MessageStrictUnmarshal(raw json.RawMessage, v any) error {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return fmt.Errorf("it is empty")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

type MessageForwardPredicate struct {
	Field string      `json:"field"`
	Op    string      `json:"op"`
	Value interface{} `json:"value"`
}

type MessageAutoDraftChunkResult struct {
	ActionItemsSeen int
	DraftsGenerated int
	Done            bool
}

type MessageSyncChunkResult struct {
	MessagesUpserted int
	Fetched          int
	DeltaReused      bool
	DeltaResetReason string
	NextCursor       *domainjobs.JobCursor
	Done             bool
}

func MessageNormalizePredicate(p MessageForwardPredicate) (MessageForwardPredicate, error) {
	field := strings.ToLower(strings.TrimSpace(p.Field))
	op := strings.ToLower(strings.TrimSpace(p.Op))
	if op == "eq" {
		op = MessageForwardOpEquals
	}
	ops, ok := MessageForwardFieldOps[field]
	if !ok {
		return p, MessageRuleError("unknown condition field %q", p.Field)
	}
	allowed := false
	for _, o := range ops {
		allowed = allowed || o == op
	}
	if !allowed {
		return p, MessageRuleError("%q cannot use %q; use one of %s", field, p.Op, strings.Join(ops, ", "))
	}
	out := MessageForwardPredicate{Field: field, Op: op}
	if field == MessageForwardFieldHasAttachments {
		b, ok := p.Value.(bool)
		if !ok {
			return p, MessageRuleError("%q needs true or false", field)
		}
		out.Value = b
		return out, nil
	}
	str, ok := p.Value.(string)
	str = strings.ToLower(strings.TrimSpace(str))
	if !ok || str == "" {
		return p, MessageRuleError("%q needs a value", field)
	}
	if field == MessageForwardFieldFrom && op == MessageForwardOpDomain {
		str = strings.TrimPrefix(str, "@")
	}
	out.Value = str
	return out, nil
}

type MessageAutoDraftResult struct {
	JobRunID        uuid.UUID
	ActionItemsSeen int
	DraftsGenerated int
}

type MessageCategorizeOptions struct {
	Recategorize bool
	RunID        *uuid.UUID
	Trigger      string
}

type MessageSyncOptions struct {
	RunID   *uuid.UUID
	Trigger string
	// Force starts from an empty delta link instead of the stored one. Graph
	// only resends messages that changed, so a row whose content was lost
	// locally is never repaired by an ordinary run; a forced one refetches the
	// full payloads.
	Force bool
}

// MessageNormalizeForwardRule validates a rule's mode and condition and returns them
// in canonical form. An empty condition is an error, not "match everything":
// one bad paste must never forward a whole mailbox.
func MessageNormalizeForwardRule(mode string, condition json.RawMessage) (string, string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case MessageForwardModeLogic:
		var c MessageForwardLogicCondition
		if err := MessageStrictUnmarshal(condition, &c); err != nil {
			return "", "", MessageRuleError("the condition is not valid: %v", err)
		}
		if len(c.All) == 0 {
			return "", "", MessageRuleError("add at least one condition; a rule with none would forward every message")
		}
		if len(c.All) > MessageMaxForwardPredicates {
			return "", "", MessageRuleError("a rule can have at most %d conditions", MessageMaxForwardPredicates)
		}
		for i := range c.All {
			p, err := MessageNormalizePredicate(c.All[i])
			if err != nil {
				return "", "", err
			}
			c.All[i] = p
		}
		out, _ := json.Marshal(c)
		return mode, string(out), nil
	case MessageForwardModeLLM:
		var c MessageForwardLLMCondition
		if err := MessageStrictUnmarshal(condition, &c); err != nil {
			return "", "", MessageRuleError("the condition is not valid: %v", err)
		}
		c.Prompt = strings.TrimSpace(c.Prompt)
		if c.Prompt == "" {
			return "", "", MessageRuleError("describe which messages to forward")
		}
		if utf8.RuneCountInString(c.Prompt) > MessageMaxForwardPromptLen {
			return "", "", MessageRuleError("the description can be at most %d characters", MessageMaxForwardPromptLen)
		}
		out, _ := json.Marshal(c)
		return mode, string(out), nil
	default:
		return "", "", MessageRuleError("mode must be %q or %q", MessageForwardModeLogic, MessageForwardModeLLM)
	}
}

// MessageForwardFieldOps lists the operators each field supports.
var MessageForwardFieldOps = map[string][]string{
	MessageForwardFieldHasAttachments: {MessageForwardOpEquals},
	MessageForwardFieldCategory:       {MessageForwardOpEquals},
	MessageForwardFieldFrom:           {MessageForwardOpEquals, MessageForwardOpContains, MessageForwardOpDomain},
	MessageForwardFieldSubject:        {MessageForwardOpEquals, MessageForwardOpContains},
}

// MessageForwardRuleError is a rule the user must fix. Its message says what.
type MessageForwardRuleError struct{ Reason string }

// Forward rule modes.
const (
	MessageForwardModeLogic = "logic"
	MessageForwardModeLLM   = "llm"
)

type MessageForwardLogicCondition struct {
	All []MessageForwardPredicate `json:"all"`
}

type MessageForwardLLMCondition struct {
	Prompt string `json:"prompt"`
}

type MessageAutoDraftOptions struct {
	RunID      *uuid.UUID
	Trigger    string
	OnlyUnseen bool
	MessageID  *uuid.UUID
	Force      bool
}

type MessageSummarizeResult struct {
	JobRunID    uuid.UUID
	ActionItems int
	FYIItems    int
}

type MessageCategorizeResult struct {
	JobRunID            uuid.UUID
	MessagesCategorized int
}

type MessageCategorizeChunkResult struct {
	MessagesCategorized int
	MessagesScanned     int
	NextCursor          *domainjobs.JobCursor
	Done                bool
}

type MessageForwardRulesOptions struct {
	RunID   *uuid.UUID
	Trigger string
	Since   *time.Time
}

type MessageAutoDraftUseCases interface {
	GenerateForAccount(ctx context.Context, userID, accountID uuid.UUID, opts MessageAutoDraftOptions) (*MessageAutoDraftResult, error)
	GenerateChunk(ctx context.Context, run domainjobs.RunContext) (*MessageAutoDraftChunkResult, error)
}

type MessageCategorizeUseCases interface {
	CategorizeAccount(ctx context.Context, userID, accountID uuid.UUID, opts MessageCategorizeOptions) (*MessageCategorizeResult, error)
	CategorizeChunk(ctx context.Context, run domainjobs.RunContext) (*MessageCategorizeChunkResult, error)
}

type MessageDraftLifecycleUseCases interface {
	SaveDraft(ctx context.Context, userID, draftID uuid.UUID, subject, body string) error
	DiscardDraft(ctx context.Context, userID, draftID uuid.UUID) error
	SendDraft(ctx context.Context, userID, draftID uuid.UUID) error
}

type MessageForwardRulesUseCases interface {
	RunChunk(ctx context.Context, run domainjobs.RunContext, effects domainjobs.JobStore, after *ForwardCandidateCursor, limit int) (*MessageForwardChunkResult, error)
	RunAccount(ctx context.Context, userID, accountID uuid.UUID, opts MessageForwardRulesOptions) (uuid.UUID, error)
	Preview(ctx context.Context, userID, accountID uuid.UUID, mode string, condition json.RawMessage, since time.Time) (*MessageForwardPreview, error)
	ManualForwardMessage(ctx context.Context, userID, messageID uuid.UUID, toEmail, comment string) error
}

type MessageSummarizeUseCases interface {
	SummarizeAccount(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, opts MessageSummarizeOptions) (*MessageSummarizeResult, error)
}

type MessageSyncUseCases interface {
	SyncInbox(ctx context.Context, userID uuid.UUID, accountID uuid.UUID) (*MessageSyncResult, error)
	SyncChunk(ctx context.Context, run domainjobs.RunContext) (*MessageSyncChunkResult, error)
	SyncInboxWithOptions(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, opts MessageSyncOptions) (*MessageSyncResult, error)
}

func (e *MessageForwardRuleError) Error() string { return e.Reason }
