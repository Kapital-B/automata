package microsoft

import (
	"encoding/base64"
	"testing"
)

func TestSignInClaimSelectionCompatibility(t *testing.T) {
	for _, tc := range []struct{ claims, subject, email string }{
		{`{"oid":"object","sub":"subject","email":" EMAIL@EXAMPLE.COM ","preferred_username":"other@example.com"}`, "object", "email@example.com"},
		{`{"sub":"subject","preferred_username":" User@Example.com ","upn":"other@example.com"}`, "subject", "user@example.com"},
		{`{"sub":"subject","upn":" USER@EXAMPLE.COM "}`, "subject", "user@example.com"},
	} {
		token := "header." + base64.RawURLEncoding.EncodeToString([]byte(tc.claims)) + ".signature"
		sub, email, err := parseMSAccessTokenClaims(token)
		if err != nil || sub != tc.subject || email != tc.email {
			t.Fatalf("claims %s: %s %s %v", tc.claims, sub, email, err)
		}
	}
	if _, _, err := parseMSAccessTokenClaims("bad"); err == nil {
		t.Fatal("malformed token accepted")
	}
}
