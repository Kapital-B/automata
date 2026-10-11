package jobs

import (
	"context"
	"github.com/google/uuid"
)

type Submission struct {
	Mode                                                                    string
	RunID, ChainID                                                          uuid.UUID
	MessagesUpserted, MessagesCategorized, DraftsGenerated, ActionItemsSeen int
	AlreadyRunning                                                          bool
}
type RequestSubmission interface {
	Submit(ctx context.Context, in CreateJobInput) (*JobRecord, error)
}
