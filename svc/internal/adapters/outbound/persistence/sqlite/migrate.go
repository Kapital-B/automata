package sqlite

import (
	"database/sql"
	"embed"
	"strings"

	"github.com/google/uuid"
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
		if e.Name() == "015_organisations_contacts.sql" {
			if err := migrateOrganisationsContacts(db); err != nil {
				return err
			}
			continue
		}
		if e.Name() == "016_projects_assignments.sql" {
			if err := migrateProjectsAssignments(db); err != nil {
				return err
			}
			continue
		}
		if e.Name() == "017_manual_items.sql" {
			if err := migrateManualItems(db); err != nil {
				return err
			}
			continue
		}
		if e.Name() == "018_issues.sql" {
			if err := migrateIssues(db); err != nil {
				return err
			}
			continue
		}
		if e.Name() == "019_facts.sql" {
			if err := migrateFacts(db); err != nil {
				return err
			}
			continue
		}
		if e.Name() == "020_interpretations.sql" {
			if err := migrateInterpretations(db); err != nil {
				return err
			}
			continue
		}
		if e.Name() == "021_contradictions.sql" {
			if err := migrateContradictions(db); err != nil {
				return err
			}
			continue
		}
		if e.Name() == "022_decisions.sql" {
			if err := migrateDecisions(db); err != nil {
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
		strings.Contains(msg, "duplicate column name: forward_seen_at") ||
		strings.Contains(msg, "duplicate column name: status") ||
		strings.Contains(msg, "duplicate column name: sent_at") ||
		strings.Contains(msg, "duplicate column name: discarded_at") ||
		strings.Contains(msg, "duplicate column name: updated_at") ||
		strings.Contains(msg, "duplicate column name: home_organisation_id") ||
		strings.Contains(msg, "duplicate column name: to_json") ||
		strings.Contains(msg, "duplicate column name: cc_json")
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

func migrateOrganisationsContacts(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS organisations (
			id TEXT PRIMARY KEY NOT NULL,
			name TEXT NOT NULL DEFAULT 'Personal',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS organisation_members (
			id TEXT PRIMARY KEY NOT NULL,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			org_role TEXT NOT NULL CHECK (org_role IN ('owner', 'member')),
			created_at TEXT NOT NULL,
			UNIQUE (organisation_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_organisation_members_user ON organisation_members(user_id)`,
		`CREATE TABLE IF NOT EXISTS contacts (
			id TEXT PRIMARY KEY NOT NULL,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			display_name TEXT NOT NULL DEFAULT '',
			company TEXT,
			merged_into_contact_id TEXT REFERENCES contacts(id) ON DELETE SET NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_org ON contacts(organisation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_org_name ON contacts(organisation_id, display_name)`,
		`CREATE TABLE IF NOT EXISTS contact_identities (
			id TEXT PRIMARY KEY NOT NULL,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			contact_id TEXT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
			kind TEXT NOT NULL CHECK (kind IN ('email', 'phone', 'display_name_hint')),
			value_normalized TEXT NOT NULL,
			value_raw TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE (organisation_id, kind, value_normalized)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_identities_contact ON contact_identities(contact_id)`,
		`CREATE TABLE IF NOT EXISTS contact_profile_links (
			id TEXT PRIMARY KEY NOT NULL,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			contact_id TEXT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_by_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TEXT NOT NULL,
			UNIQUE (contact_id),
			UNIQUE (organisation_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS correspondence_participants (
			id TEXT PRIMARY KEY NOT NULL,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			contact_id TEXT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
			role TEXT NOT NULL CHECK (role IN ('from', 'to', 'cc', 'participant')),
			message_id TEXT REFERENCES messages(id) ON DELETE CASCADE,
			manual_item_id TEXT,
			CHECK (
				(message_id IS NOT NULL AND manual_item_id IS NULL) OR
				(message_id IS NULL AND manual_item_id IS NOT NULL)
			)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_corr_part_msg
			ON correspondence_participants(contact_id, role, message_id)
			WHERE message_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_corr_part_message ON correspondence_participants(message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_corr_part_contact ON correspondence_participants(contact_id)`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	hasHomeOrg, err := tableHasColumn(db, "users", "home_organisation_id")
	if err != nil {
		return err
	}
	if !hasHomeOrg {
		if _, err := db.Exec(`ALTER TABLE users ADD COLUMN home_organisation_id TEXT REFERENCES organisations(id)`); err != nil {
			return err
		}
	}

	hasToJSON, err := tableHasColumn(db, "messages", "to_json")
	if err != nil {
		return err
	}
	if !hasToJSON {
		if _, err := db.Exec(`ALTER TABLE messages ADD COLUMN to_json TEXT NOT NULL DEFAULT '[]'`); err != nil {
			return err
		}
	}
	hasCcJSON, err := tableHasColumn(db, "messages", "cc_json")
	if err != nil {
		return err
	}
	if !hasCcJSON {
		if _, err := db.Exec(`ALTER TABLE messages ADD COLUMN cc_json TEXT NOT NULL DEFAULT '[]'`); err != nil {
			return err
		}
	}

	return backfillHomeOrganisations(db)
}

func backfillHomeOrganisations(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, created_at FROM users WHERE home_organisation_id IS NULL OR home_organisation_id = ''`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type userRow struct {
		id        string
		createdAt string
	}
	var users []userRow
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.id, &u.createdAt); err != nil {
			return err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, u := range users {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		orgID := newMigrationUUID()
		now := u.createdAt
		if now == "" {
			now = "1970-01-01T00:00:00Z"
		}
		if _, err := tx.Exec(`
			INSERT INTO organisations (id, name, created_at, updated_at) VALUES (?, 'Personal', ?, ?)
		`, orgID, now, now); err != nil {
			_ = tx.Rollback()
			return err
		}
		memberID := newMigrationUUID()
		if _, err := tx.Exec(`
			INSERT INTO organisation_members (id, organisation_id, user_id, org_role, created_at)
			VALUES (?, ?, ?, 'owner', ?)
		`, memberID, orgID, u.id, now); err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := tx.Exec(`UPDATE users SET home_organisation_id = ? WHERE id = ?`, orgID, u.id); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func newMigrationUUID() string {
	return uuid.New().String()
}

func migrateProjectsAssignments(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY NOT NULL,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			code TEXT NOT NULL,
			description TEXT,
			client TEXT,
			keywords_json TEXT NOT NULL DEFAULT '[]',
			archived_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE (organisation_id, code)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_projects_org ON projects(organisation_id)`,
		`CREATE TABLE IF NOT EXISTS project_members (
			id TEXT PRIMARY KEY NOT NULL,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			role TEXT NOT NULL DEFAULT '',
			discipline TEXT,
			responsibilities TEXT,
			current_scope TEXT,
			approval_authority TEXT,
			out_of_scope TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE (project_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_project_members_user ON project_members(user_id)`,
		`CREATE TABLE IF NOT EXISTS project_participants (
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			contact_id TEXT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
			first_seen_at TEXT NOT NULL,
			PRIMARY KEY (project_id, contact_id)
		)`,
		`CREATE TABLE IF NOT EXISTS thread_assignments (
			id TEXT PRIMARY KEY NOT NULL,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			conversation_id TEXT NOT NULL,
			project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
			status TEXT NOT NULL CHECK (status IN ('committed', 'provisional')),
			confidence REAL,
			reason TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL CHECK (source IN ('user', 'rule', 'llm')),
			run_id TEXT REFERENCES job_runs(id) ON DELETE SET NULL,
			assigned_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE (account_id, conversation_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_thread_assignments_org ON thread_assignments(organisation_id)`,
		`CREATE TABLE IF NOT EXISTS message_assignment_overrides (
			message_id TEXT PRIMARY KEY NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
			status TEXT NOT NULL CHECK (status IN ('committed', 'provisional')),
			confidence REAL,
			reason TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL CHECK (source IN ('user', 'rule', 'llm')),
			run_id TEXT REFERENCES job_runs(id) ON DELETE SET NULL,
			assigned_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_msg_overrides_org ON message_assignment_overrides(organisation_id)`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return extendJobRunTypes(db)
}

func extendJobRunTypes(db *sql.DB) error {
	var sqlText string
	err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='job_runs'`).Scan(&sqlText)
	if err != nil {
		return err
	}
	if strings.Contains(sqlText, "reconcile_project") {
		return nil
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
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
		CREATE TABLE job_runs_new (
			id TEXT PRIMARY KEY NOT NULL,
			account_id TEXT REFERENCES accounts(id) ON DELETE SET NULL,
			job_type TEXT NOT NULL CHECK (job_type IN (
				'sync', 'summarize', 'categorize', 'forward_rules', 'draft_suggest',
				'resolve_contacts', 'assign_projects', 'interpret_project', 'reconcile_project'
			)),
			trigger_kind TEXT NOT NULL CHECK (trigger_kind IN ('schedule', 'api')),
			status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'success', 'failed', 'cancelled')),
			time_window_start TEXT,
			time_window_end TEXT,
			started_at TEXT NOT NULL,
			finished_at TEXT,
			error_message TEXT,
			meta_json TEXT NOT NULL DEFAULT '{}'
		)
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO job_runs_new (
			id, account_id, job_type, trigger_kind, status,
			time_window_start, time_window_end, started_at, finished_at, error_message, meta_json
		)
		SELECT
			id, account_id, job_type, trigger_kind, status,
			time_window_start, time_window_end, started_at, finished_at, error_message, meta_json
		FROM job_runs
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE job_runs`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE job_runs_new RENAME TO job_runs`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateManualItems(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS manual_items (
			id TEXT PRIMARY KEY NOT NULL,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			channel TEXT NOT NULL CHECK (channel IN ('whatsapp', 'teams', 'sms', 'call', 'meeting', 'note')),
			occurred_at TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			body_text TEXT NOT NULL,
			project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
			assignment_status TEXT NOT NULL CHECK (assignment_status IN ('committed', 'provisional', 'unassigned')),
			assignment_reason TEXT,
			assignment_source TEXT CHECK (assignment_source IS NULL OR assignment_source IN ('user', 'rule', 'llm')),
			created_by_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_manual_items_org ON manual_items(organisation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_manual_items_project ON manual_items(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_manual_items_occurred ON manual_items(occurred_at)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_corr_part_manual
			ON correspondence_participants(contact_id, role, manual_item_id)
			WHERE manual_item_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_corr_part_manual_item ON correspondence_participants(manual_item_id)`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func migrateIssues(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS issues (
			id TEXT PRIMARY KEY NOT NULL,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			current_position_note TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL CHECK (status IN ('open', 'awaiting_input', 'resolved')),
			assignee_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
			assignee_contact_id TEXT REFERENCES contacts(id) ON DELETE SET NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			CHECK (
				assignee_user_id IS NULL OR assignee_contact_id IS NULL
			)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_issues_project ON issues(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_issues_org ON issues(organisation_id)`,
		`CREATE TABLE IF NOT EXISTS issue_items (
			id TEXT PRIMARY KEY NOT NULL,
			issue_id TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
			message_id TEXT REFERENCES messages(id) ON DELETE CASCADE,
			manual_item_id TEXT REFERENCES manual_items(id) ON DELETE CASCADE,
			added_at TEXT NOT NULL,
			CHECK (
				(message_id IS NOT NULL AND manual_item_id IS NULL) OR
				(message_id IS NULL AND manual_item_id IS NOT NULL)
			)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_issue_items_message
			ON issue_items(message_id) WHERE message_id IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_issue_items_manual
			ON issue_items(manual_item_id) WHERE manual_item_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_issue_items_issue ON issue_items(issue_id)`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func migrateFacts(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS facts (
			id TEXT PRIMARY KEY NOT NULL,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			issue_id TEXT REFERENCES issues(id) ON DELETE SET NULL,
			subject_key TEXT NOT NULL,
			label TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE (project_id, subject_key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_facts_project ON facts(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_facts_org ON facts(organisation_id)`,
		`CREATE TABLE IF NOT EXISTS fact_versions (
			id TEXT PRIMARY KEY NOT NULL,
			fact_id TEXT NOT NULL REFERENCES facts(id) ON DELETE CASCADE,
			status TEXT NOT NULL CHECK (status IN ('proposed', 'active', 'superseded', 'rejected')),
			value_json TEXT NOT NULL,
			value_text TEXT NOT NULL,
			unit TEXT,
			source TEXT NOT NULL CHECK (source IN ('user', 'rule', 'llm')),
			confidence REAL,
			interpretation_id TEXT,
			supersedes_version_id TEXT REFERENCES fact_versions(id) ON DELETE SET NULL,
			superseded_by_version_id TEXT REFERENCES fact_versions(id) ON DELETE SET NULL,
			superseded_at TEXT,
			created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fact_versions_fact ON fact_versions(fact_id, status)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_fact_versions_one_active
			ON fact_versions(fact_id) WHERE status = 'active'`,
		`CREATE TABLE IF NOT EXISTS fact_evidence (
			id TEXT PRIMARY KEY NOT NULL,
			fact_version_id TEXT NOT NULL REFERENCES fact_versions(id) ON DELETE CASCADE,
			message_id TEXT REFERENCES messages(id) ON DELETE CASCADE,
			manual_item_id TEXT REFERENCES manual_items(id) ON DELETE CASCADE,
			added_at TEXT NOT NULL,
			CHECK (
				(message_id IS NOT NULL AND manual_item_id IS NULL) OR
				(message_id IS NULL AND manual_item_id IS NOT NULL)
			)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fact_evidence_version ON fact_evidence(fact_version_id)`,
		`CREATE INDEX IF NOT EXISTS idx_fact_evidence_message ON fact_evidence(message_id)
			WHERE message_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_fact_evidence_manual ON fact_evidence(manual_item_id)
			WHERE manual_item_id IS NOT NULL`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func migrateInterpretations(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS interpretations (
			id TEXT PRIMARY KEY NOT NULL,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			account_id TEXT REFERENCES accounts(id) ON DELETE SET NULL,
			run_id TEXT REFERENCES job_runs(id) ON DELETE SET NULL,
			status TEXT NOT NULL CHECK (status IN ('pending', 'accepted', 'dismissed', 'expired')),
			payload_json TEXT NOT NULL,
			confidence REAL,
			reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_interpretations_project_status ON interpretations(project_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_interpretations_org ON interpretations(organisation_id)`,
		`CREATE TABLE IF NOT EXISTS interpretation_sources (
			id TEXT PRIMARY KEY NOT NULL,
			interpretation_id TEXT NOT NULL REFERENCES interpretations(id) ON DELETE CASCADE,
			message_id TEXT REFERENCES messages(id) ON DELETE CASCADE,
			manual_item_id TEXT REFERENCES manual_items(id) ON DELETE CASCADE,
			CHECK (
				(message_id IS NOT NULL AND manual_item_id IS NULL) OR
				(message_id IS NULL AND manual_item_id IS NOT NULL)
			)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_interpretation_sources_interp ON interpretation_sources(interpretation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_interpretation_sources_message ON interpretation_sources(message_id)
			WHERE message_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_interpretation_sources_manual ON interpretation_sources(manual_item_id)
			WHERE manual_item_id IS NOT NULL`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return extendJobRunTypes(db)
}

func migrateContradictions(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS contradictions (
			id TEXT PRIMARY KEY NOT NULL,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			status TEXT NOT NULL CHECK (status IN ('open', 'resolved')),
			summary TEXT NOT NULL,
			resolution_note TEXT,
			resolved_at TEXT,
			resolved_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contradictions_project_status ON contradictions(project_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_contradictions_org ON contradictions(organisation_id)`,
		`CREATE TABLE IF NOT EXISTS contradiction_sides (
			id TEXT PRIMARY KEY NOT NULL,
			contradiction_id TEXT NOT NULL REFERENCES contradictions(id) ON DELETE CASCADE,
			fact_version_id TEXT REFERENCES fact_versions(id) ON DELETE SET NULL,
			decision_id TEXT,
			CHECK (
				fact_version_id IS NOT NULL OR decision_id IS NOT NULL
			)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contradiction_sides_contradiction ON contradiction_sides(contradiction_id)`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return extendJobRunTypes(db)
}

func migrateDecisions(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS decisions (
			id TEXT PRIMARY KEY NOT NULL,
			organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			issue_id TEXT REFERENCES issues(id) ON DELETE SET NULL,
			statement TEXT NOT NULL,
			status TEXT NOT NULL CHECK (status IN ('proposed', 'accepted', 'superseded', 'withdrawn')),
			decided_at TEXT,
			assignee_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
			assignee_contact_id TEXT REFERENCES contacts(id) ON DELETE SET NULL,
			source TEXT NOT NULL CHECK (source IN ('user', 'rule', 'llm')),
			confidence REAL,
			supersedes_decision_id TEXT REFERENCES decisions(id) ON DELETE SET NULL,
			created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			CHECK (
				(assignee_user_id IS NULL AND assignee_contact_id IS NULL) OR
				(assignee_user_id IS NOT NULL AND assignee_contact_id IS NULL) OR
				(assignee_user_id IS NULL AND assignee_contact_id IS NOT NULL)
			)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_decisions_project_status ON decisions(project_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_decisions_org ON decisions(organisation_id)`,
		`CREATE TABLE IF NOT EXISTS decision_evidence (
			id TEXT PRIMARY KEY NOT NULL,
			decision_id TEXT NOT NULL REFERENCES decisions(id) ON DELETE CASCADE,
			message_id TEXT REFERENCES messages(id) ON DELETE CASCADE,
			manual_item_id TEXT REFERENCES manual_items(id) ON DELETE CASCADE,
			added_at TEXT NOT NULL,
			CHECK (
				(message_id IS NOT NULL AND manual_item_id IS NULL) OR
				(message_id IS NULL AND manual_item_id IS NOT NULL)
			)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_decision_evidence_decision ON decision_evidence(decision_id)`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
