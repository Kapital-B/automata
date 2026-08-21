package organisations

// OrgRole is a member's role within an organisation.
type OrgRole string

const (
	RoleOwner  OrgRole = "owner"
	RoleMember OrgRole = "member"
)

func (r OrgRole) Valid() bool {
	return r == RoleOwner || r == RoleMember
}

// DefaultName is the organisation name created at signup.
const DefaultName = "Personal"
