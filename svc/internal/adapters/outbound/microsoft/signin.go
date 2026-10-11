package microsoft

import (
	"context"
	domainaccounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
)

// SignIn adapts the shared Microsoft OAuth capability to authenticated identity values.
type SignIn struct{ OAuth domainaccounts.MicrosoftOAuth }

func (s SignIn) AuthorizationURL(ctx context.Context, state string) (string, error) {
	return s.OAuth.AuthorizationURL(ctx, domainaccounts.KindCommon, state)
}
func (s SignIn) ExchangeIdentity(ctx context.Context, code string) (domainauth.ExternalIdentity, error) {
	tok, err := s.OAuth.ExchangeCode(ctx, domainaccounts.KindCommon, code)
	if err != nil {
		return domainauth.ExternalIdentity{}, err
	}
	sub, email, err := parseMSAccessTokenClaims(tok.AccessToken)
	return domainauth.ExternalIdentity{Subject: sub, Email: email}, err
}

var _ domainauth.IdentityProvider = SignIn{}
