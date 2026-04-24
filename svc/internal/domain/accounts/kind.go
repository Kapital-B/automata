package accounts

// MsAccountKind selects the Entra authority segment (spec §3.1).
type MsAccountKind string

const (
	KindWork     MsAccountKind = "work"
	KindPersonal MsAccountKind = "personal"
)

func (k MsAccountKind) Valid() bool {
	return k == KindWork || k == KindPersonal
}
