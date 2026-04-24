package sqlite

import (
	"database/sql"
	"embed"
	"strings"
)

//go:embed *.sql
var migrationFS embed.FS

// Migrate applies embedded SQL migrations in lexical order.
func Migrate(db *sql.DB) error {
	entries, err := migrationFS.ReadDir(".")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := migrationFS.ReadFile(e.Name())
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(b)); err != nil {
			return err
		}
	}
	return nil
}
