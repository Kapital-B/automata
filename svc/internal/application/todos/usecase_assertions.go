package todos

import domainissues "github.com/Kapital-B/automata/svc/internal/domain/issues"

var _ domainissues.TodoUseCases = (*Service)(nil)
