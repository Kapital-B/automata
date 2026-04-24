package microsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
)

const graphScope = "https://graph.microsoft.com/Mail.Read https://graph.microsoft.com/Mail.Send https://graph.microsoft.com/User.Read offline_access"

// OAuth implements driven.MicrosoftOAuth using the v2.0 token endpoint.
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

func authorityBase(kind accounts.MsAccountKind) string {
	switch kind {
	case accounts.KindWork:
		return "https://login.microsoftonline.com/organizations"
	case accounts.KindPersonal:
		return "https://login.microsoftonline.com/consumers"
	default:
		return "https://login.microsoftonline.com/common"
	}
}

// AuthorizationURL builds the authorize URL for the given account kind.
func (o *OAuth) AuthorizationURL(ctx context.Context, kind accounts.MsAccountKind, state string) (string, error) {
	_ = ctx
	base := authorityBase(kind) + "/oauth2/v2.0/authorize"
	v := url.Values{}
	v.Set("client_id", o.ClientID)
	v.Set("response_type", "code")
	v.Set("redirect_uri", o.RedirectURI)
	v.Set("response_mode", "query")
	v.Set("scope", graphScope)
	v.Set("state", state)
	v.Set("prompt", "consent")
	return base + "?" + v.Encode(), nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// ExchangeCode exchanges an authorization code for tokens.
func (o *OAuth) ExchangeCode(ctx context.Context, kind accounts.MsAccountKind, code string) (driven.TokenPair, error) {
	return o.postToken(ctx, kind, url.Values{
		"client_id":     {o.ClientID},
		"client_secret": {o.ClientSecret},
		"code":          {code},
		"redirect_uri":  {o.RedirectURI},
		"grant_type":    {"authorization_code"},
		"scope":         {graphScope},
	})
}

// RefreshAccessToken refreshes using a refresh token.
func (o *OAuth) RefreshAccessToken(ctx context.Context, kind accounts.MsAccountKind, refreshToken string) (driven.TokenPair, error) {
	return o.postToken(ctx, kind, url.Values{
		"client_id":     {o.ClientID},
		"client_secret": {o.ClientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
		"scope":         {graphScope},
	})
}

func (o *OAuth) postToken(ctx context.Context, kind accounts.MsAccountKind, form url.Values) (driven.TokenPair, error) {
	endpoint := authorityBase(kind) + "/oauth2/v2.0/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return driven.TokenPair{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := o.client().Do(req)
	if err != nil {
		return driven.TokenPair{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return driven.TokenPair{}, err
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return driven.TokenPair{}, fmt.Errorf("token json: %w", err)
	}
	if tr.Error != "" {
		return driven.TokenPair{}, fmt.Errorf("token error %s: %s", tr.Error, tr.ErrorDesc)
	}
	if tr.AccessToken == "" {
		return driven.TokenPair{}, fmt.Errorf("empty access_token")
	}
	return driven.TokenPair{AccessToken: tr.AccessToken, RefreshToken: tr.RefreshToken, ExpiresIn: tr.ExpiresIn}, nil
}
