package reconcile

import domaincontradictions "github.com/Kapital-B/automata/svc/internal/domain/contradictions"

var _ domaincontradictions.ReconciliationUseCases = (*Service)(nil)
