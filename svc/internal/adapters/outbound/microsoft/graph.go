package microsoft

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
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
	return g.doGetJSON(ctx, accessToken, reqURL, out, false)
}

// getJSONMail sets Prefer for HTML bodies and immutable message ids (stable across folder moves).
func (g *GraphClient) getJSONMail(ctx context.Context, accessToken, reqURL string, out any) error {
	return g.doGetJSON(ctx, accessToken, reqURL, out, true)
}

func (g *GraphClient) doGetJSON(ctx context.Context, accessToken, reqURL string, out any, mail bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	if mail {
		req.Header.Set("Prefer", `outlook.body-content-type="html", IdType="ImmutableId"`)
	}
	body, err := g.doJSONWithRetry(ctx, req)
	if err != nil {
		return err
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
	_, err = g.doJSONWithRetry(ctx, req)
	return err
}

func (g *GraphClient) doJSONWithRetry(ctx context.Context, req *http.Request) ([]byte, error) {
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = body
		}
		resp, err := g.client().Do(req)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return body, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxAttempts {
			wait := retryAfterDelay(resp.Header.Get("Retry-After"), attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
				continue
			}
		}
		return nil, fmt.Errorf("graph %s: %s", resp.Status, truncate(string(body), 300))
	}
	return nil, fmt.Errorf("graph request retries exhausted")
}

func retryAfterDelay(retryAfter string, attempt int) time.Duration {
	const (
		minDelay = 1 * time.Second
		maxDelay = 10 * time.Second
	)
	retryAfter = strings.TrimSpace(retryAfter)
	if retryAfter != "" {
		if secs, err := strconv.Atoi(retryAfter); err == nil && secs >= 0 {
			d := time.Duration(secs) * time.Second
			if d < minDelay {
				return minDelay
			}
			if d > maxDelay {
				return maxDelay
			}
			return d
		}
		if when, err := http.ParseTime(retryAfter); err == nil {
			d := time.Until(when)
			if d < minDelay {
				return minDelay
			}
			if d > maxDelay {
				return maxDelay
			}
			return d
		}
	}
	backoff := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
	if backoff > maxDelay {
		return maxDelay
	}
	if backoff < minDelay {
		return minDelay
	}
	return backoff
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

type deltaMessagesResponse struct {
	Value     []graphMessageJSON `json:"value"`
	NextLink  string             `json:"@odata.nextLink"`
	DeltaLink string             `json:"@odata.deltaLink"`
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
	ToRecipients []struct {
		EmailAddress struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"toRecipients"`
	CcRecipients []struct {
		EmailAddress struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"ccRecipients"`
	Body struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	} `json:"body"`
}

func mapGraphRecipients(in []struct {
	EmailAddress struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	} `json:"emailAddress"`
}) []driven.GraphRecipient {
	if len(in) == 0 {
		return nil
	}
	out := make([]driven.GraphRecipient, 0, len(in))
	for _, r := range in {
		out = append(out, driven.GraphRecipient{
			Name:    r.EmailAddress.Name,
			Address: r.EmailAddress.Address,
		})
	}
	return out
}

func mapGraphMessage(m graphMessageJSON) driven.GraphMessage {
	return driven.GraphMessage{
		ID:               m.ID,
		ConversationID:   m.ConversationID,
		ReceivedDateTime: m.ReceivedDateTime,
		Subject:          m.Subject,
		FromName:         m.From.EmailAddress.Name,
		FromAddress:      m.From.EmailAddress.Address,
		ToRecipients:     mapGraphRecipients(m.ToRecipients),
		CcRecipients:     mapGraphRecipients(m.CcRecipients),
		BodyPreview:      m.BodyPreview,
		BodyContent:      m.Body.Content,
		BodyContentType:  m.Body.ContentType,
		HasAttachments:   m.HasAttachments,
		ChangeKey:        m.ChangeKey,
	}
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
	q.Set("$select", "id,conversationId,receivedDateTime,subject,body,bodyPreview,from,toRecipients,ccRecipients,hasAttachments,changeKey")
	u.RawQuery = q.Encode()
	var res listMessagesResponse
	if err := g.getJSONMail(ctx, accessToken, u.String(), &res); err != nil {
		return nil, err
	}
	out := make([]driven.GraphMessage, 0, len(res.Value))
	for _, m := range res.Value {
		out = append(out, mapGraphMessage(m))
	}
	return out, nil
}

// ListInboxDelta lists exactly one Graph delta page and returns either a next link or a final delta link.
func (g *GraphClient) ListInboxDelta(ctx context.Context, accessToken string, deltaLink string, pageSize int) (*driven.GraphDeltaResult, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}
	nextURL := strings.TrimSpace(deltaLink)
	if nextURL == "" {
		u, _ := url.Parse(g.apiRoot() + "/me/mailFolders/inbox/messages/delta")
		q := u.Query()
		q.Set("$top", fmt.Sprintf("%d", pageSize))
		q.Set("$select", "id,conversationId,receivedDateTime,subject,body,bodyPreview,from,toRecipients,ccRecipients,hasAttachments,changeKey")
		u.RawQuery = q.Encode()
		nextURL = u.String()
	}
	var res deltaMessagesResponse
	if err := g.getJSONMail(ctx, accessToken, nextURL, &res); err != nil {
		return nil, err
	}
	out := make([]driven.GraphMessage, 0, len(res.Value))
	for _, m := range res.Value {
		// Graph delta may include tombstones; skip rows without id.
		if strings.TrimSpace(m.ID) == "" {
			continue
		}
		out = append(out, mapGraphMessage(m))
	}
	if strings.TrimSpace(res.NextLink) == "" && strings.TrimSpace(res.DeltaLink) == "" {
		return nil, fmt.Errorf("graph delta response missing cursor")
	}
	return &driven.GraphDeltaResult{
		Messages:  out,
		NextLink:  strings.TrimSpace(res.NextLink),
		DeltaLink: strings.TrimSpace(res.DeltaLink),
	}, nil
}

// GetMessageBody fetches full message body content.
func (g *GraphClient) GetMessageBody(ctx context.Context, accessToken string, providerMessageID string) (*driven.GraphMessage, error) {
	id := strings.TrimSpace(providerMessageID)
	if id == "" {
		return nil, fmt.Errorf("empty provider message id")
	}
	u := g.apiRoot() + "/me/messages/" + url.PathEscape(id) + "?$select=id,conversationId,receivedDateTime,subject,body,bodyPreview,from,toRecipients,ccRecipients,hasAttachments,changeKey"
	var m graphMessageJSON
	if err := g.getJSONMail(ctx, accessToken, u, &m); err != nil {
		return nil, err
	}
	gm := mapGraphMessage(m)
	return &gm, nil
}

// ResolveGraphMessageID returns the message id Graph accepts for mutating requests, using immutable ids when supported.
func (g *GraphClient) ResolveGraphMessageID(ctx context.Context, accessToken string, providerMessageID string) (string, error) {
	id := strings.TrimSpace(providerMessageID)
	if id == "" {
		return "", fmt.Errorf("empty provider message id")
	}
	u := g.apiRoot() + "/me/messages/" + url.PathEscape(id) + "?$select=id"
	var m struct {
		ID string `json:"id"`
	}
	if err := g.getJSONMail(ctx, accessToken, u, &m); err != nil {
		return "", err
	}
	out := strings.TrimSpace(m.ID)
	if out == "" {
		return "", fmt.Errorf("graph returned empty message id")
	}
	return out, nil
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

// ReplyToMessage implements POST /me/messages/{id}/reply to keep thread context.
func (g *GraphClient) ReplyToMessage(ctx context.Context, accessToken string, providerMessageID string, body string) error {
	if strings.TrimSpace(providerMessageID) == "" {
		return fmt.Errorf("empty provider message id")
	}
	u := g.apiRoot() + "/me/messages/" + url.PathEscape(strings.TrimSpace(providerMessageID)) + "/reply"
	payload := map[string]any{
		"message": map[string]any{
			"body": map[string]any{
				"contentType": "HTML",
				"content":     plainTextToHTML(body),
			},
		},
	}
	return g.postJSON(ctx, accessToken, u, payload)
}

func plainTextToHTML(s string) string {
	normalized := strings.ReplaceAll(s, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	escaped := html.EscapeString(normalized)
	return strings.ReplaceAll(escaped, "\n", "<br>")
}

// ForwardMessage implements POST /me/messages/{id}/forward (Graph preserves MIME body and attachments).
func (g *GraphClient) ForwardMessage(ctx context.Context, accessToken string, providerMessageID string, toEmail string, comment string) error {
	if strings.TrimSpace(providerMessageID) == "" {
		return fmt.Errorf("empty provider message id")
	}
	toEmail = strings.TrimSpace(toEmail)
	if toEmail == "" {
		return fmt.Errorf("empty forward recipient")
	}
	id := strings.TrimSpace(providerMessageID)
	u := g.apiRoot() + "/me/messages/" + url.PathEscape(id) + "/forward"
	payload := map[string]any{
		"comment": comment,
		"toRecipients": []map[string]any{
			{"emailAddress": map[string]any{"address": toEmail}},
		},
	}
	return g.postJSON(ctx, accessToken, u, payload)
}
