package slack

import domain "github.com/Kapital-B/automata/svc/internal/domain/connectors"

var _ domain.SlackClient = (*Client)(nil)
