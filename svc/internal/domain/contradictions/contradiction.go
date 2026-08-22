package contradictions

// Status is the contradiction lifecycle status.
type Status string

const (
	StatusOpen     Status = "open"
	StatusResolved Status = "resolved"
)

func (s Status) Valid() bool {
	switch s {
	case StatusOpen, StatusResolved:
		return true
	default:
		return false
	}
}

// Resolution is how an open contradiction is closed.
type Resolution string

const (
	ResolutionSupersede Resolution = "supersede"
	ResolutionRejectA   Resolution = "reject_a"
	ResolutionRejectB   Resolution = "reject_b"
	ResolutionNote      Resolution = "note"
)

func (r Resolution) Valid() bool {
	switch r {
	case ResolutionSupersede, ResolutionRejectA, ResolutionRejectB, ResolutionNote:
		return true
	default:
		return false
	}
}
