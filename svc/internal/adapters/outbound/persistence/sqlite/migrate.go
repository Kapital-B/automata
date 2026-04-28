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
		if e.Name() == "005_user_category_definitions.sql" {
			if err := migrateUserCategoryDefinitions(db); err != nil {
				return err
			}
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
	return strings.Contains(msg, "duplicate column name: user_id") ||
		strings.Contains(msg, "duplicate column name: chunk_size") ||
		strings.Contains(msg, "duplicate column name: auto_draft_seen_at") ||
		strings.Contains(msg, "duplicate column name: summary_seen_at") ||
		strings.Contains(msg, "duplicate column name: status") ||
		strings.Contains(msg, "duplicate column name: sent_at") ||
		strings.Contains(msg, "duplicate column name: discarded_at") ||
		strings.Contains(msg, "duplicate column name: updated_at")
}

func migrateUserCategoryDefinitions(db *sql.DB) error {
	hasUserID, err := tableHasColumn(db, "category_definitions", "user_id")
	if err != nil {
		return err
	}

	var foreignKeys int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer func() {
		if foreignKeys != 0 {
			_, _ = db.Exec(`PRAGMA foreign_keys=ON`)
		}
	}()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if hasUserID {
		if err := ensureDefaultCategories(tx); err != nil {
			return err
		}
		if err := deleteOrphanedMessageCategories(tx); err != nil {
			return err
		}
		if err := ensureUserCategoryIndexes(tx); err != nil {
			return err
		}
		if _, err := tx.Exec(`DROP TABLE IF EXISTS message_categories_legacy`); err != nil {
			return err
		}
		if _, err := tx.Exec(`DROP TABLE IF EXISTS category_definitions_legacy`); err != nil {
			return err
		}
		return tx.Commit()
	}

	if _, err := tx.Exec(`DROP TABLE IF EXISTS message_categories_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE IF EXISTS category_definitions_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE category_definitions RENAME TO category_definitions_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		CREATE TABLE category_definitions (
			id TEXT PRIMARY KEY NOT NULL,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			slug TEXT NOT NULL,
			display_name TEXT NOT NULL,
			definition TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE (user_id, slug)
		)
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO category_definitions (id, user_id, slug, display_name, definition, sort_order, created_at, updated_at)
		SELECT
			lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
				  substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
			u.id,
			cl.slug,
			cl.display_name,
			'',
			cl.sort_order,
			'1970-01-01T00:00:00Z',
			'1970-01-01T00:00:00Z'
		FROM users u
		CROSS JOIN category_definitions_legacy cl
	`); err != nil {
		return err
	}
	if err := ensureDefaultCategories(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE message_categories RENAME TO message_categories_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		CREATE TABLE message_categories (
			id TEXT PRIMARY KEY NOT NULL,
			message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
			account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			category_id TEXT NOT NULL REFERENCES category_definitions(id) ON DELETE CASCADE,
			source TEXT NOT NULL CHECK (source IN ('llm', 'rule', 'user')),
			confidence REAL,
			run_id TEXT NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE (message_id, source)
		)
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO message_categories (id, message_id, account_id, category_id, source, confidence, run_id, created_at, updated_at)
		SELECT
			mc.id,
			mc.message_id,
			mc.account_id,
			COALESCE(
				(
					SELECT cd.id
					FROM category_definitions cd
					JOIN accounts a ON a.id = mc.account_id
					JOIN category_definitions_legacy cl ON cl.id = mc.category_id
					WHERE cd.user_id = COALESCE(a.user_id, 'a0000001-0000-4000-8000-000000000001')
					  AND cd.slug = cl.slug
					LIMIT 1
				),
				(
					SELECT cd.id
					FROM category_definitions cd
					JOIN accounts a ON a.id = mc.account_id
					WHERE cd.user_id = COALESCE(a.user_id, 'a0000001-0000-4000-8000-000000000001')
					  AND cd.slug = 'other'
					LIMIT 1
				)
			),
			mc.source,
			mc.confidence,
			mc.run_id,
			mc.created_at,
			mc.updated_at
		FROM message_categories_legacy mc
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE message_categories_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE category_definitions_legacy`); err != nil {
		return err
	}
	if err := ensureUserCategoryIndexes(tx); err != nil {
		return err
	}
	if err := deleteOrphanedMessageCategories(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func tableHasColumn(db *sql.DB, tableName string, columnName string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + tableName + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}
	return false, rows.Err()
}

func ensureUserCategoryIndexes(tx *sql.Tx) error {
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_category_definitions_user ON category_definitions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_category_definitions_user_sort ON category_definitions(user_id, sort_order, slug)`,
		`CREATE INDEX IF NOT EXISTS idx_message_categories_account ON message_categories(account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_categories_message ON message_categories(message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_categories_category ON message_categories(category_id)`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func ensureDefaultCategories(tx *sql.Tx) error {
	_, err := tx.Exec(`
		INSERT OR IGNORE INTO category_definitions (id, user_id, slug, display_name, definition, sort_order, created_at, updated_at)
		SELECT
			lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-4' || substr(hex(randomblob(2)), 2) || '-' ||
				  substr('89ab', abs(random()) % 4 + 1, 1) || substr(hex(randomblob(2)), 2) || '-' || hex(randomblob(6))),
			u.id,
			defaults.slug,
			defaults.display_name,
			defaults.definition,
			defaults.sort_order,
			'1970-01-01T00:00:00Z',
			'1970-01-01T00:00:00Z'
		FROM users u
		CROSS JOIN (
			SELECT 'important' AS slug, 'Important' AS display_name, 'Emails requiring timely attention, decisions, or follow-up.' AS definition, 10 AS sort_order
			UNION ALL SELECT 'finance', 'Finance', 'Invoices, receipts, statements, payments, taxes, banking, or accounting.', 20
			UNION ALL SELECT 'personal', 'Personal', 'Personal correspondence, family, friends, appointments, or non-work life admin.', 30
			UNION ALL SELECT 'newsletter', 'Newsletter', 'Subscriptions, digests, marketing updates, announcements, or recurring publications.', 40
			UNION ALL SELECT 'spam', 'Spam', 'Unwanted, suspicious, deceptive, or low-value promotional email.', 50
			UNION ALL SELECT 'other', 'Other', 'Anything that does not clearly fit another category.', 60
		) defaults
	`)
	return err
}

func deleteOrphanedMessageCategories(tx *sql.Tx) error {
	_, err := tx.Exec(`
		DELETE FROM message_categories
		WHERE category_id NOT IN (SELECT id FROM category_definitions)
	`)
	return err
}
