package contacts

import (
	"context"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

type ResolveContactsExecutor struct {
	Service *ResolveService
}

func (e *ResolveContactsExecutor) JobType() string { return "resolve_contacts" }

func (e *ResolveContactsExecutor) ExecuteChunk(ctx context.Context, run driven.RunContext) (driven.ChunkResult, error) {
	res, err := e.Service.ResolveAccountChunk(ctx, run)
	if err != nil {
		return driven.ChunkResult{}, err
	}
	return driven.ChunkResult{
		NextCursor: res.NextCursor,
		ProgressDelta: driven.JobProgress{
			Processed: res.MessagesProcessed,
		},
		Done: res.Done,
	}, nil
}
