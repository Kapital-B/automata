package contacts

import (
	"context"

	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
)

type ResolveContactsExecutor struct {
	Service *ResolveService
}

func (e *ResolveContactsExecutor) JobType() string { return "resolve_contacts" }

func (e *ResolveContactsExecutor) ExecuteChunk(ctx context.Context, run domainjobs.RunContext) (domainjobs.ChunkResult, error) {
	res, err := e.Service.ResolveAccountChunk(ctx, run)
	if err != nil {
		return domainjobs.ChunkResult{}, err
	}
	return domainjobs.ChunkResult{
		NextCursor: res.NextCursor,
		ProgressDelta: domainjobs.JobProgress{
			Processed: res.MessagesProcessed,
		},
		Done: res.Done,
	}, nil
}
