package messages

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

// Forward rule modes.
const (
	ForwardModeLogic = "logic"
	ForwardModeLLM   = "llm"
)

// Condition fields and operators a logic rule may use. Anything else is
// refused when the rule is saved: an unknown field used to be accepted and
// then silently never match.
const (
	ForwardFieldHasAttachments = "has_attachments"
	ForwardFieldCategory       = "category_slug"
	ForwardFieldFrom           = "from"
	ForwardFieldSubject        = "subject"

	ForwardOpEquals   = "equals"
	ForwardOpContains = "contains"
	ForwardOpDomain   = "domain"
)

// forwardFieldOps lists the operators each field supports.
var forwardFieldOps = map[string][]string{
	ForwardFieldHasAttachments: {ForwardOpEquals},
	ForwardFieldCategory:       {ForwardOpEquals},
	ForwardFieldFrom:           {ForwardOpEquals, ForwardOpContains, ForwardOpDomain},
	ForwardFieldSubject:        {ForwardOpEquals, ForwardOpContains},
}

const (
	maxForwardPredicates = 10
	maxForwardPromptLen  = 1000
	// categoryWait is how long a category rule waits for a message to be
	// categorised before judging it uncategorised. Forwarding can run before
	// categorisation reaches new mail; judging then used to skip the message
	// for good.
	categoryWait = 72 * time.Hour
)

// ForwardRuleError is a rule the user must fix. Its message says what.
type ForwardRuleError struct{ Reason string }

func (e *ForwardRuleError) Error() string { return e.Reason }

func ruleError(format string, args ...any) error {
	return &ForwardRuleError{Reason: fmt.Sprintf(format, args...)}
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

// NormalizeForwardRule validates a rule's mode and condition and returns them
// in canonical form. An empty condition is an error, not "match everything":
// one bad paste must never forward a whole mailbox.
func NormalizeForwardRule(mode string, condition json.RawMessage) (string, string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case ForwardModeLogic:
		var c forwardLogicCondition
		if err := strictUnmarshal(condition, &c); err != nil {
			return "", "", ruleError("the condition is not valid: %v", err)
		}
		if len(c.All) == 0 {
			return "", "", ruleError("add at least one condition; a rule with none would forward every message")
		}
		if len(c.All) > maxForwardPredicates {
			return "", "", ruleError("a rule can have at most %d conditions", maxForwardPredicates)
		}
		for i := range c.All {
			p, err := normalizePredicate(c.All[i])
			if err != nil {
				return "", "", err
			}
			c.All[i] = p
		}
		out, _ := json.Marshal(c)
		return mode, string(out), nil
	case ForwardModeLLM:
		var c forwardLLMCondition
		if err := strictUnmarshal(condition, &c); err != nil {
			return "", "", ruleError("the condition is not valid: %v", err)
		}
		c.Prompt = strings.TrimSpace(c.Prompt)
		if c.Prompt == "" {
			return "", "", ruleError("describe which messages to forward")
		}
		if utf8.RuneCountInString(c.Prompt) > maxForwardPromptLen {
			return "", "", ruleError("the description can be at most %d characters", maxForwardPromptLen)
		}
		out, _ := json.Marshal(c)
		return mode, string(out), nil
	default:
		return "", "", ruleError("mode must be %q or %q", ForwardModeLogic, ForwardModeLLM)
	}
}

func strictUnmarshal(raw json.RawMessage, v any) error {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return fmt.Errorf("it is empty")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func normalizePredicate(p forwardPredicate) (forwardPredicate, error) {
	field := strings.ToLower(strings.TrimSpace(p.Field))
	op := strings.ToLower(strings.TrimSpace(p.Op))
	if op == "eq" {
		op = ForwardOpEquals
	}
	ops, ok := forwardFieldOps[field]
	if !ok {
		return p, ruleError("unknown condition field %q", p.Field)
	}
	allowed := false
	for _, o := range ops {
		allowed = allowed || o == op
	}
	if !allowed {
		return p, ruleError("%q cannot use %q; use one of %s", field, p.Op, strings.Join(ops, ", "))
	}
	out := forwardPredicate{Field: field, Op: op}
	if field == ForwardFieldHasAttachments {
		b, ok := p.Value.(bool)
		if !ok {
			return p, ruleError("%q needs true or false", field)
		}
		out.Value = b
		return out, nil
	}
	str, ok := p.Value.(string)
	str = strings.ToLower(strings.TrimSpace(str))
	if !ok || str == "" {
		return p, ruleError("%q needs a value", field)
	}
	if field == ForwardFieldFrom && op == ForwardOpDomain {
		str = strings.TrimPrefix(str, "@")
	}
	out.Value = str
	return out, nil
}

// ruleVerdict is what a rule concluded about one message.
type ruleVerdict struct {
	match bool
	// waiting means the rule cannot judge yet and must look again later.
	waiting bool
	reason  string
}

// evaluateLogic applies a normalized logic condition.
func evaluateLogic(conditionJSON string, msg driven.MessageRow, now time.Time) (ruleVerdict, error) {
	var c forwardLogicCondition
	if err := json.Unmarshal([]byte(conditionJSON), &c); err != nil {
		return ruleVerdict{}, err
	}
	if len(c.All) == 0 {
		// Never reachable for a saved rule; refuse rather than match all.
		return ruleVerdict{}, fmt.Errorf("rule has no conditions")
	}
	for _, p := range c.All {
		if p.Field == ForwardFieldCategory && msg.CategorySlug == nil && now.Sub(msg.ReceivedAt) < categoryWait {
			return ruleVerdict{waiting: true, reason: "waiting for the message to be categorised"}, nil
		}
		if !predicateMatches(msg, p) {
			return ruleVerdict{reason: "condition did not match"}, nil
		}
	}
	return ruleVerdict{match: true, reason: "all conditions matched"}, nil
}

func senderOf(fromJSON string) (name, address string) {
	var from struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	}
	_ = json.Unmarshal([]byte(fromJSON), &from)
	return from.Name, strings.ToLower(strings.TrimSpace(from.Address))
}

func predicateMatches(msg driven.MessageRow, p forwardPredicate) bool {
	switch p.Field {
	case ForwardFieldHasAttachments:
		want, ok := p.Value.(bool)
		return ok && msg.HasAttachments == want
	case ForwardFieldCategory:
		got := ""
		if msg.CategorySlug != nil {
			got = strings.ToLower(strings.TrimSpace(*msg.CategorySlug))
		}
		return got == fmt.Sprint(p.Value)
	case ForwardFieldFrom:
		want := fmt.Sprint(p.Value)
		name, addr := senderOf(msg.FromJSON)
		switch p.Op {
		case ForwardOpEquals:
			return addr == want
		case ForwardOpContains:
			return strings.Contains(addr, want) || strings.Contains(strings.ToLower(name), want)
		case ForwardOpDomain:
			domain := addr[strings.LastIndex(addr, "@")+1:]
			return domain == want || strings.HasSuffix(domain, "."+want)
		}
	case ForwardFieldSubject:
		want := fmt.Sprint(p.Value)
		subject := strings.ToLower(strings.TrimSpace(msg.Subject))
		switch p.Op {
		case ForwardOpEquals:
			return subject == want
		case ForwardOpContains:
			return strings.Contains(subject, want)
		}
	}
	return false
}

// ValidForwardAddress reports whether s is a bare email address.
func ValidForwardAddress(s string) bool {
	s = strings.TrimSpace(s)
	a, err := mail.ParseAddress(s)
	return err == nil && strings.EqualFold(a.Address, s)
}

// truncateRunes cuts s to at most n characters without splitting one.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + " …[truncated]"
}
