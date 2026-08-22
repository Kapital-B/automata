package issues

// Status is the Wave 1 issue lifecycle status.
type Status string

const (
	StatusOpen          Status = "open"
	StatusAwaitingInput Status = "awaiting_input"
	StatusResolved      Status = "resolved"
)

func (s Status) Valid() bool {
	switch s {
	case StatusOpen, StatusAwaitingInput, StatusResolved:
		return true
	default:
		return false
	}
}

// ValidAssigneeXOR reports whether at most one of user/contact assignee is set.
func ValidAssigneeXOR(userSet, contactSet bool) bool {
	return !(userSet && contactSet)
}
