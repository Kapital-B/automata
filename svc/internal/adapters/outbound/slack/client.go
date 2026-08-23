package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

const defaultScopes = "channels:history,channels:read,groups:history,groups:read,users:read"

// Client implements Slack OAuth v2 and the Web API. Empty ClientID enables fake mode.
type Client struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Mode         string
	HTTPClient   *http.Client
}

func (c *Client) fake() bool {
	return strings.EqualFold(strings.TrimSpace(c.Mode), "fake") || strings.TrimSpace(c.ClientID) == ""
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (c *Client) AuthorizationURL(_ context.Context, state string) (string, error) {
	if c.fake() {
		u, err := url.Parse(c.RedirectURI)
		if err != nil {
			return "", err
		}
		q := u.Query()
		q.Set("code", "fake")
		q.Set("state", state)
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	u, _ := url.Parse("https://slack.com/oauth/v2/authorize")
	q := u.Query()
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", c.RedirectURI)
	q.Set("scope", defaultScopes)
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *Client) ExchangeCode(ctx context.Context, code string) (*driven.SlackOAuthResult, error) {
	if c.fake() {
		if code != "fake" {
			return nil, fmt.Errorf("invalid fake Slack code")
		}
		return &driven.SlackOAuthResult{
			TeamID: "T_FAKE", TeamName: "Fake Slack", AccessToken: "xoxb-fake",
			Scopes: strings.Split(defaultScopes, ","),
		}, nil
	}
	form := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"code":          {code},
		"redirect_uri":  {c.RedirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/oauth.v2.access", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response struct {
		OK           bool   `json:"ok"`
		Error        string `json:"error"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		Team         struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"team"`
	}
	if err := c.doJSON(req, &response); err != nil {
		return nil, err
	}
	if !response.OK {
		return nil, fmt.Errorf("slack oauth: %s", response.Error)
	}
	if response.AccessToken == "" {
		return nil, fmt.Errorf("slack oauth: missing bot access token")
	}
	return &driven.SlackOAuthResult{
		TeamID: response.Team.ID, TeamName: response.Team.Name,
		AccessToken: response.AccessToken, RefreshToken: response.RefreshToken,
		Scopes: splitScopes(response.Scope),
	}, nil
}

func (c *Client) ListConversations(ctx context.Context, accessToken string) ([]driven.SlackConversation, error) {
	if c.fake() {
		return []driven.SlackConversation{{ID: "C_FAKE_DC01", Name: "dc01-project"}}, nil
	}
	cursor := ""
	out := make([]driven.SlackConversation, 0)
	for {
		u, _ := url.Parse("https://slack.com/api/conversations.list")
		q := u.Query()
		q.Set("limit", "200")
		q.Set("types", "public_channel,private_channel")
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		u.RawQuery = q.Encode()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		var response struct {
			OK       bool   `json:"ok"`
			Error    string `json:"error"`
			Channels []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"channels"`
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := c.doJSON(req, &response); err != nil {
			return nil, err
		}
		if !response.OK {
			return nil, fmt.Errorf("slack conversations.list: %s", response.Error)
		}
		for _, channel := range response.Channels {
			out = append(out, driven.SlackConversation{ID: channel.ID, Name: channel.Name})
		}
		cursor = strings.TrimSpace(response.ResponseMetadata.NextCursor)
		if cursor == "" {
			return out, nil
		}
	}
}

func (c *Client) FetchHistory(ctx context.Context, accessToken, channelID, cursor string) (*driven.SlackHistoryPage, error) {
	if c.fake() {
		if channelID != "C_FAKE_DC01" || cursor != "" {
			return &driven.SlackHistoryPage{Messages: []driven.SlackMessage{}}, nil
		}
		return &driven.SlackHistoryPage{Messages: []driven.SlackMessage{
			{
				ProviderEventID: "1724320800.000100",
				Text:            "Pump P-03 duty point approved at 42 L/s.",
				AuthorLabel:     "Alex Engineer",
				OccurredAt:      time.Date(2024, 8, 22, 10, 0, 0, 100000, time.UTC),
			},
			{
				ProviderEventID: "1724321100.000200",
				Text:            "Proceed with the DC01 cooling layout from revision C.",
				AuthorLabel:     "Morgan Lead",
				OccurredAt:      time.Date(2024, 8, 22, 10, 5, 0, 200000, time.UTC),
			},
		}}, nil
	}
	u, _ := url.Parse("https://slack.com/api/conversations.history")
	q := u.Query()
	q.Set("channel", channelID)
	q.Set("limit", "200")
	if strings.TrimSpace(cursor) != "" {
		q.Set("cursor", cursor)
	}
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	var response struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error"`
		Messages []struct {
			TS       string `json:"ts"`
			Text     string `json:"text"`
			User     string `json:"user"`
			Username string `json:"username"`
			BotID    string `json:"bot_id"`
		} `json:"messages"`
		ResponseMetadata struct {
			NextCursor string `json:"next_cursor"`
		} `json:"response_metadata"`
	}
	if err := c.doJSON(req, &response); err != nil {
		return nil, err
	}
	if !response.OK {
		return nil, fmt.Errorf("slack conversations.history: %s", response.Error)
	}
	displayCache := map[string]string{}
	out := &driven.SlackHistoryPage{
		Messages:   make([]driven.SlackMessage, 0, len(response.Messages)),
		NextCursor: strings.TrimSpace(response.ResponseMetadata.NextCursor),
	}
	for _, message := range response.Messages {
		occurredAt, err := parseSlackTimestamp(message.TS)
		if err != nil {
			continue
		}
		author := strings.TrimSpace(message.Username)
		if author == "" && message.User != "" {
			author = displayCache[message.User]
			if author == "" {
				author = c.userDisplay(ctx, accessToken, message.User)
				displayCache[message.User] = author
			}
		}
		if author == "" && message.BotID != "" {
			author = "Slack bot"
		}
		out.Messages = append(out.Messages, driven.SlackMessage{
			ProviderEventID: message.TS,
			Text:            message.Text,
			AuthorLabel:     author,
			OccurredAt:      occurredAt,
		})
	}
	return out, nil
}

func (c *Client) userDisplay(ctx context.Context, accessToken, userID string) string {
	u, _ := url.Parse("https://slack.com/api/users.info")
	q := u.Query()
	q.Set("user", userID)
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	var response struct {
		OK   bool `json:"ok"`
		User struct {
			RealName string `json:"real_name"`
			Name     string `json:"name"`
			Profile  struct {
				DisplayName string `json:"display_name"`
				RealName    string `json:"real_name"`
			} `json:"profile"`
		} `json:"user"`
	}
	if err := c.doJSON(req, &response); err != nil || !response.OK {
		return userID
	}
	for _, value := range []string{response.User.Profile.DisplayName, response.User.Profile.RealName, response.User.RealName, response.User.Name} {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return userID
}

func (c *Client) doJSON(req *http.Request, target any) error {
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack http status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func parseSlackTimestamp(value string) (time.Time, error) {
	parts := strings.SplitN(value, ".", 2)
	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	var nanos int64
	if len(parts) == 2 {
		fraction := parts[1]
		if len(fraction) > 9 {
			fraction = fraction[:9]
		}
		fraction += strings.Repeat("0", 9-len(fraction))
		nanos, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return time.Time{}, err
		}
	}
	return time.Unix(seconds, nanos).UTC(), nil
}

func splitScopes(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
