package connectors

import (
	"context"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

type SyncSlackExecutor struct {
	Service *Service
}

func (e *SyncSlackExecutor) JobType() string { return "sync_slack" }

func (e *SyncSlackExecutor) ExecuteChunk(ctx context.Context, run driven.RunContext) (driven.ChunkResult, error) {
	res, err := e.Service.SyncConnectorChunk(ctx, run)
	if err != nil {
		return driven.ChunkResult{}, err
	}
	return driven.ChunkResult{
		NextCursor: res.NextCursor,
		ProgressDelta: driven.JobProgress{
			Processed: res.MessagesUpserted,
		},
		Done: res.Done,
	}, nil
}
