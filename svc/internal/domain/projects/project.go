package projects

import (
	"regexp"
	"strings"
)

// AssignmentStatus is committed or provisional.
type AssignmentStatus string

const (
	StatusCommitted   AssignmentStatus = "committed"
	StatusProvisional AssignmentStatus = "provisional"
)

func (s AssignmentStatus) Valid() bool {
	return s == StatusCommitted || s == StatusProvisional
}

// AssignmentSource is how an assignment was produced.
type AssignmentSource string

const (
	SourceUser AssignmentSource = "user"
	SourceRule AssignmentSource = "rule"
	SourceLLM  AssignmentSource = "llm"
)

func (s AssignmentSource) Valid() bool {
	return s == SourceUser || s == SourceRule || s == SourceLLM
}

// AssignScope is thread (default) or single message.
type AssignScope string

const (
	ScopeThread  AssignScope = "thread"
	ScopeMessage AssignScope = "message"
)

func (s AssignScope) Valid() bool {
	return s == ScopeThread || s == ScopeMessage
}

var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,7}$`)

// NormalizeCode trims and uppercases a project code.
func NormalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// ValidCode reports whether code matches Wave 1 structured format (2–8 chars).
func ValidCode(code string) bool {
	return codePattern.MatchString(NormalizeCode(code))
}
