package decisions

// Status is the decision lifecycle status.
type Status string

const (
	StatusProposed   Status = "proposed"
	StatusAccepted   Status = "accepted"
	StatusSuperseded Status = "superseded"
	StatusWithdrawn  Status = "withdrawn"
)

func (s Status) Valid() bool {
	switch s {
	case StatusProposed, StatusAccepted, StatusSuperseded, StatusWithdrawn:
		return true
	default:
		return false
	}
}

// Source is how a decision was produced.
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

// ValidAssigneeXOR reports whether at most one assignee is set.
func ValidAssigneeXOR(userSet, contactSet bool) bool {
	return !(userSet && contactSet)
}

// ValidEvidenceXOR reports whether exactly one of message/manual is set.
func ValidEvidenceXOR(messageSet, manualSet bool) bool {
	return (messageSet && !manualSet) || (!messageSet && manualSet)
}
