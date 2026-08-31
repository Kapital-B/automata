package jobs

import (
	"context"
	"fmt"

	appconnectors "github.com/Kapital-B/automata/svc/internal/application/connectors"
	appcontacts "github.com/Kapital-B/automata/svc/internal/application/contacts"
	appinterpret "github.com/Kapital-B/automata/svc/internal/application/interpret"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojectai "github.com/Kapital-B/automata/svc/internal/application/projectai"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	appreconcile "github.com/Kapital-B/automata/svc/internal/application/reconcile"
)

type ExecutorDeps struct {
	Store            driven.JobStore
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
	ProjectAI        *appprojectai.Service
}

func NewExecutorRegistry(deps ExecutorDeps) ExecutorRegistry {
	reg := ExecutorRegistry{}
	if deps.Sync != nil {
		reg[TypeSync] = &appmessages.SyncExecutor{Service: deps.Sync}
	}
	if deps.SyncSlack != nil {
		reg[TypeSyncSlack] = &appconnectors.SyncSlackExecutor{Service: deps.SyncSlack}
	}
	if deps.Categorize != nil {
		reg[TypeCategorize] = &appmessages.CategorizeExecutor{Service: deps.Categorize}
	}
	if deps.Summarize != nil {
		reg[TypeSummarize] = &appmessages.SummarizeExecutor{Service: deps.Summarize}
	}
	if deps.DraftSuggest != nil {
		reg[TypeDraftSuggest] = &appmessages.DraftSuggestExecutor{Service: deps.DraftSuggest}
	}
	if deps.ForwardRules != nil && deps.Store != nil {
		reg[TypeForwardRules] = &appmessages.ForwardRulesExecutor{Service: deps.ForwardRules, Store: deps.Store}
	}
	if deps.ResolveContacts != nil {
		reg[TypeResolveContacts] = &appcontacts.ResolveContactsExecutor{Service: deps.ResolveContacts}
	}
	if deps.AssignProjects != nil {
		reg[TypeAssignProjects] = &appprojects.AssignProjectsExecutor{Service: deps.AssignProjects}
	}
	if deps.InterpretProject != nil {
		reg[TypeInterpretProject] = &interpretProjectExecutor{service: deps.InterpretProject}
	}
	if deps.ReconcileProject != nil {
		reg[TypeReconcileProject] = &reconcileProjectExecutor{service: deps.ReconcileProject}
	}
	if deps.ProjectAI != nil {
		reg[TypeProjectAI] = &projectAIExecutor{service: deps.ProjectAI}
	}
	return reg
}

type interpretProjectExecutor struct {
	service *appinterpret.Service
}

func (e *interpretProjectExecutor) JobType() string { return TypeInterpretProject }

func (e *interpretProjectExecutor) ExecuteChunk(ctx context.Context, run driven.RunContext) (driven.ChunkResult, error) {
	if e == nil || e.service == nil {
		return driven.ChunkResult{}, fmt.Errorf("interpret project executor not configured")
	}
	if run.Payload.ProjectID == nil {
		return driven.ChunkResult{}, fmt.Errorf("project_id is required")
	}
	view, err := e.service.Run(ctx, run.UserID, *run.Payload.ProjectID, appinterpret.RunInput{
		AccountID: run.AccountID,
		Trigger:   run.JobType,
	})
	if err != nil {
		return driven.ChunkResult{}, err
	}
	count := 0
	if view != nil {
		count = len(view.Candidates)
	}
	return driven.ChunkResult{
		ProgressDelta: driven.JobProgress{
			Processed: 1,
			Detail: map[string]interface{}{
				"candidate_count": count,
			},
		},
		Done: true,
	}, nil
}

type reconcileProjectExecutor struct {
	service *appreconcile.Service
}

func (e *reconcileProjectExecutor) JobType() string { return TypeReconcileProject }

func (e *reconcileProjectExecutor) ExecuteChunk(ctx context.Context, run driven.RunContext) (driven.ChunkResult, error) {
	if e == nil || e.service == nil {
		return driven.ChunkResult{}, fmt.Errorf("reconcile project executor not configured")
	}
	if run.Payload.ProjectID == nil {
		return driven.ChunkResult{}, fmt.Errorf("project_id is required")
	}
	res, err := e.service.Run(ctx, run.UserID, *run.Payload.ProjectID, appreconcile.ReconcileInput{})
	if err != nil {
		return driven.ChunkResult{}, err
	}
	return driven.ChunkResult{
		ProgressDelta: driven.JobProgress{
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

func (e *projectAIExecutor) JobType() string { return TypeProjectAI }

func (e *projectAIExecutor) ExecuteChunk(ctx context.Context, run driven.RunContext) (driven.ChunkResult, error) {
	if e == nil || e.service == nil {
		return driven.ChunkResult{}, fmt.Errorf("project ai executor not configured")
	}
	question := "Provide a concise project audit covering open issues, important facts, and recent correspondence."
	if run.Payload.ProjectID != nil {
		_, err := e.service.Ask(ctx, run.UserID, *run.Payload.ProjectID, question)
		if err != nil {
			return driven.ChunkResult{}, err
		}
		return driven.ChunkResult{ProgressDelta: driven.JobProgress{Processed: 1}, Done: true}, nil
	}
	_, err := e.service.AskAcross(ctx, run.UserID, question)
	if err != nil {
		return driven.ChunkResult{}, err
	}
	return driven.ChunkResult{ProgressDelta: driven.JobProgress{Processed: 1}, Done: true}, nil
}
