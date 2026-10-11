package contacts

import domaincontacts "github.com/Kapital-B/automata/svc/internal/domain/contacts"

var _ domaincontacts.ContactResolveUseCases = (*ResolveService)(nil)
var _ domaincontacts.ContactUseCases = (*Service)(nil)
