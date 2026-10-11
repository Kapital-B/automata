package security

import domain "github.com/Kapital-B/automata/svc/internal/domain/shared"

var _ domain.TokenVault = (*AESGCMVault)(nil)
