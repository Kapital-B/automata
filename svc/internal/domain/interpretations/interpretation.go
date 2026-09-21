package interpretations

// Status is the interpretation lifecycle status.
type Status string

const (
	StatusPending   Status = "pending"
	StatusAccepted  Status = "accepted"
	StatusDismissed Status = "dismissed"
	StatusExpired   Status = "expired"
)

func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusAccepted, StatusDismissed, StatusExpired:
		return true
	default:
		return false
	}
}

// CandidateKind is a claim kind inside an interpretation payload.
type CandidateKind string

const (
	KindFact     CandidateKind = "fact"
	KindDecision CandidateKind = "decision"
	// KindIssue is an open question or piece of work, as opposed to a value
	// that is true (fact) or a choice that was made (decision).
	KindIssue CandidateKind = "issue"
)

func (k CandidateKind) Valid() bool {
	switch k {
	case KindFact, KindDecision, KindIssue:
		return true
	default:
		return false
	}
}
