// Package driven provides temporary inward aliases during the domain-port migration.
package driven

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
	domainshared "github.com/Kapital-B/automata/svc/internal/domain/shared"

	"context"
	"github.com/google/uuid"
)

type GoogleOAuth = domainauth.GoogleOAuth
type GoogleTokenResult = domainauth.GoogleTokenResult

const JobStatusPending = domainjobs.JobStatusPending
const JobStatusRunning = domainjobs.JobStatusRunning
const JobStatusSuccess = domainjobs.JobStatusSuccess
const JobStatusFailed = domainjobs.JobStatusFailed
const JobStatusCancelled = domainjobs.JobStatusCancelled
const JobTriggerAPI = domainjobs.JobTriggerAPI
const JobTriggerSchedule = domainjobs.JobTriggerSchedule
const EffectClaimed = domainjobs.EffectClaimed
const EffectSucceededPendingAudit = domainjobs.EffectSucceededPendingAudit
const EffectRetryable = domainjobs.EffectRetryable
const EffectRejected = domainjobs.EffectRejected
const EffectUnknown = domainjobs.EffectUnknown
const EffectSucceeded = domainjobs.EffectSucceeded

var ErrJobNotFound = domainjobs.ErrJobNotFound
var ErrJobConflict = domainjobs.ErrJobConflict
var ErrJobLockHeld = domainjobs.ErrJobLockHeld
var ErrEffectAlreadyClaimed = domainjobs.ErrEffectAlreadyClaimed
var ErrOffsetNotSupported = domainjobs.ErrOffsetNotSupported

type JobCursor = domainjobs.JobCursor
type JobProgress = domainjobs.JobProgress
type JobPayload = domainjobs.JobPayload
type JobRecord = domainjobs.JobRecord
type CreateJobInput = domainjobs.CreateJobInput
type JobListFilter = domainjobs.JobListFilter
type JobListPage = domainjobs.JobListPage
type EffectRecord = domainjobs.EffectRecord
type ClaimEffectInput = domainjobs.ClaimEffectInput
type ChunkResult = domainjobs.ChunkResult
type RunContext = domainjobs.RunContext
type JobStore = domainjobs.JobStore
type JobEnqueuer = domainjobs.JobEnqueuer
type JobExecutor = domainjobs.JobExecutor
type JobExecutionPort = domainjobs.JobExecutionPort
type LLMMessage = domainai.LLMMessage
type LLMRequestOptions = domainai.LLMRequestOptions
type LLMUsage = domainai.LLMUsage
type LLMResponse = domainai.LLMResponse
type LLMClient = domainai.LLMClient
type LLMClientWithOptions = domainai.LLMClientWithOptions
type LLMUsageRow = domainai.LLMUsageRow
type LLMUsageTotal = domainai.LLMUsageTotal
type LLMUsageRepository = domainai.LLMUsageRepository

var ErrMailNotSent = domainaccounts.ErrMailNotSent
var ErrMailTooLarge = domainaccounts.ErrMailTooLarge
var ErrCursorExpired = domainaccounts.ErrCursorExpired
var ErrCredentialsRejected = domainaccounts.ErrCredentialsRejected
var ErrUnsupportedProvider = domainaccounts.ErrUnsupportedProvider

type MailboxCapabilities = domainaccounts.MailboxCapabilities
type MailMessage = domainaccounts.MailMessage
type MailRecipient = domainaccounts.MailRecipient
type MailChangePage = domainaccounts.MailChangePage
type Mailbox = domainaccounts.Mailbox
type MailProvider = domainaccounts.MailProvider
type ConnectOptions = domainaccounts.ConnectOptions
type ConnectedMailbox = domainaccounts.ConnectedMailbox
type OAuthMailConnector = domainaccounts.OAuthMailConnector
type MailServer = domainaccounts.MailServer
type PasswordConnectRequest = domainaccounts.PasswordConnectRequest

var ErrConnectRejected = domainaccounts.ErrConnectRejected

type ConnectRejectedError = domainaccounts.ConnectRejectedError
type PasswordMailConnector = domainaccounts.PasswordMailConnector
type MessageSearchFilter = domainmessages.MessageSearchFilter
type MessageSearchHit = domainmessages.MessageSearchHit
type MessageSearchRepository = domainmessages.MessageSearchRepository
type TokenPair = domainaccounts.TokenPair
type MicrosoftOAuth = domainaccounts.MicrosoftOAuth
type TokenVault = domainshared.TokenVault
type AccountRow = domainaccounts.AccountRow
type ConnectorAccountRow = domainconnectors.ConnectorAccountRow
type ConnectorBindingRow = domainconnectors.ConnectorBindingRow
type ConnectorMessageRow = domainconnectors.ConnectorMessageRow
type MessageRow = domainmessages.MessageRow
type CategoryDefinitionRow = domainmessages.CategoryDefinitionRow
type MessageCategoryRow = domainmessages.MessageCategoryRow
type MessageListFilter = domainmessages.MessageListFilter
type JobRunRow = domainjobs.JobRunRow
type JobRunListFilter = domainjobs.JobRunListFilter
type SummarySettingsRow = domainmessages.SummarySettingsRow
type SummarySnapshotRow = domainmessages.SummarySnapshotRow
type SummaryJobChunkRow = domainmessages.SummaryJobChunkRow
type ActionItemRow = domainmessages.ActionItemRow
type ProjectTodoRow = domainmessages.ProjectTodoRow
type FYIRow = domainmessages.FYIRow
type DraftSuggestionRow = domainmessages.DraftSuggestionRow
type SendAttemptRow = domainmessages.SendAttemptRow
type ScheduleChainRow = domainjobs.ScheduleChainRow
type ForwardAllowlistRow = domainmessages.ForwardAllowlistRow
type ForwardRuleRow = domainmessages.ForwardRuleRow
type ForwardAuditRow = domainmessages.ForwardAuditRow
type ForwardCandidateCursor = domainmessages.ForwardCandidateCursor
type ForwardRuleStats = domainmessages.ForwardRuleStats
type ForwardActivityRow = domainmessages.ForwardActivityRow
type ManualForwardAuditRow = domainmessages.ManualForwardAuditRow
type AccountRepository = domainaccounts.AccountRepository
type ConnectorRepository = domainconnectors.ConnectorRepository
type MessageRepository = domainmessages.MessageRepository
type OAuthStateRepository = domainauth.OAuthStateRepository
type UserRepository = domainauth.UserRepository
type OrganisationRow = domainorganisations.OrganisationRow
type OrganisationRepository = domainorganisations.OrganisationRepository
type ContactRow = domaincontacts.ContactRow
type AssignCandidateFilter = domainprojects.AssignCandidateFilter
type ContactIdentityRow = domaincontacts.ContactIdentityRow
type CorrespondenceParticipantRow = domaincontacts.CorrespondenceParticipantRow
type ContactListFilter = domaincontacts.ContactListFilter
type ContactRepository = domaincontacts.ContactRepository
type ProjectRow = domainprojects.ProjectRow
type ProjectMemberRow = domainprojects.ProjectMemberRow
type AttentionRow = domainprojects.AttentionRow
type DueProject = domainprojects.DueProject
type ActivityItem = domainprojects.ActivityItem
type ActivityFilter = domainprojects.ActivityFilter
type OverviewCounts = domainprojects.OverviewCounts
type OverviewProject = domainprojects.OverviewProject
type ProjectListFilter = domainprojects.ProjectListFilter
type ProjectRepository = domainprojects.ProjectRepository
type AssignmentRow = domainprojects.AssignmentRow
type EffectiveAssignment = domainprojects.EffectiveAssignment
type UnassignedItem = domainprojects.UnassignedItem
type UnassignedListFilter = domainprojects.UnassignedListFilter
type UnassignedSummary = domainprojects.UnassignedSummary
type ManualItemRow = domainprojects.ManualItemRow
type TimelineContact = domainprojects.TimelineContact
type TimelineItem = domainprojects.TimelineItem
type TimelineFilter = domainprojects.TimelineFilter
type IssueRow = domainissues.IssueRow
type IssueItemRow = domainissues.IssueItemRow
type IssueLink = domainissues.IssueLink
type IssueRepository = domainissues.IssueRepository
type FactRow = domainfacts.FactRow
type FactVersionRow = domainfacts.FactVersionRow
type FactEvidenceRow = domainfacts.FactEvidenceRow
type FactRepository = domainfacts.FactRepository
type InterpretationRow = domaininterpretations.InterpretationRow
type InterpretationSourceRow = domaininterpretations.InterpretationSourceRow
type InterpretationRepository = domaininterpretations.InterpretationRepository
type ContradictionRow = domaincontradictions.ContradictionRow
type ContradictionSideRow = domaincontradictions.ContradictionSideRow
type ContradictionRepository = domaincontradictions.ContradictionRepository
type DecisionRow = domaindecisions.DecisionRow
type DecisionEvidenceRow = domaindecisions.DecisionEvidenceRow
type DecisionRepository = domaindecisions.DecisionRepository
type ManualItemRepository = domainprojects.ManualItemRepository
type TimelineRepository = domainprojects.TimelineRepository
type AssignmentRepository = domainprojects.AssignmentRepository
type JobRunRepository = domainjobs.JobRunRepository
type SummaryRepository = domainmessages.SummaryRepository
type ScheduleRepository = domainjobs.ScheduleRepository
type ForwardRepository = domainmessages.ForwardRepository
type AuthSessionRepository = domainauth.AuthSessionRepository
type SlackOAuthResult = domainconnectors.SlackOAuthResult
type SlackConversation = domainconnectors.SlackConversation
type SlackMessage = domainconnectors.SlackMessage
type SlackHistoryPage = domainconnectors.SlackHistoryPage
type SlackClient = domainconnectors.SlackClient

func ChatCompletion(ctx context.Context, client LLMClient, messages []LLMMessage, opts LLMRequestOptions) (*LLMResponse, error) {
	return domainai.ChatCompletion(ctx, client, messages, opts)
}
func WithUsageScope(ctx context.Context, userID, accountID *uuid.UUID) context.Context {
	return domainai.WithUsageScope(ctx, userID, accountID)
}
func UsageScopeFrom(ctx context.Context) (*uuid.UUID, *uuid.UUID) {
	return domainai.UsageScopeFrom(ctx)
}
func TooLarge(limit int64) error { return domainaccounts.TooLarge(limit) }
func ConnectRejected(format string, args ...any) error {
	return domainaccounts.ConnectRejected(format, args...)
}
