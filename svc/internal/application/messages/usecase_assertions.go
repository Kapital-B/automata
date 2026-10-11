package messages

import domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"

var _ domainmessages.MessageAutoDraftUseCases = (*AutoDraftService)(nil)
var _ domainmessages.MessageCategorizeUseCases = (*CategorizeService)(nil)
var _ domainmessages.MessageDraftLifecycleUseCases = (*DraftLifecycleService)(nil)
var _ domainmessages.MessageForwardRulesUseCases = (*ForwardRulesService)(nil)
var _ domainmessages.MessageSummarizeUseCases = (*SummarizeService)(nil)
var _ domainmessages.MessageSyncUseCases = (*SyncService)(nil)
