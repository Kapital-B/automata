package microsoft

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
)

func invalidGrantIdP(t *testing.T) *OAuth {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"AADSTS700082: The refresh token has expired."}`))
	}))
	t.Cleanup(srv.Close)
	return &OAuth{ClientID: "c", ClientSecret: "s", BaseAuthority: srv.URL, HTTPClient: srv.Client()}
}

// A refresh token Microsoft will no longer honour means the account needs
// reconnecting; nothing a retry can do.
func TestRefreshReportsARejectedRefreshToken(t *testing.T) {
	_, err := invalidGrantIdP(t).RefreshAccessToken(context.Background(), accounts.KindWork, "expired")
	if !errors.Is(err, driven.ErrCredentialsRejected) {
		t.Fatalf("err = %v, want ErrCredentialsRejected", err)
	}
}

// The same code on a code exchange is a bad or reused authorization code — a
// failed connect, not an account whose credentials were revoked.
func TestExchangeDoesNotMistakeABadCodeForARevokedAccount(t *testing.T) {
	_, err := invalidGrantIdP(t).ExchangeCode(context.Background(), accounts.KindWork, "reused")
	if err == nil || errors.Is(err, driven.ErrCredentialsRejected) {
		t.Fatalf("err = %v, want a plain failure", err)
	}
}
