package driven

import "context"

// GoogleOAuth is Google Identity OAuth2 (sign-in) for linking users.
type GoogleOAuth interface {
	AuthorizationURL(ctx context.Context, state string) (string, error)
	ExchangeCode(ctx context.Context, code string) (GoogleTokenResult, error)
}

// GoogleTokenResult holds subject and email from Google sign-in.
type GoogleTokenResult struct {
	ProviderSubject string // id_token "sub"
	Email           string
}
