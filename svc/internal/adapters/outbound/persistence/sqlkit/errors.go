package sqlkit

import (
	"database/sql"
	"errors"
	domainshared "github.com/Kapital-B/automata/svc/internal/domain/shared"
)

// DomainError translates error-based absence at the repository boundary.
// Nil/boolean/no-op results have already been handled inside the adapter.
func DomainError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domainshared.ErrNotFound
	}
	return err
}
