package projects

import (
	"context"

	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
)

type AssignProjectsExecutor struct {
	Service *AssignService
}

func (e *AssignProjectsExecutor) JobType() string { return "assign_projects" }

func (e *AssignProjectsExecutor) ExecuteChunk(ctx context.Context, run domainjobs.RunContext) (domainjobs.ChunkResult, error) {
	res, err := e.Service.AssignAccountChunk(ctx, run)
	if err != nil {
		return domainjobs.ChunkResult{}, err
	}
	return domainjobs.ChunkResult{
		NextCursor: res.NextCursor,
		ProgressDelta: domainjobs.JobProgress{
			Processed: res.MessagesProcessed,
			Detail: map[string]interface{}{
				"assigned": res.MessagesAssigned,
			},
		},
		Done: res.Done,
	}, nil
}
