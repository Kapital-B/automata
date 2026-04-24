package accounts

import (
	"encoding/json"
	"fmt"

	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
)

type mailboxOAuthPayload struct {
	MsAccountKind string  `json:"ms_account_kind"`
	Label         *string `json:"label,omitempty"`
}

func EncodeMailboxOAuthPayload(kind accounts.MsAccountKind, label *string) (string, error) {
	p := mailboxOAuthPayload{MsAccountKind: string(kind), Label: label}
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func DecodeMailboxOAuthPayload(s string) (accounts.MsAccountKind, *string, error) {
	var p mailboxOAuthPayload
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return "", nil, err
	}
	k := accounts.MsAccountKind(p.MsAccountKind)
	if !k.Valid() {
		return "", nil, fmt.Errorf("invalid ms_account_kind in state")
	}
	return k, p.Label, nil
}
