package accounts

import (
	"encoding/json"
	"fmt"

	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
)

// mailboxOAuthPayload rides in OAuth state between starting a connect and
// its callback.
type mailboxOAuthPayload struct {
	// Provider is empty in states written before provider support; those
	// were all Microsoft.
	Provider      string  `json:"provider,omitempty"`
	MsAccountKind string  `json:"ms_account_kind,omitempty"`
	Label         *string `json:"label,omitempty"`
}

func EncodeMailboxOAuthPayload(provider string, kind accounts.MsAccountKind, label *string) (string, error) {
	b, err := json.Marshal(mailboxOAuthPayload{Provider: ProviderKey(provider), MsAccountKind: string(kind), Label: label})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func DecodeMailboxOAuthPayload(s string) (string, accounts.MsAccountKind, *string, error) {
	var p mailboxOAuthPayload
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return "", "", nil, err
	}
	provider := ProviderKey(p.Provider)
	k := accounts.MsAccountKind(p.MsAccountKind)
	if provider == ProviderM365 && !k.Valid() {
		return "", "", nil, fmt.Errorf("invalid ms_account_kind in state")
	}
	return provider, k, p.Label, nil
}
