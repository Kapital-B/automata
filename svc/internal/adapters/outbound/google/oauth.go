package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

const signInScope = "openid email profile"

// OAuth implements driven.GoogleOAuth.
type OAuth struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	HTTPClient   *http.Client
}

func (o *OAuth) client() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return http.DefaultClient
}

func (o *OAuth) AuthorizationURL(ctx context.Context, state string) (string, error) {
	_ = ctx
	v := url.Values{}
	v.Set("client_id", o.ClientID)
	v.Set("redirect_uri", o.RedirectURI)
	v.Set("response_type", "code")
	v.Set("scope", signInScope)
	v.Set("state", state)
	v.Set("access_type", "online")
	v.Set("include_granted_scopes", "true")
	return "https://accounts.google.com/o/oauth2/v2/auth?" + v.Encode(), nil
}

type tokenResp struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	Error       string `json:"error"`
	ErrDesc     string `json:"error_description"`
}

type idClaims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
}

func (o *OAuth) ExchangeCode(ctx context.Context, code string) (driven.GoogleTokenResult, error) {
	out := driven.GoogleTokenResult{}
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", o.ClientID)
	form.Set("client_secret", o.ClientSecret)
	form.Set("redirect_uri", o.RedirectURI)
	form.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := o.client().Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return out, err
	}
	var tr tokenResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return out, fmt.Errorf("token json: %w", err)
	}
	if tr.Error != "" {
		return out, fmt.Errorf("google token %s: %s", tr.Error, tr.ErrDesc)
	}
	if tr.IDToken == "" {
		return out, fmt.Errorf("missing id_token")
	}
	sub, email, err := parseIDTokenClaims(tr.IDToken)
	if err != nil {
		return out, err
	}
	out.ProviderSubject = sub
	out.Email = email
	return out, nil
}

func parseIDTokenClaims(jwt string) (sub, email string, err error) {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("bad id_token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", err
	}
	var c idClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", "", err
	}
	if c.Sub == "" {
		return "", "", fmt.Errorf("id_token missing sub")
	}
	return c.Sub, c.Email, nil
}
