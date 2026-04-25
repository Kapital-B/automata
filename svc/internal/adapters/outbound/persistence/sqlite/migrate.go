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
		for _, stmt := range strings.Split(string(b), ";") {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				if ignoreRepeatedMigrationError(err) {
					continue
				}
				return err
			}
		}
	}
	return nil
}

func ignoreRepeatedMigrationError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column name: user_id")
}
