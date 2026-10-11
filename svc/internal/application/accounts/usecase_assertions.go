package accounts

import domainaccounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"

var _ domainaccounts.AccountUseCases = (*Service)(nil)
