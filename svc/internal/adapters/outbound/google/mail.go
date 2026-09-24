package google

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-message/charset"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/mailmime"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

const (
	// Least privilege for what Automata does: read to sync, send for replies
	// and forwards. Both are restricted scopes; see the RFC on verification.
	mailScopes = "https://www.googleapis.com/auth/gmail.readonly https://www.googleapis.com/auth/gmail.send"

	defaultAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultTokenURL = "https://oauth2.googleapis.com/token"
	defaultGmailAPI = "https://gmail.googleapis.com/gmail/v1"

	// DefaultMaxRawMessageBytes bounds raw fetches and re-sent forwards.
	// Gmail's own send limit sits just above it.
	DefaultMaxRawMessageBytes int64 = 25 << 20

	inboxLabel = "INBOX"
)

// MailProvider connects and opens Gmail mailboxes — Workspace and consumer
// alike, since they are the same API. It is a separate OAuth client from
// Google sign-in, so signing in never asks for mailbox access and a user can
// attach several Google mailboxes.
type MailProvider struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	HTTPClient   *http.Client
	// Endpoint overrides for tests.
	AuthURL  string
	TokenURL string
	APIRoot  string
	// MaxRawMessageBytes bounds raw fetches; zero means the default.
	MaxRawMessageBytes int64
}

var (
	_ driven.MailProvider       = (*MailProvider)(nil)
	_ driven.OAuthMailConnector = (*MailProvider)(nil)
)

// credential is the stored form of a Gmail mailbox credential. Unlike the
// Microsoft shape it is tagged, so a blob cannot be mistaken for another's.
type credential struct {
	Type         string `json:"type"`
	RefreshToken string `json:"refresh_token"`
}

const credentialType = "google_oauth"

func (p *MailProvider) client() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return http.DefaultClient
}

func orDefault(v, d string) string {
	if v != "" {
		return strings.TrimRight(v, "/")
	}
	return d
}

func (p *MailProvider) maxRaw() int64 {
	if p.MaxRawMessageBytes > 0 {
		return p.MaxRawMessageBytes
	}
	return DefaultMaxRawMessageBytes
}

func (p *MailProvider) AuthorizationURL(_ context.Context, state string, _ driven.ConnectOptions) (string, error) {
	v := url.Values{}
	v.Set("client_id", p.ClientID)
	v.Set("redirect_uri", p.RedirectURI)
	v.Set("response_type", "code")
	v.Set("scope", mailScopes)
	v.Set("state", state)
	// offline + consent guarantees a refresh token on every connect, which a
	// reconnect after expiry depends on.
	v.Set("access_type", "offline")
	v.Set("prompt", "consent")
	return orDefault(p.AuthURL, defaultAuthURL) + "?" + v.Encode(), nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func (p *MailProvider) token(ctx context.Context, form url.Values) (*tokenResponse, error) {
	form.Set("client_id", p.ClientID)
	form.Set("client_secret", p.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, orDefault(p.TokenURL, defaultTokenURL), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var tr tokenResponse
	_ = json.Unmarshal(body, &tr)
	if tr.Error != "" || resp.StatusCode != http.StatusOK {
		// invalid_grant is Google saying the refresh token is revoked or
		// expired — including the 7-day expiry of a Testing-mode client.
		// Retrying cannot fix it; the account needs reconnecting.
		if tr.Error == "invalid_grant" {
			return nil, fmt.Errorf("%w: google token error %s: %s", driven.ErrCredentialsRejected, tr.Error, tr.ErrorDesc)
		}
		return nil, fmt.Errorf("google token error %s (%s): %s", tr.Error, resp.Status, tr.ErrorDesc)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("google token response without access_token")
	}
	return &tr, nil
}

func (p *MailProvider) Complete(ctx context.Context, code string, _ driven.ConnectOptions) (*driven.ConnectedMailbox, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("redirect_uri", p.RedirectURI)
	form.Set("grant_type", "authorization_code")
	tok, err := p.token(ctx, form)
	if err != nil {
		return nil, fmt.Errorf("exchange: %w", err)
	}
	if tok.RefreshToken == "" {
		return nil, fmt.Errorf("missing refresh_token")
	}
	box := p.mailbox(tok.AccessToken, "")
	prof, err := box.profile(ctx)
	if err != nil {
		return nil, fmt.Errorf("gmail profile: %w", err)
	}
	cred, err := json.Marshal(credential{Type: credentialType, RefreshToken: tok.RefreshToken})
	if err != nil {
		return nil, err
	}
	label := prof.EmailAddress
	if label == "" {
		label = "Google"
	}
	return &driven.ConnectedMailbox{Email: prof.EmailAddress, DefaultLabel: label, Credential: cred}, nil
}

func (p *MailProvider) Open(ctx context.Context, account driven.AccountRow, raw []byte) (driven.Mailbox, []byte, error) {
	var c credential
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, nil, err
	}
	if c.Type != credentialType || c.RefreshToken == "" {
		return nil, nil, fmt.Errorf("not a google mail credential")
	}
	form := url.Values{}
	form.Set("refresh_token", c.RefreshToken)
	form.Set("grant_type", "refresh_token")
	tok, err := p.token(ctx, form)
	if err != nil {
		return nil, nil, fmt.Errorf("refresh token: %w", err)
	}
	// Google rarely rotates refresh tokens; persist only when it does.
	var rotated []byte
	if tok.RefreshToken != "" && tok.RefreshToken != c.RefreshToken {
		if rotated, err = json.Marshal(credential{Type: credentialType, RefreshToken: tok.RefreshToken}); err != nil {
			return nil, nil, err
		}
	}
	return p.mailbox(tok.AccessToken, account.PrimaryEmail), rotated, nil
}

func (p *MailProvider) mailbox(accessToken, email string) *gmailMailbox {
	return &gmailMailbox{
		api:    orDefault(p.APIRoot, defaultGmailAPI) + "/users/me",
		token:  accessToken,
		email:  email,
		client: p.client(),
		maxRaw: p.maxRaw(),
	}
}

// gmailMailbox is an opened Gmail mailbox.
type gmailMailbox struct {
	api    string
	token  string
	email  string
	client *http.Client
	maxRaw int64
}

// apiError is a non-2xx answer from Gmail.
type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("gmail %d: %s", e.Status, e.Body) }

func statusOf(err error) int {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.Status
	}
	return 0
}

func (m *gmailMailbox) do(ctx context.Context, method, path string, q url.Values, body any, limit int64, out any) error {
	u := m.api + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	for attempt := 1; ; attempt++ {
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rdr)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+m.token)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := m.client.Do(req)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		_ = resp.Body.Close()
		if readErr != nil {
			return readErr
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
				continue
			}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			b := string(data)
			if len(b) > 300 {
				b = b[:300]
			}
			return &apiError{Status: resp.StatusCode, Body: b}
		}
		if int64(len(data)) > limit {
			return driven.TooLarge(limit)
		}
		if out != nil {
			return json.Unmarshal(data, out)
		}
		return nil
	}
}

const jsonLimit = 8 << 20

type profileResponse struct {
	EmailAddress string `json:"emailAddress"`
	HistoryID    string `json:"historyId"`
}

func (m *gmailMailbox) profile(ctx context.Context) (*profileResponse, error) {
	var p profileResponse
	if err := m.do(ctx, http.MethodGet, "/profile", nil, nil, jsonLimit, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

var capabilities = driven.MailboxCapabilities{
	IncrementalSync:   true,
	ServerSideForward: false,
	ServerSideReply:   false,
	ReportsRemovals:   true,
	StableMessageIDs:  true,
}

func (p *MailProvider) Capabilities() driven.MailboxCapabilities { return capabilities }

func (m *gmailMailbox) Capabilities() driven.MailboxCapabilities { return capabilities }

// Cursors are opaque to callers:
//
//	full:<historyId>:<pageToken>  mid-way through a full enumeration
//	h:<historyId>                 caught up; resume from this history point
//	hist:<historyId>:<pageToken>  mid-way through a history replay
//
// A full enumeration records the history id it started from, so mail that
// arrives while it pages is picked up by the first history replay after it.
func parseCursor(c string) (kind, history, token string) {
	parts := strings.SplitN(c, ":", 3)
	switch {
	case c == "":
		return "", "", ""
	case len(parts) == 2 && parts[0] == "h":
		return "h", parts[1], ""
	case len(parts) == 3 && (parts[0] == "full" || parts[0] == "hist"):
		return parts[0], parts[1], parts[2]
	}
	return "invalid", "", ""
}

func clampPage(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 100 {
		return 100
	}
	return n
}

func (m *gmailMailbox) ListChanges(ctx context.Context, cursor string, pageSize int) (*driven.MailChangePage, error) {
	pageSize = clampPage(pageSize)
	kind, history, token := parseCursor(cursor)
	switch kind {
	case "":
		prof, err := m.profile(ctx)
		if err != nil {
			return nil, err
		}
		return m.listFull(ctx, prof.HistoryID, "", pageSize)
	case "full":
		return m.listFull(ctx, history, token, pageSize)
	case "h":
		return m.listHistory(ctx, history, "", pageSize)
	case "hist":
		return m.listHistory(ctx, history, token, pageSize)
	}
	return nil, fmt.Errorf("%w: unrecognised cursor", driven.ErrCursorExpired)
}

type listResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
	NextPageToken string `json:"nextPageToken"`
}

func (m *gmailMailbox) listFull(ctx context.Context, history, token string, pageSize int) (*driven.MailChangePage, error) {
	q := url.Values{}
	q.Set("labelIds", inboxLabel)
	q.Set("maxResults", strconv.Itoa(pageSize))
	if token != "" {
		q.Set("pageToken", token)
	}
	var lr listResponse
	if err := m.do(ctx, http.MethodGet, "/messages", q, nil, jsonLimit, &lr); err != nil {
		return nil, err
	}
	page := &driven.MailChangePage{}
	for _, ref := range lr.Messages {
		msg, err := m.fetch(ctx, ref.ID)
		if statusOf(err) == http.StatusNotFound {
			continue // deleted between listing and fetching
		}
		if err != nil {
			return nil, err
		}
		page.Messages = append(page.Messages, msg.toMailMessage())
	}
	if lr.NextPageToken != "" {
		page.NextCursor = "full:" + history + ":" + lr.NextPageToken
	} else {
		page.FinalCursor = "h:" + history
	}
	return page, nil
}

type historyMessage struct {
	ID       string   `json:"id"`
	LabelIDs []string `json:"labelIds"`
}

type historyResponse struct {
	History []struct {
		MessagesAdded []struct {
			Message historyMessage `json:"message"`
		} `json:"messagesAdded"`
		MessagesDeleted []struct {
			Message historyMessage `json:"message"`
		} `json:"messagesDeleted"`
		LabelsAdded []struct {
			Message  historyMessage `json:"message"`
			LabelIDs []string       `json:"labelIds"`
		} `json:"labelsAdded"`
		LabelsRemoved []struct {
			Message  historyMessage `json:"message"`
			LabelIDs []string       `json:"labelIds"`
		} `json:"labelsRemoved"`
	} `json:"history"`
	NextPageToken string `json:"nextPageToken"`
	HistoryID     string `json:"historyId"`
}

func hasInbox(labels []string) bool {
	for _, l := range labels {
		if l == inboxLabel {
			return true
		}
	}
	return false
}

func (m *gmailMailbox) listHistory(ctx context.Context, start, token string, pageSize int) (*driven.MailChangePage, error) {
	q := url.Values{}
	q.Set("startHistoryId", start)
	q.Set("maxResults", strconv.Itoa(pageSize))
	for _, t := range []string{"messageAdded", "messageDeleted", "labelAdded", "labelRemoved"} {
		q.Add("historyTypes", t)
	}
	if token != "" {
		q.Set("pageToken", token)
	}
	var hr historyResponse
	if err := m.do(ctx, http.MethodGet, "/history", q, nil, jsonLimit, &hr); err != nil {
		// Google answers 404 once a start id is older than it keeps.
		if statusOf(err) == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %v", driven.ErrCursorExpired, err)
		}
		return nil, err
	}
	// Replay in order, so a message added then moved out in the same window
	// ends up removed, not added.
	touched := map[string]bool{}
	var order []string
	mark := func(id string, inInbox bool) {
		if _, seen := touched[id]; !seen {
			order = append(order, id)
		}
		touched[id] = inInbox
	}
	for _, h := range hr.History {
		for _, a := range h.MessagesAdded {
			if hasInbox(a.Message.LabelIDs) {
				mark(a.Message.ID, true)
			}
		}
		for _, a := range h.LabelsAdded {
			if hasInbox(a.LabelIDs) {
				mark(a.Message.ID, true)
			}
		}
		for _, r := range h.LabelsRemoved {
			if hasInbox(r.LabelIDs) {
				mark(r.Message.ID, false)
			}
		}
		for _, d := range h.MessagesDeleted {
			mark(d.Message.ID, false)
		}
	}
	page := &driven.MailChangePage{}
	for _, id := range order {
		if !touched[id] {
			page.Removed = append(page.Removed, id)
			continue
		}
		msg, err := m.fetch(ctx, id)
		if statusOf(err) == http.StatusNotFound {
			page.Removed = append(page.Removed, id)
			continue
		}
		if err != nil {
			return nil, err
		}
		// The current labels are the truth, whatever the history said.
		if !hasInbox(msg.LabelIDs) {
			page.Removed = append(page.Removed, id)
			continue
		}
		page.Messages = append(page.Messages, msg.toMailMessage())
	}
	if hr.NextPageToken != "" {
		page.NextCursor = "hist:" + start + ":" + hr.NextPageToken
	} else {
		latest := hr.HistoryID
		if latest == "" {
			latest = start
		}
		page.FinalCursor = "h:" + latest
	}
	return page, nil
}

type gmailPart struct {
	MimeType string `json:"mimeType"`
	Filename string `json:"filename"`
	Headers  []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"headers"`
	Body struct {
		AttachmentID string `json:"attachmentId"`
		Data         string `json:"data"`
	} `json:"body"`
	Parts []gmailPart `json:"parts"`
}

func (p *gmailPart) header(name string) string {
	for _, h := range p.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

type gmailMessage struct {
	ID           string     `json:"id"`
	ThreadID     string     `json:"threadId"`
	LabelIDs     []string   `json:"labelIds"`
	Snippet      string     `json:"snippet"`
	HistoryID    string     `json:"historyId"`
	InternalDate string     `json:"internalDate"`
	Payload      *gmailPart `json:"payload"`
	Raw          string     `json:"raw"`
}

// fetch gets a message with text parts inline and attachments by reference,
// so syncing never downloads attachment bytes.
func (m *gmailMailbox) fetch(ctx context.Context, id string) (*gmailMessage, error) {
	q := url.Values{}
	q.Set("format", "full")
	var msg gmailMessage
	if err := m.do(ctx, http.MethodGet, "/messages/"+url.PathEscape(id), q, nil, jsonLimit, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

var wordDecoder = &mime.WordDecoder{CharsetReader: charset.Reader}

func decodeHeader(v string) string {
	if out, err := wordDecoder.DecodeHeader(v); err == nil {
		return out
	}
	return v
}

func decodeData(s string) []byte {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		return nil
	}
	return b
}

func decodeText(p *gmailPart) string {
	data := decodeData(p.Body.Data)
	if _, params, err := mime.ParseMediaType(p.header("Content-Type")); err == nil {
		if cs := params["charset"]; cs != "" && !strings.EqualFold(cs, "utf-8") {
			if r, err := charset.Reader(cs, bytes.NewReader(data)); err == nil {
				if out, err := io.ReadAll(r); err == nil {
					return string(out)
				}
			}
		}
	}
	return string(data)
}

func (msg *gmailMessage) toMailMessage() driven.MailMessage {
	out := driven.MailMessage{ID: msg.ID, ConversationID: msg.ThreadID, ChangeKey: msg.HistoryID, BodyPreview: msg.Snippet}
	if ms, err := strconv.ParseInt(msg.InternalDate, 10, 64); err == nil {
		out.ReceivedDateTime = time.UnixMilli(ms).UTC().Format(time.RFC3339)
	}
	if msg.Payload == nil {
		return out
	}
	top := msg.Payload
	out.Subject = decodeHeader(top.header("Subject"))
	parser := &mail.AddressParser{WordDecoder: wordDecoder}
	if from, err := parser.Parse(top.header("From")); err == nil {
		out.FromName, out.FromAddress = from.Name, from.Address
	}
	for _, key := range []string{"To", "Cc"} {
		list, err := parser.ParseList(top.header(key))
		if err != nil {
			continue
		}
		for _, a := range list {
			r := driven.MailRecipient{Name: a.Name, Address: a.Address}
			if key == "To" {
				out.ToRecipients = append(out.ToRecipients, r)
			} else {
				out.CcRecipients = append(out.CcRecipients, r)
			}
		}
	}
	var text, html string
	var walk func(p *gmailPart)
	walk = func(p *gmailPart) {
		if p.Filename != "" || (p.Body.AttachmentID != "" && !strings.HasPrefix(p.MimeType, "text/")) {
			out.HasAttachments = true
			return
		}
		switch strings.ToLower(p.MimeType) {
		case "text/plain":
			if text == "" {
				text = decodeText(p)
			}
		case "text/html":
			if html == "" {
				html = decodeText(p)
			}
		}
		for i := range p.Parts {
			walk(&p.Parts[i])
		}
	}
	walk(top)
	if html != "" {
		out.BodyContent, out.BodyContentType = html, "HTML"
	} else {
		out.BodyContent, out.BodyContentType = text, "Text"
	}
	return out
}

// raw fetches the full RFC 5322 message and its thread.
func (m *gmailMailbox) raw(ctx context.Context, id string) ([]byte, string, error) {
	q := url.Values{}
	q.Set("format", "raw")
	var msg gmailMessage
	// base64 inflates by a third; allow for it plus the JSON around it.
	if err := m.do(ctx, http.MethodGet, "/messages/"+url.PathEscape(id), q, nil, m.maxRaw*4/3+64<<10, &msg); err != nil {
		return nil, "", err
	}
	data := decodeData(msg.Raw)
	if data == nil {
		return nil, "", fmt.Errorf("gmail returned no raw message for %s", id)
	}
	if int64(len(data)) > m.maxRaw {
		return nil, "", driven.TooLarge(m.maxRaw)
	}
	return data, msg.ThreadID, nil
}

func (m *gmailMailbox) GetRawMessage(ctx context.Context, id string) ([]byte, error) {
	data, _, err := m.raw(ctx, id)
	return data, err
}

// notSent marks a failure that happened before Gmail accepted anything.
func notSent(err error) error {
	if errors.Is(err, driven.ErrMailNotSent) {
		return err
	}
	return fmt.Errorf("%w: %v", driven.ErrMailNotSent, err)
}

// send hands a composed message to Gmail. A 4xx is Gmail refusing it, so
// nothing was sent; a transport failure or 5xx leaves the outcome unknown.
func (m *gmailMailbox) send(ctx context.Context, raw []byte, threadID string) error {
	if int64(len(raw)) > m.maxRaw {
		return driven.TooLarge(m.maxRaw)
	}
	body := map[string]string{"raw": base64.RawURLEncoding.EncodeToString(raw)}
	if threadID != "" {
		body["threadId"] = threadID
	}
	err := m.do(ctx, http.MethodPost, "/messages/send", nil, body, jsonLimit, nil)
	if s := statusOf(err); s >= 400 && s < 500 {
		return notSent(err)
	}
	return err
}

func (m *gmailMailbox) Reply(ctx context.Context, id, body string) error {
	original, thread, err := m.raw(ctx, id)
	if err != nil {
		return notSent(err)
	}
	reply, err := mailmime.BuildReply(m.email, original, body, time.Now())
	if err != nil {
		return notSent(err)
	}
	return m.send(ctx, reply, thread)
}

// Forward re-sends the original as an attachment: Gmail has no server-side
// forward. Everything up to the send is not-sent, so a rule can retry it.
func (m *gmailMailbox) Forward(ctx context.Context, id, to, comment string) error {
	original, _, err := m.raw(ctx, id)
	if err != nil {
		return notSent(err)
	}
	fwd, err := mailmime.BuildForward(m.email, strings.TrimSpace(to), comment, original, time.Now())
	if err != nil {
		return notSent(err)
	}
	return m.send(ctx, fwd, "")
}
