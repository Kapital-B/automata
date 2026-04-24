package accounts

import (
	"encoding/json"

	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
)

type refreshPayload struct {
	RefreshToken string `json:"refresh_token"`
	Kind         string `json:"ms_account_kind"`
}

func encodeRefreshPayload(kind accounts.MsAccountKind, refresh string) ([]byte, error) {
	return json.Marshal(refreshPayload{RefreshToken: refresh, Kind: string(kind)})
}

// EncodeRefreshPayloadForStorage exports token JSON for other application packages.
func EncodeRefreshPayloadForStorage(kind accounts.MsAccountKind, refresh string) ([]byte, error) {
	return encodeRefreshPayload(kind, refresh)
}

// DecodeRefreshPayload decrypts application-owned JSON (after vault decrypt).
func DecodeRefreshPayload(raw []byte) (accounts.MsAccountKind, string, error) {
	var p refreshPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", "", err
	}
	return accounts.MsAccountKind(p.Kind), p.RefreshToken, nil
}
