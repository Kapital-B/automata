package messages

import (
	"encoding/json"
	"fmt"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"

	"strings"
	"time"
	"unicode/utf8"
)

// ruleVerdict is what a rule concluded about one message.
type ruleVerdict struct {
	match bool
	// waiting means the rule cannot judge yet and must look again later.
	waiting bool
	reason  string
}

// evaluateLogic applies a normalized logic condition.
func evaluateLogic(conditionJSON string, msg domainmessages.MessageRow, now time.Time) (ruleVerdict, error) {
	var c domainmessages.MessageForwardLogicCondition
	if err := json.Unmarshal([]byte(conditionJSON), &c); err != nil {
		return ruleVerdict{}, err
	}
	if len(c.All) == 0 {
		// Never reachable for a saved rule; refuse rather than match all.
		return ruleVerdict{}, fmt.Errorf("rule has no conditions")
	}
	for _, p := range c.All {
		if p.Field == domainmessages.MessageForwardFieldCategory && msg.CategorySlug == nil && now.Sub(msg.ReceivedAt) < domainmessages.MessageCategoryWait {
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

func predicateMatches(msg domainmessages.MessageRow, p domainmessages.MessageForwardPredicate) bool {
	switch p.Field {
	case domainmessages.MessageForwardFieldHasAttachments:
		want, ok := p.Value.(bool)
		return ok && msg.HasAttachments == want
	case domainmessages.MessageForwardFieldCategory:
		got := ""
		if msg.CategorySlug != nil {
			got = strings.ToLower(strings.TrimSpace(*msg.CategorySlug))
		}
		return got == fmt.Sprint(p.Value)
	case domainmessages.MessageForwardFieldFrom:
		want := fmt.Sprint(p.Value)
		name, addr := senderOf(msg.FromJSON)
		switch p.Op {
		case domainmessages.MessageForwardOpEquals:
			return addr == want
		case domainmessages.MessageForwardOpContains:
			return strings.Contains(addr, want) || strings.Contains(strings.ToLower(name), want)
		case domainmessages.MessageForwardOpDomain:
			domain := addr[strings.LastIndex(addr, "@")+1:]
			return domain == want || strings.HasSuffix(domain, "."+want)
		}
	case domainmessages.MessageForwardFieldSubject:
		want := fmt.Sprint(p.Value)
		subject := strings.ToLower(strings.TrimSpace(msg.Subject))
		switch p.Op {
		case domainmessages.MessageForwardOpEquals:
			return subject == want
		case domainmessages.MessageForwardOpContains:
			return strings.Contains(subject, want)
		}
	}
	return false
}

// truncateRunes cuts s to at most n characters without splitting one.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + " …[truncated]"
}
