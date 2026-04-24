package accounts

// MsAccountKind selects the Entra authority segment (spec §3.1).
type MsAccountKind string

const (
	KindWork     MsAccountKind = "work"
	KindPersonal MsAccountKind = "personal"
	// KindCommon is login.microsoftonline.com/common (organizational + personal Microsoft accounts).
	KindCommon MsAccountKind = "common"
)

func (k MsAccountKind) Valid() bool {
	return k == KindWork || k == KindPersonal || k == KindCommon
}
