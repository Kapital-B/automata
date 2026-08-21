package contacts

import "strings"

// IdentityKind is a contact identity channel.
type IdentityKind string

const (
	KindEmail           IdentityKind = "email"
	KindPhone           IdentityKind = "phone"
	KindDisplayNameHint IdentityKind = "display_name_hint"
)

func (k IdentityKind) Valid() bool {
	switch k {
	case KindEmail, KindPhone, KindDisplayNameHint:
		return true
	default:
		return false
	}
}

// ParticipantRole is how a contact appears on a correspondence item.
type ParticipantRole string

const (
	RoleFrom        ParticipantRole = "from"
	RoleTo          ParticipantRole = "to"
	RoleCc          ParticipantRole = "cc"
	RoleParticipant ParticipantRole = "participant"
)

func (r ParticipantRole) Valid() bool {
	switch r {
	case RoleFrom, RoleTo, RoleCc, RoleParticipant:
		return true
	default:
		return false
	}
}

// NormalizeEmail lowercases and trims an email address.
func NormalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

// NormalizePhone keeps digits and a leading +, or digits-only fallback.
func NormalizePhone(phone string) string {
	raw := strings.TrimSpace(phone)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for i, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if r == '+' && i == 0 {
			b.WriteRune(r)
		}
	}
	return b.String()
}
