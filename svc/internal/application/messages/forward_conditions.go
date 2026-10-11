package messages

import (
	"encoding/json"
	"fmt"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"

	"strings"
	"time"
	"unicode/utf8"
)

const ForwardModeLogic = domainmessages.MessageForwardModeLogic
const ForwardModeLLM = domainmessages.MessageForwardModeLLM

const ForwardFieldHasAttachments = domainmessages.MessageForwardFieldHasAttachments
const ForwardFieldCategory = domainmessages.MessageForwardFieldCategory
const ForwardFieldFrom = domainmessages.MessageForwardFieldFrom
const ForwardFieldSubject = domainmessages.MessageForwardFieldSubject
const ForwardOpEquals = domainmessages.MessageForwardOpEquals
const ForwardOpContains = domainmessages.MessageForwardOpContains
const ForwardOpDomain = domainmessages.MessageForwardOpDomain

var forwardFieldOps = domainmessages.MessageForwardFieldOps

const maxForwardPredicates = domainmessages.MessageMaxForwardPredicates
const maxForwardPromptLen = domainmessages.MessageMaxForwardPromptLen
const categoryWait = domainmessages.MessageCategoryWait

type ForwardRuleError = domainmessages.MessageForwardRuleError

var ruleError = domainmessages.MessageRuleError

type forwardLogicCondition = domainmessages.MessageForwardLogicCondition

type forwardPredicate = domainmessages.MessageForwardPredicate

type forwardLLMCondition = domainmessages.MessageForwardLLMCondition

var NormalizeForwardRule = domainmessages.MessageNormalizeForwardRule

var strictUnmarshal = domainmessages.MessageStrictUnmarshal

var normalizePredicate = domainmessages.MessageNormalizePredicate

// ruleVerdict is what a rule concluded about one message.
type ruleVerdict struct {
	match bool
	// waiting means the rule cannot judge yet and must look again later.
	waiting bool
	reason  string
}

// evaluateLogic applies a normalized logic condition.
func evaluateLogic(conditionJSON string, msg domainmessages.MessageRow, now time.Time) (ruleVerdict, error) {
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

func predicateMatches(msg domainmessages.MessageRow, p forwardPredicate) bool {
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

var ValidForwardAddress = domainmessages.MessageValidForwardAddress

// truncateRunes cuts s to at most n characters without splitting one.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + " …[truncated]"
}
