package jobs

import (
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
)

const TypeSync = domainjobs.TypeSync
const TypeSyncSlack = domainjobs.TypeSyncSlack
const TypeCategorize = domainjobs.TypeCategorize
const TypeSummarize = domainjobs.TypeSummarize
const TypeDraftSuggest = domainjobs.TypeDraftSuggest
const TypeForwardRules = domainjobs.TypeForwardRules
const TypeResolveContacts = domainjobs.TypeResolveContacts
const TypeAssignProjects = domainjobs.TypeAssignProjects
const TypeInterpretProject = domainjobs.TypeInterpretProject
const TypeReconcileProject = domainjobs.TypeReconcileProject
const TypeProjectAI = domainjobs.TypeProjectAI

var ErrUnknownJobType = domainjobs.ErrUnknownJobType
var ErrReservedJobType = domainjobs.ErrReservedJobType

type JobMode = domainjobs.JobMode

const ModeStreamed = domainjobs.ModeStreamed
const ModeSync = domainjobs.ModeSync

type CursorKind = domainjobs.CursorKind

const CursorGraphNextLink = domainjobs.CursorGraphNextLink
const CursorSlackHistory = domainjobs.CursorSlackHistory
const CursorMessageKeyset = domainjobs.CursorMessageKeyset
const CursorSummaryMap = domainjobs.CursorSummaryMap
const CursorNone = domainjobs.CursorNone

type RetryClass = domainjobs.RetryClass

const RetryTransient = domainjobs.RetryTransient
const RetryTerminal = domainjobs.RetryTerminal
const RetryEffect = domainjobs.RetryEffect

type EffectPolicy = domainjobs.EffectPolicy

const EffectNone = domainjobs.EffectNone
const EffectAtMostOnce = domainjobs.EffectAtMostOnce

var ReservedJobTypes = domainjobs.ReservedJobTypes

var enqueueAliases = domainjobs.EnqueueAliases

const SummarizeMaxPartials = domainjobs.SummarizeMaxPartials
const SummarizeMaxMessages = domainjobs.SummarizeMaxMessages

const DefaultTerminalTTL = domainjobs.DefaultTerminalTTL

type Definition = domainjobs.Definition

var DefaultMailboxChain = domainjobs.DefaultMailboxChain

type Registry = domainjobs.Registry

var DefaultRegistry = domainjobs.DefaultRegistry

var normalizeCanonicalType = domainjobs.NormalizeCanonicalType

var normalizeEnqueueType = domainjobs.NormalizeEnqueueType

var SchedulableJobs = domainjobs.SchedulableJobs

var ErrNotSchedulable = domainjobs.ErrNotSchedulable

var ValidateType = domainjobs.ValidateType

var ValidateChain = domainjobs.ValidateChain

var SummarizeMapBatchSize = domainjobs.SummarizeMapBatchSize

var ChainNamespace = domainjobs.ChainNamespace

var DeterministicJobID = domainjobs.DeterministicJobID

var DeterministicScheduleJobID = domainjobs.DeterministicScheduleJobID

var DeterministicChainID = domainjobs.DeterministicChainID
