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

// GraphClient talks to Microsoft Graph with a caller-supplied access token.
// Mailbox (mailbox.go) is what the application sees.
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

// Profile is who a Graph access token belongs to.
type Profile struct {
	Mail              string
	UserPrincipalName string
	TenantID          string // oid of tenant for work; consumers may use placeholder
}

// GetMe returns profile from Graph /me.
func (g *GraphClient) GetMe(ctx context.Context, accessToken string) (*Profile, error) {
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
	return &Profile{
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

type graphRemovedJSON struct {
	Reason string `json:"reason"`
}

type graphMessageJSON struct {
	ID string `json:"id"`
	// Removed is present only on delta tombstones, which carry no other
	// fields. Without this they decode as a message with every field empty.
	Removed          *graphRemovedJSON `json:"@removed"`
	ConversationID   string            `json:"conversationId"`
	ReceivedDateTime string            `json:"receivedDateTime"`
	Subject          string            `json:"subject"`
	BodyPreview      string            `json:"bodyPreview"`
	HasAttachments   bool              `json:"hasAttachments"`
	ChangeKey        string            `json:"changeKey"`
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
}) []driven.MailRecipient {
	if len(in) == 0 {
		return nil
	}
	out := make([]driven.MailRecipient, 0, len(in))
	for _, r := range in {
		out = append(out, driven.MailRecipient{
			Name:    r.EmailAddress.Name,
			Address: r.EmailAddress.Address,
		})
	}
	return out
}

func mapGraphMessage(m graphMessageJSON) driven.MailMessage {
	return driven.MailMessage{
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

// ListInboxDelta lists exactly one Graph delta page and returns either a next
// link or a final delta link.
func (g *GraphClient) ListInboxDelta(ctx context.Context, accessToken string, deltaLink string, pageSize int) (*driven.MailChangePage, error) {
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
		if deltaLink != "" && isExpiredDeltaError(err) {
			return nil, fmt.Errorf("%w: %v", driven.ErrCursorExpired, err)
		}
		return nil, err
	}
	page := &driven.MailChangePage{Messages: make([]driven.MailMessage, 0, len(res.Value))}
	for _, m := range res.Value {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		// A tombstone carries an id and @removed and nothing else. It is a
		// removal, reported as one — decoding it as a message yields a row
		// of empty fields.
		if m.Removed != nil {
			page.Removed = append(page.Removed, id)
			continue
		}
		page.Messages = append(page.Messages, mapGraphMessage(m))
	}
	page.NextCursor = strings.TrimSpace(res.NextLink)
	page.FinalCursor = strings.TrimSpace(res.DeltaLink)
	if page.NextCursor == "" && page.FinalCursor == "" {
		return nil, fmt.Errorf("graph delta response missing cursor")
	}
	return page, nil
}

// isExpiredDeltaError recognises Graph's ways of saying a delta link is no
// longer valid and the folder has to be enumerated again.
func isExpiredDeltaError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "syncstatenotfound") ||
		strings.Contains(msg, "invaliddeltatoken") ||
		strings.Contains(msg, "invalid delta token") ||
		strings.Contains(msg, "resyncrequired") ||
		strings.Contains(msg, "410 gone")
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

// GetRawMessage returns the full MIME of a message via /$value. Reads are
// bounded by limit so an oversized message fails rather than exhausting memory.
func (g *GraphClient) GetRawMessage(ctx context.Context, accessToken, providerMessageID string, limit int64) ([]byte, error) {
	id := strings.TrimSpace(providerMessageID)
	if id == "" {
		return nil, fmt.Errorf("empty provider message id")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiRoot()+"/me/messages/"+url.PathEscape(id)+"/$value", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := g.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("graph %s: %s", resp.Status, truncate(string(body), 300))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, driven.TooLarge(limit)
	}
	return raw, nil
}
