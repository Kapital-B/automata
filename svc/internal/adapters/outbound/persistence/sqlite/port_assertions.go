package sqlite

import (
	domainaccounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	domainai "github.com/Kapital-B/automata/svc/internal/domain/ai"
	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
	domainconnectors "github.com/Kapital-B/automata/svc/internal/domain/connectors"
	domaincontacts "github.com/Kapital-B/automata/svc/internal/domain/contacts"
	domaincontradictions "github.com/Kapital-B/automata/svc/internal/domain/contradictions"
	domaindecisions "github.com/Kapital-B/automata/svc/internal/domain/decisions"
	domainfacts "github.com/Kapital-B/automata/svc/internal/domain/facts"
	domaininterpretations "github.com/Kapital-B/automata/svc/internal/domain/interpretations"
	domainissues "github.com/Kapital-B/automata/svc/internal/domain/issues"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	domainorganisations "github.com/Kapital-B/automata/svc/internal/domain/organisations"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
)

var (
	_ domainaccounts.AccountRepository               = (*Repository)(nil)
	_ domainprojects.AssignmentRepository            = (*Repository)(nil)
	_ domainauth.AuthSessionRepository               = (*Repository)(nil)
	_ domainconnectors.ConnectorRepository           = (*Repository)(nil)
	_ domaincontacts.ContactRepository               = (*Repository)(nil)
	_ domaincontradictions.ContradictionRepository   = (*Repository)(nil)
	_ domaindecisions.DecisionRepository             = (*Repository)(nil)
	_ domainfacts.FactRepository                     = (*Repository)(nil)
	_ domainmessages.ForwardRepository               = (*Repository)(nil)
	_ domaininterpretations.InterpretationRepository = (*Repository)(nil)
	_ domainissues.IssueRepository                   = (*Repository)(nil)
	_ domainjobs.JobRunRepository                    = (*Repository)(nil)
	_ domainai.LLMUsageRepository                    = (*Repository)(nil)
	_ domainprojects.ManualItemRepository            = (*Repository)(nil)
	_ domainmessages.MessageRepository               = (*Repository)(nil)
	_ domainmessages.MessageSearchRepository         = (*Repository)(nil)
	_ domainauth.OAuthStateRepository                = (*Repository)(nil)
	_ domainorganisations.OrganisationRepository     = (*Repository)(nil)
	_ domainprojects.ProjectRepository               = (*Repository)(nil)
	_ domainjobs.ScheduleRepository                  = (*Repository)(nil)
	_ domainmessages.SummaryRepository               = (*Repository)(nil)
	_ domainprojects.TimelineRepository              = (*Repository)(nil)
	_ domainauth.UserRepository                      = (*Repository)(nil)
)
