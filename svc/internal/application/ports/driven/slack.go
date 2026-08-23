package driven

import (
	"context"
	"time"
)

// SlackOAuthResult is the identity and token returned by Slack OAuth v2.
type SlackOAuthResult struct {
	TeamID       string
	TeamName     string
	AccessToken  string
	RefreshToken string
	Scopes       []string
}

// SlackConversation is a channel visible to the connected bot.
type SlackConversation struct {
	ID   string
	Name string
}

// SlackMessage is one channel history event.
type SlackMessage struct {
	ProviderEventID string
	Text            string
	AuthorLabel     string
	OccurredAt      time.Time
}

// SlackHistoryPage is one page of channel history.
type SlackHistoryPage struct {
	Messages   []SlackMessage
	NextCursor string
}

// SlackClient is the outbound Slack OAuth and Web API port.
type SlackClient interface {
	AuthorizationURL(ctx context.Context, state string) (string, error)
	ExchangeCode(ctx context.Context, code string) (*SlackOAuthResult, error)
	ListConversations(ctx context.Context, accessToken string) ([]SlackConversation, error)
	FetchHistory(ctx context.Context, accessToken, channelID, cursor string) (*SlackHistoryPage, error)
}
