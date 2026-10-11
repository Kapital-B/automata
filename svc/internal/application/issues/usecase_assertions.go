package issues

import domainissues "github.com/Kapital-B/automata/svc/internal/domain/issues"

var _ domainissues.IssueUseCases = (*Service)(nil)
