package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

func parseMSAccessTokenClaims(jwt string) (oid, email string, err error) {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("bad access token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", err
	}
	var c struct {
		OID                  string `json:"oid"`
		Sub                  string `json:"sub"`
		Email                string `json:"email"`
		PreferredUsername    string `json:"preferred_username"`
		UPN                  string `json:"upn"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", "", err
	}
	sub := c.OID
	if sub == "" {
		sub = c.Sub
	}
	if sub == "" {
		return "", "", fmt.Errorf("token missing oid/sub")
	}
	em := strings.TrimSpace(strings.ToLower(c.Email))
	if em == "" {
		em = strings.TrimSpace(strings.ToLower(c.PreferredUsername))
	}
	if em == "" {
		em = strings.TrimSpace(strings.ToLower(c.UPN))
	}
	return sub, em, nil
}
