package projects

import (
	"context"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

type AssignProjectsExecutor struct {
	Service *AssignService
}

func (e *AssignProjectsExecutor) JobType() string { return "assign_projects" }

func (e *AssignProjectsExecutor) ExecuteChunk(ctx context.Context, run driven.RunContext) (driven.ChunkResult, error) {
	res, err := e.Service.AssignAccountChunk(ctx, run)
	if err != nil {
		return driven.ChunkResult{}, err
	}
	return driven.ChunkResult{
		NextCursor: res.NextCursor,
		ProgressDelta: driven.JobProgress{
			Processed: res.MessagesProcessed,
			Detail: map[string]interface{}{
				"assigned": res.MessagesAssigned,
			},
		},
		Done: res.Done,
	}, nil
}
