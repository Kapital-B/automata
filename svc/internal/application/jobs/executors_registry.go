package jobs

import (
	domaincontradictions "github.com/Kapital-B/automata/svc/internal/domain/contradictions"
	domaininterpretations "github.com/Kapital-B/automata/svc/internal/domain/interpretations"
)

import (
	"context"
	"fmt"
	"time"

	appconnectors "github.com/Kapital-B/automata/svc/internal/application/connectors"
	appcontacts "github.com/Kapital-B/automata/svc/internal/application/contacts"
	appinterpret "github.com/Kapital-B/automata/svc/internal/application/interpret"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	appprojectai "github.com/Kapital-B/automata/svc/internal/application/projectai"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	appreconcile "github.com/Kapital-B/automata/svc/internal/application/reconcile"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
)

type ExecutorDeps struct {
	Store            domainjobs.JobStore
	Sync             *appmessages.SyncService
	SyncSlack        *appconnectors.Service
	Categorize       *appmessages.CategorizeService
	Summarize        *appmessages.SummarizeService
	DraftSuggest     *appmessages.AutoDraftService
	ForwardRules     *appmessages.ForwardRulesService
	ResolveContacts  *appcontacts.ResolveService
	AssignProjects   *appprojects.AssignService
	InterpretProject *appinterpret.Service
	ReconcileProject *appreconcile.Service
	// Projects records the extraction watermark when a chain completes.
	Projects  domainprojects.ProjectRepository
	ProjectAI *appprojectai.Service
}

func NewExecutorRegistry(deps ExecutorDeps) ExecutorRegistry {
	reg := ExecutorRegistry{}
	if deps.Sync != nil {
		reg[domainjobs.TypeSync] = &appmessages.SyncExecutor{Service: deps.Sync}
	}
	if deps.SyncSlack != nil {
		reg[domainjobs.TypeSyncSlack] = &appconnectors.SyncSlackExecutor{Service: deps.SyncSlack}
	}
	if deps.Categorize != nil {
		reg[domainjobs.TypeCategorize] = &appmessages.CategorizeExecutor{Service: deps.Categorize}
	}
	if deps.Summarize != nil {
		reg[domainjobs.TypeSummarize] = &appmessages.SummarizeExecutor{Service: deps.Summarize}
	}
	if deps.DraftSuggest != nil {
		reg[domainjobs.TypeDraftSuggest] = &appmessages.DraftSuggestExecutor{Service: deps.DraftSuggest}
	}
	if deps.ForwardRules != nil && deps.Store != nil {
		reg[domainjobs.TypeForwardRules] = &appmessages.ForwardRulesExecutor{Service: deps.ForwardRules, Store: deps.Store}
	}
	if deps.ResolveContacts != nil {
		reg[domainjobs.TypeResolveContacts] = &appcontacts.ResolveContactsExecutor{Service: deps.ResolveContacts}
	}
	if deps.AssignProjects != nil {
		reg[domainjobs.TypeAssignProjects] = &appprojects.AssignProjectsExecutor{Service: deps.AssignProjects}
	}
	if deps.InterpretProject != nil {
		reg[domainjobs.TypeInterpretProject] = &interpretProjectExecutor{service: deps.InterpretProject}
	}
	if deps.ReconcileProject != nil {
		reg[domainjobs.TypeReconcileProject] = &reconcileProjectExecutor{service: deps.ReconcileProject, projects: deps.Projects}
	}
	if deps.ProjectAI != nil {
		reg[domainjobs.TypeProjectAI] = &projectAIExecutor{service: deps.ProjectAI}
	}
	return reg
}

type interpretProjectExecutor struct {
	service *appinterpret.Service
}

func (e *interpretProjectExecutor) JobType() string { return domainjobs.TypeInterpretProject }

func (e *interpretProjectExecutor) ExecuteChunk(ctx context.Context, run domainjobs.RunContext) (domainjobs.ChunkResult, error) {
	if e == nil || e.service == nil {
		return domainjobs.ChunkResult{}, fmt.Errorf("interpret project executor not configured")
	}
	if run.Payload.ProjectID == nil {
		return domainjobs.ChunkResult{}, fmt.Errorf("project_id is required")
	}
	view, err := e.service.Run(ctx, run.UserID, *run.Payload.ProjectID, domaininterpretations.InterpretationRunInput{
		AccountID: run.AccountID,
		Trigger:   run.JobType,
	})
	if err != nil {
		return domainjobs.ChunkResult{}, err
	}
	count := 0
	if view != nil {
		count = len(view.Candidates)
	}
	return domainjobs.ChunkResult{
		ProgressDelta: domainjobs.JobProgress{
			Processed: 1,
			Detail: map[string]interface{}{
				"candidate_count": count,
			},
		},
		Done: true,
	}, nil
}

type reconcileProjectExecutor struct {
	service  *appreconcile.Service
	projects domainprojects.ProjectRepository
}

func (e *reconcileProjectExecutor) JobType() string { return domainjobs.TypeReconcileProject }

func (e *reconcileProjectExecutor) ExecuteChunk(ctx context.Context, run domainjobs.RunContext) (domainjobs.ChunkResult, error) {
	if e == nil || e.service == nil {
		return domainjobs.ChunkResult{}, fmt.Errorf("reconcile project executor not configured")
	}
	if run.Payload.ProjectID == nil {
		return domainjobs.ChunkResult{}, fmt.Errorf("project_id is required")
	}
	res, err := e.service.Run(ctx, run.UserID, *run.Payload.ProjectID, domaincontradictions.ReconciliationReconcileInput{})
	if err != nil {
		return domainjobs.ChunkResult{}, err
	}
	// Advance the extraction watermark only on success, so a failed run is
	// retried rather than skipping the correspondence it never reached.
	if e.projects != nil {
		if err := e.projects.MarkProjectExtracted(ctx, *run.Payload.ProjectID, time.Now().UTC()); err != nil {
			return domainjobs.ChunkResult{}, err
		}
	}
	return domainjobs.ChunkResult{
		ProgressDelta: domainjobs.JobProgress{
			Processed: res.ProcessedInterpretations,
			Detail: map[string]interface{}{
				"contradictions_opened": res.ContradictionsOpened,
			},
		},
		Done: true,
	}, nil
}

type projectAIExecutor struct {
	service *appprojectai.Service
}

func (e *projectAIExecutor) JobType() string { return domainjobs.TypeProjectAI }

func (e *projectAIExecutor) ExecuteChunk(ctx context.Context, run domainjobs.RunContext) (domainjobs.ChunkResult, error) {
	if e == nil || e.service == nil {
		return domainjobs.ChunkResult{}, fmt.Errorf("project ai executor not configured")
	}
	question := "Provide a concise project audit covering open issues, important facts, and recent correspondence."
	if run.Payload.ProjectID != nil {
		_, err := e.service.Ask(ctx, run.UserID, *run.Payload.ProjectID, question)
		if err != nil {
			return domainjobs.ChunkResult{}, err
		}
		return domainjobs.ChunkResult{ProgressDelta: domainjobs.JobProgress{Processed: 1}, Done: true}, nil
	}
	_, err := e.service.AskAcross(ctx, run.UserID, question)
	if err != nil {
		return domainjobs.ChunkResult{}, err
	}
	return domainjobs.ChunkResult{ProgressDelta: domainjobs.JobProgress{Processed: 1}, Done: true}, nil
}
