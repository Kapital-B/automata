package auth

import domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"

var _ domainauth.AuthUseCases = (*Service)(nil)
