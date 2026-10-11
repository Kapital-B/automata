package connectors

import (
	"context"

	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
)

type SyncSlackExecutor struct {
	Service *Service
}

func (e *SyncSlackExecutor) JobType() string { return "sync_slack" }

func (e *SyncSlackExecutor) ExecuteChunk(ctx context.Context, run domainjobs.RunContext) (domainjobs.ChunkResult, error) {
	res, err := e.Service.SyncConnectorChunk(ctx, run)
	if err != nil {
		return domainjobs.ChunkResult{}, err
	}
	return domainjobs.ChunkResult{
		NextCursor: res.NextCursor,
		ProgressDelta: domainjobs.JobProgress{
			Processed: res.MessagesUpserted,
		},
		Done: res.Done,
	}, nil
}
