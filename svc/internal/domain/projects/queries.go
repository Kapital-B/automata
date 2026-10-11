package projects

import (
	"context"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	"github.com/google/uuid"
)

type AssignmentsQueries interface {
	EffectiveProjectIDsForMessages(ctx context.Context, userID uuid.UUID, messages []domainmessages.MessageRow) (map[uuid.UUID]*uuid.UUID, error)
}
