package composition

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	asynqadapter "github.com/Kapital-B/automata/svc/internal/adapters/inbound/asynq"
	httphandler "github.com/Kapital-B/automata/svc/internal/adapters/inbound/http"
	dynamodbjobs "github.com/Kapital-B/automata/svc/internal/adapters/outbound/dynamodbjobs"
	googleoauth "github.com/Kapital-B/automata/svc/internal/adapters/outbound/google"
	llmadapter "github.com/Kapital-B/automata/svc/internal/adapters/outbound/llm"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/microsoft"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/factory"
	pgmigrate "github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/migrate"
	pgrepo "github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/postgres"
	sqliterepo "github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
	slackadapter "github.com/Kapital-B/automata/svc/internal/adapters/outbound/slack"
	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	appattention "github.com/Kapital-B/automata/svc/internal/application/attention"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appconnectors "github.com/Kapital-B/automata/svc/internal/application/connectors"
	appcontacts "github.com/Kapital-B/automata/svc/internal/application/contacts"
	appdecisions "github.com/Kapital-B/automata/svc/internal/application/decisions"
	appfacts "github.com/Kapital-B/automata/svc/internal/application/facts"
	appinterpret "github.com/Kapital-B/automata/svc/internal/application/interpret"
	appissues "github.com/Kapital-B/automata/svc/internal/application/issues"
	appjobs "github.com/Kapital-B/automata/svc/internal/application/jobs"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojectai "github.com/Kapital-B/automata/svc/internal/application/projectai"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	appreconcile "github.com/Kapital-B/automata/svc/internal/application/reconcile"
	"github.com/Kapital-B/automata/svc/internal/configuration"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const msSignInScopes = "openid offline_access profile email"

type repository interface {
	driven.AccountRepository
	driven.ConnectorRepository
	driven.MessageRepository
	driven.OAuthStateRepository
	driven.UserRepository
	driven.AuthSessionRepository
	driven.ProjectRepository
	driven.ContactRepository
	driven.IssueRepository
	driven.FactRepository
	driven.InterpretationRepository
	driven.ContradictionRepository
	driven.DecisionRepository
	driven.ManualItemRepository
	driven.TimelineRepository
	driven.AssignmentRepository
	driven.SummaryRepository
	driven.ScheduleRepository
	driven.ForwardRepository
}

type Options struct {
	AutoMigrate    bool
	EnableAsynq    bool
	EnableJobStore bool
	LeaseOwner     string
}

type Runtime struct {
	Config      configuration.Config
	Log         *slog.Logger
	DB          *sql.DB
	Repository  repository
	Handlers    *httphandler.Handlers
	ChiRouter   *chi.Mux
	Router      http.Handler
	Scheduler   *appjobs.SchedulerService
	Execution   *appjobs.ExecutionService
	Enqueuer    *appjobs.Enqueuer
	Registry    *appjobs.Registry
	QueueClient *asynqadapter.QueueClient
	JobStore    driven.JobStore
}

func (r *Runtime) Close() error {
	var firstErr error
	if r == nil {
		return nil
	}
	if r.QueueClient != nil {
		if err := r.QueueClient.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if r.DB != nil {
		if err := r.DB.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func Build(ctx context.Context, log *slog.Logger, cfg configuration.Config, opts Options) (*Runtime, error) {
	if log == nil {
		log = slog.Default()
	}
	db, repo, err := openRepository(ctx, cfg, opts.AutoMigrate)
	if err != nil {
		return nil, err
	}
	rt := &Runtime{
		Config:     cfg,
		Log:        log,
		DB:         db,
		Repository: repo,
		Registry:   appjobs.DefaultRegistry(),
	}

	if opts.EnableAsynq && !cfg.JobsInline {
		rt.QueueClient = asynqadapter.NewQueueClient(cfg.RedisAddr, cfg.AsynqPrefix)
	}
	if opts.EnableJobStore && strings.TrimSpace(cfg.JobsTableName) != "" {
		store, err := buildJobStore(ctx, cfg)
		if err != nil {
			_ = rt.Close()
			return nil, err
		}
		rt.JobStore = store
		rt.Enqueuer = &appjobs.Enqueuer{Store: store, Registry: rt.Registry}
		leaseOwner := strings.TrimSpace(opts.LeaseOwner)
		if leaseOwner == "" {
			leaseOwner = "worker"
		}
		rt.Execution = &appjobs.ExecutionService{
			Store:       store,
			Executors:   nil,
			LeaseOwner:  leaseOwner,
			LeaseFor:    cfg.JobLeaseDuration,
			TerminalTTL: cfg.JobTerminalRetention,
		}
	}

	if err := rt.buildServices(ctx); err != nil {
		_ = rt.Close()
		return nil, err
	}
	return rt, nil
}

func ApplyMigrations(ctx context.Context, cfg configuration.Config) error {
	engine, err := factory.ParseEngine(cfg.DatabaseEngine)
	if err != nil {
		return err
	}
	db, err := factory.Open(ctx, factory.Config{
		Engine:           engine,
		DatabaseURL:      cfg.DatabaseURL,
		EnableForeignKey: true,
	})
	if err != nil {
		return err
	}
	defer db.Close()
	switch engine {
	case factory.EngineSQLite:
		return sqliterepo.Migrate(db)
	case factory.EnginePostgres, factory.EngineDSQL:
		return pgmigrate.Apply(ctx, db, engine)
	default:
		return fmt.Errorf("unsupported database engine %q", engine)
	}
}

func openRepository(ctx context.Context, cfg configuration.Config, autoMigrate bool) (*sql.DB, repository, error) {
	engine, err := factory.ParseEngine(cfg.DatabaseEngine)
	if err != nil {
		return nil, nil, err
	}
	db, err := factory.Open(ctx, factory.Config{
		Engine:           engine,
		DatabaseURL:      cfg.DatabaseURL,
		EnableForeignKey: true,
	})
	if err != nil {
		return nil, nil, err
	}
	if autoMigrate {
		switch engine {
		case factory.EngineSQLite:
			if err := sqliterepo.Migrate(db); err != nil {
				_ = db.Close()
				return nil, nil, err
			}
		case factory.EnginePostgres, factory.EngineDSQL:
			if err := pgmigrate.Apply(ctx, db, engine); err != nil {
				_ = db.Close()
				return nil, nil, err
			}
		default:
			_ = db.Close()
			return nil, nil, fmt.Errorf("unsupported database engine %q", engine)
		}
	}
	switch engine {
	case factory.EngineSQLite:
		return db, sqliterepo.NewRepository(db, cfg.OAuthStateTTL), nil
	case factory.EnginePostgres, factory.EngineDSQL:
		return db, pgrepo.NewRepository(db, cfg.OAuthStateTTL), nil
	default:
		_ = db.Close()
		return nil, nil, fmt.Errorf("unsupported database engine %q", engine)
	}
}

func buildJobStore(ctx context.Context, cfg configuration.Config) (driven.JobStore, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		return nil, err
	}
	client := dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
		if cfg.AWSEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.AWSEndpoint)
		}
	})
	return dynamodbjobs.NewStore(client, cfg.JobsTableName, cfg.JobCursorSecret), nil
}

func (r *Runtime) buildServices(ctx context.Context) error {
	vault, err := security.NewAESGCMVault(r.Config.EncryptionKey)
	if err != nil {
		return err
	}
	msMailOAuth := &microsoft.OAuth{
		ClientID:     r.Config.MSClientID,
		ClientSecret: r.Config.MSClientSecret,
		RedirectURI:  r.Config.MSRedirectURI,
	}
	msAuthOAuth := &microsoft.OAuth{
		ClientID:     r.Config.MSClientID,
		ClientSecret: r.Config.MSClientSecret,
		RedirectURI:  r.Config.MSAuthRedirectURI,
		Scopes:       msSignInScopes,
	}
	graph := &microsoft.GraphClient{}
	repo := r.Repository
	jobRuns := resolveJobRuns(repo)

	accountSvc := appaccounts.NewService(appaccounts.Deps{
		Accounts:    repo,
		OAuthState:  repo,
		JobRuns:     jobRuns,
		OAuth:       msMailOAuth,
		Graph:       graph,
		Vault:       vault,
		Dashboard:   r.Config.DashboardBaseURL,
		SuccessPath: r.Config.OAuthSuccessPath,
		ErrorPath:   r.Config.OAuthErrorPath,
		StateTTL:    r.Config.OAuthStateTTL,
	})
	slackClient := &slackadapter.Client{
		ClientID:     r.Config.SlackClientID,
		ClientSecret: r.Config.SlackClientSecret,
		RedirectURI:  r.Config.SlackRedirectURI,
		Mode:         r.Config.SlackMode,
	}
	connectorSvc := &appconnectors.Service{
		Connectors: repo,
		OAuthState: repo,
		Users:      repo,
		Projects:   repo,
		Slack:      slackClient,
		Vault:      vault,
		JobRuns:    jobRuns,
	}
	resolveSvc := &appcontacts.ResolveService{
		Users:    repo,
		Messages: repo,
		Contacts: repo,
	}
	contactSvc := &appcontacts.Service{
		Users:    repo,
		Contacts: repo,
		Messages: repo,
	}
	projectSvc := &appprojects.Service{
		Users:       repo,
		Projects:    repo,
		Assignments: repo,
		Manuals:     repo,
		Timeline:    repo,
		Contacts:    repo,
		Messages:    repo,
	}
	issueSvc := &appissues.Service{
		Users:       repo,
		Projects:    repo,
		Issues:      repo,
		Assignments: repo,
		Manuals:     repo,
		Contacts:    repo,
		Messages:    repo,
		Timeline:    repo,
	}
	factSvc := &appfacts.Service{
		Users:       repo,
		Projects:    repo,
		Facts:       repo,
		Issues:      repo,
		Assignments: repo,
		Manuals:     repo,
		Messages:    repo,
	}
	decisionSvc := &appdecisions.Service{
		Users:       repo,
		Projects:    repo,
		Decisions:   repo,
		Issues:      repo,
		Assignments: repo,
		Manuals:     repo,
		Messages:    repo,
	}
	interpretSvc := &appinterpret.Service{
		Users:           repo,
		Projects:        repo,
		Interpretations: repo,
		Facts:           repo,
		Timeline:        repo,
		Assignments:     repo,
		Manuals:         repo,
		Messages:        repo,
		Connectors:      repo,
		JobRuns:         jobRuns,
	}
	projectSvc.AfterProjectCorrespondence = func(ctx context.Context, userID, projectID uuid.UUID, messageID, manualItemID *uuid.UUID) {
		in := appinterpret.RunInput{Trigger: "api"}
		if messageID != nil {
			in.MessageIDs = []uuid.UUID{*messageID}
		}
		if manualItemID != nil {
			in.ManualItemIDs = []uuid.UUID{*manualItemID}
		}
		interpretSvc.TryRunBestEffort(ctx, userID, projectID, in)
	}
	reconcileSvc := &appreconcile.Service{
		Users:           repo,
		Projects:        repo,
		Interpretations: repo,
		FactsRepo:       repo,
		Facts:           factSvc,
		Decisions:       decisionSvc,
		Contradictions:  repo,
		JobRuns:         jobRuns,
	}
	attentionSvc := &appattention.Service{
		Users:          repo,
		Projects:       repo,
		Issues:         repo,
		Facts:          repo,
		Decisions:      repo,
		Contradictions: repo,
		Summaries:      repo,
	}
	projectAISvc := &appprojectai.Service{
		Users:     repo,
		Projects:  repo,
		Facts:     repo,
		Decisions: repo,
		Issues:    repo,
		Timeline:  repo,
		JobRuns:   jobRuns,
		Attention: attentionSvc,
	}
	assignSvc := &appprojects.AssignService{
		Users:       repo,
		Projects:    repo,
		Assignments: repo,
		Contacts:    repo,
		Messages:    repo,
		JobRuns:     jobRuns,
	}
	syncSvc := &appmessages.SyncService{
		Accounts: repo,
		Messages: repo,
		OAuth:    msMailOAuth,
		Graph:    graph,
		Vault:    vault,
		JobRuns:  jobRuns,
		Resolve:  resolveSvc,
		Assign:   assignSvc,
	}

	var categorizeSvc *appmessages.CategorizeService
	var summarizeSvc *appmessages.SummarizeService
	var autoDraftSvc *appmessages.AutoDraftService
	var forwardRulesSvc *appmessages.ForwardRulesService

	llmClient, llmLabel, err := buildLLMClient(ctx, r.Config)
	if err != nil {
		return err
	}
	if llmClient != nil {
		issueSvc.LLM = llmClient
		interpretSvc.LLM = llmClient
		projectAISvc.LLM = llmClient
		categorizeSvc = &appmessages.CategorizeService{
			Messages: repo,
			LLM:      llmClient,
			JobRuns:  jobRuns,
		}
		summarizeSvc = &appmessages.SummarizeService{
			Messages:  repo,
			Summaries: repo,
			LLM:       llmClient,
			JobRuns:   jobRuns,
		}
		autoDraftSvc = &appmessages.AutoDraftService{
			Messages:   repo,
			Summaries:  repo,
			LLM:        llmClient,
			JobRuns:    jobRuns,
			ModelLabel: llmLabel,
		}
		forwardRulesSvc = &appmessages.ForwardRulesService{
			Messages:  repo,
			Forwards:  repo,
			Accounts:  repo,
			OAuth:     msMailOAuth,
			Graph:     graph,
			Vault:     vault,
			LLM:       llmClient,
			JobRuns:   jobRuns,
			ModelName: llmLabel,
		}
	} else {
		forwardRulesSvc = &appmessages.ForwardRulesService{
			Messages: repo,
			Forwards: repo,
			Accounts: repo,
			OAuth:    msMailOAuth,
			Graph:    graph,
			Vault:    vault,
			JobRuns:  jobRuns,
		}
	}

	var googleClient *googleoauth.OAuth
	if r.Config.GoogleClientID != "" {
		googleClient = &googleoauth.OAuth{
			ClientID:     r.Config.GoogleClientID,
			ClientSecret: r.Config.GoogleClientSecret,
			RedirectURI:  r.Config.GoogleRedirectURI,
		}
	}
	authSvc := auth.NewService(repo, repo, repo, msAuthOAuth, googleClient, r.Config.JWTSecret, r.Config.JWTTTL, r.Config.RefreshTTL)

	if r.Execution != nil {
		r.Execution.Executors = appjobs.NewExecutorRegistry(appjobs.ExecutorDeps{
			Store:            r.JobStore,
			Sync:             syncSvc,
			SyncSlack:        connectorSvc,
			Categorize:       categorizeSvc,
			Summarize:        summarizeSvc,
			DraftSuggest:     autoDraftSvc,
			ForwardRules:     forwardRulesSvc,
			ResolveContacts:  resolveSvc,
			AssignProjects:   assignSvc,
			InterpretProject: interpretSvc,
			ReconcileProject: reconcileSvc,
			ProjectAI:        projectAISvc,
		})
	}
	r.Scheduler = &appjobs.SchedulerService{
		OAuthStates:       repo,
		Schedules:         repo,
		Accounts:          repo,
		Store:             r.JobStore,
		Enqueuer:          r.Enqueuer,
		Registry:          r.Registry,
		OAuthStateTTL:     r.Config.OAuthStateTTL,
		PendingWakeAfter:  r.Config.JobPendingWakeAfter,
		ScheduleBatchSize: appjobs.DefaultSchedulerBatchLimit,
	}

	h := &httphandler.Handlers{
		Log:                  r.Log,
		AccountSvc:           accountSvc,
		ConnectorSvc:         connectorSvc,
		SyncSvc:              syncSvc,
		CategorizeSvc:        categorizeSvc,
		SummarizeSvc:         summarizeSvc,
		AutoDraftSvc:         autoDraftSvc,
		DraftsSvc:            &appmessages.DraftLifecycleService{Summaries: repo, Messages: repo, Accounts: repo, OAuth: msMailOAuth, Graph: graph, Vault: vault},
		ForwardRulesSvc:      forwardRulesSvc,
		AuthSvc:              authSvc,
		ContactSvc:           contactSvc,
		ProjectSvc:           projectSvc,
		IssueSvc:             issueSvc,
		FactSvc:              factSvc,
		InterpretSvc:         interpretSvc,
		ReconcileSvc:         reconcileSvc,
		DecisionSvc:          decisionSvc,
		AttentionSvc:         attentionSvc,
		ProjectAISvc:         projectAISvc,
		Accounts:             repo,
		Messages:             repo,
		JobRuns:              jobRuns,
		JobStore:             r.JobStore,
		JobEnqueuer:          r.Enqueuer,
		JobTerminalTTL:       r.Config.JobTerminalRetention,
		JobsInline:           r.Config.JobsInline,
		Summaries:            repo,
		Forwards:             repo,
		Schedules:            repo,
		OAuthStates:          repo,
		Users:                repo,
		Contacts:             repo,
		Projects:             repo,
		Assignments:          repo,
		Issues:               repo,
		Dashboard:            r.Config.DashboardBaseURL,
		SuccessPath:          r.Config.OAuthSuccessPath,
		ConnectorSuccessPath: r.Config.SlackSuccessPath,
		ErrorPath:            r.Config.OAuthErrorPath,
		AuthSuccessPath:      r.Config.AuthSuccessPath,
		AuthErrorPath:        r.Config.AuthErrorPath,
		StateTTL:             r.Config.OAuthStateTTL,
		JWTSecret:            r.Config.JWTSecret,
		JWTTTL:               r.Config.JWTTTL,
		DefaultUserID:        r.Config.DefaultUserID,
		JobQueue:             r.QueueClient,
		CORSOrigins:          r.Config.CORSOrigins,
	}
	r.Handlers = h
	router := h.Routes()
	if chiRouter, ok := router.(*chi.Mux); ok {
		r.ChiRouter = chiRouter
	}
	r.Router = router
	return nil
}

func buildLLMClient(ctx context.Context, cfg configuration.Config) (driven.LLMClient, string, error) {
	if cfg.LLMBaseURL != "" && cfg.LLMModel != "" {
		return &llmadapter.OpenAIClient{
			BaseURL: cfg.LLMBaseURL,
			Model:   cfg.LLMModel,
			APIKey:  cfg.LLMAPIKey,
		}, cfg.LLMModel, nil
	}
	if strings.TrimSpace(cfg.BedrockModelID) == "" {
		return nil, "", nil
	}
	client, err := llmadapter.NewBedrockClient(ctx, llmadapter.BedrockConfig{
		Region:   cfg.AWSRegion,
		Endpoint: cfg.BedrockRuntimeEndpoint,
		ModelID:  cfg.BedrockModelID,
	})
	if err != nil {
		return nil, "", err
	}
	return client, cfg.BedrockModelID, nil
}
