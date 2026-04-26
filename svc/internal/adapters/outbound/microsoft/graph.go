package microsoft

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

const graphBase = "https://graph.microsoft.com/v1.0"

// GraphClient implements driven.MicrosoftGraph.
type GraphClient struct {
	HTTPClient *http.Client
	// APIRoot overrides the Graph API root (scheme + host + version prefix), e.g.
	// "http://127.0.0.1:1234/v1.0" for tests. Default is graphBase.
	APIRoot string
}

func (g *GraphClient) apiRoot() string {
	if g.APIRoot != "" {
		return strings.TrimRight(g.APIRoot, "/")
	}
	return graphBase
}

func (g *GraphClient) client() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	return http.DefaultClient
}

func (g *GraphClient) getJSON(ctx context.Context, accessToken, reqURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Prefer", `outlook.body-content-type="html"`)
	resp, err := g.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("graph %s: %s", resp.Status, truncate(string(body), 300))
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

func (g *GraphClient) postJSON(ctx context.Context, accessToken, reqURL string, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("graph %s: %s", resp.Status, truncate(string(b), 300))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

type meResponse struct {
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
}

// GetMe returns profile from Graph /me.
func (g *GraphClient) GetMe(ctx context.Context, accessToken string) (*driven.GraphProfile, error) {
	var me meResponse
	if err := g.getJSON(ctx, accessToken, g.apiRoot()+"/me", &me); err != nil {
		return nil, err
	}
	email := me.Mail
	if email == "" {
		email = me.UserPrincipalName
	}
	tenant := ""
	if tid, ok := parseTenantFromAccessToken(accessToken); ok {
		tenant = tid
	}
	if tenant == "" {
		tenant = "unknown"
	}
	return &driven.GraphProfile{
		Mail:              email,
		UserPrincipalName: me.UserPrincipalName,
		TenantID:          tenant,
	}, nil
}

func parseTenantFromAccessToken(jwt string) (string, bool) {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	var claims struct {
		TID string `json:"tid"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", false
	}
	if claims.TID == "" {
		return "", false
	}
	return claims.TID, true
}

type listMessagesResponse struct {
	Value []graphMessageJSON `json:"value"`
}

type graphMessageJSON struct {
	ID               string `json:"id"`
	ConversationID   string `json:"conversationId"`
	ReceivedDateTime string `json:"receivedDateTime"`
	Subject          string `json:"subject"`
	BodyPreview      string `json:"bodyPreview"`
	HasAttachments   bool   `json:"hasAttachments"`
	ChangeKey        string `json:"changeKey"`
	From             struct {
		EmailAddress struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"from"`
	Body struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	} `json:"body"`
}

// ListInboxMessages lists top messages from Inbox (no delta in Phase 1).
func (g *GraphClient) ListInboxMessages(ctx context.Context, accessToken string, top int) ([]driven.GraphMessage, error) {
	if top <= 0 {
		top = 25
	}
	if top > 100 {
		top = 100
	}
	u, _ := url.Parse(g.apiRoot() + "/me/mailFolders/inbox/messages")
	q := u.Query()
	q.Set("$top", fmt.Sprintf("%d", top))
	q.Set("$orderby", "receivedDateTime desc")
	q.Set("$select", "id,conversationId,receivedDateTime,subject,body,bodyPreview,from,hasAttachments,changeKey")
	u.RawQuery = q.Encode()
	var res listMessagesResponse
	if err := g.getJSON(ctx, accessToken, u.String(), &res); err != nil {
		return nil, err
	}
	out := make([]driven.GraphMessage, 0, len(res.Value))
	for _, m := range res.Value {
		rt, _ := time.Parse(time.RFC3339Nano, m.ReceivedDateTime)
		if rt.IsZero() {
			rt, _ = time.Parse(time.RFC3339, m.ReceivedDateTime)
		}
		gm := driven.GraphMessage{
			ID:               m.ID,
			ConversationID:   m.ConversationID,
			ReceivedDateTime: m.ReceivedDateTime,
			Subject:          m.Subject,
			FromName:         m.From.EmailAddress.Name,
			FromAddress:      m.From.EmailAddress.Address,
			BodyPreview:      m.BodyPreview,
			BodyContent:      m.Body.Content,
			BodyContentType:  m.Body.ContentType,
			HasAttachments:   m.HasAttachments,
			ChangeKey:        m.ChangeKey,
		}
		_ = rt
		out = append(out, gm)
	}
	return out, nil
}

// GetMessageBody fetches full message body content.
func (g *GraphClient) GetMessageBody(ctx context.Context, accessToken string, providerMessageID string) (*driven.GraphMessage, error) {
	u := g.apiRoot() + "/me/messages/" + url.PathEscape(providerMessageID) + "?$select=id,conversationId,receivedDateTime,subject,body,bodyPreview,from,hasAttachments,changeKey"
	var m graphMessageJSON
	if err := g.getJSON(ctx, accessToken, u, &m); err != nil {
		return nil, err
	}
	return &driven.GraphMessage{
		ID:               m.ID,
		ConversationID:   m.ConversationID,
		ReceivedDateTime: m.ReceivedDateTime,
		Subject:          m.Subject,
		FromName:         m.From.EmailAddress.Name,
		FromAddress:      m.From.EmailAddress.Address,
		BodyPreview:      m.BodyPreview,
		BodyContent:      m.Body.Content,
		BodyContentType:  m.Body.ContentType,
		HasAttachments:   m.HasAttachments,
		ChangeKey:        m.ChangeKey,
	}, nil
}

func (g *GraphClient) SendMail(ctx context.Context, accessToken string, toEmail, subject, body string) error {
	payload := map[string]any{
		"message": map[string]any{
			"subject": subject,
			"body": map[string]any{
				"contentType": "Text",
				"content":     body,
			},
			"toRecipients": []map[string]any{
				{"emailAddress": map[string]any{"address": toEmail}},
			},
		},
		"saveToSentItems": true,
	}
	return g.postJSON(ctx, accessToken, g.apiRoot()+"/me/sendMail", payload)
}
