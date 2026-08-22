package facts

import "regexp"

// Status is the fact version lifecycle status.
type Status string

const (
	StatusProposed   Status = "proposed"
	StatusActive     Status = "active"
	StatusSuperseded Status = "superseded"
	StatusRejected   Status = "rejected"
)

func (s Status) Valid() bool {
	switch s {
	case StatusProposed, StatusActive, StatusSuperseded, StatusRejected:
		return true
	default:
		return false
	}
}

// Source is how a fact version was produced.
type Source string

const (
	SourceUser Source = "user"
	SourceRule Source = "rule"
	SourceLLM  Source = "llm"
)

func (s Source) Valid() bool {
	switch s {
	case SourceUser, SourceRule, SourceLLM:
		return true
	default:
		return false
	}
}

var subjectKeyRE = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)

// ValidSubjectKey reports whether key matches the Wave 2 dotted identifier form.
func ValidSubjectKey(key string) bool {
	return subjectKeyRE.MatchString(key)
}

// ValidEvidenceXOR reports whether exactly one of message/manual is set.
func ValidEvidenceXOR(messageSet, manualSet bool) bool {
	return (messageSet && !manualSet) || (!messageSet && manualSet)
}
