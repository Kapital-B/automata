package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
)

var (
	dsqlIdentRE  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	iamRoleARNRE = regexp.MustCompile(`^arn:aws:iam::\d{12}:role/[\w+=,.@/-]+$`)
)

const (
	defaultDSQLRuntimeRole = "automata_runtime"
	defaultDSQLSchema      = "automata"
	schemaMigrationsTable  = "schema_migrations"
)

// ensureDSQLRuntimeAccess creates the least-privilege runtime DB role, moves
// application tables into a user schema (DSQL rejects grants on public), grants
// privileges, and maps Lambda IAM role ARNs via AWS IAM GRANT.
func ensureDSQLRuntimeAccess(ctx context.Context, db *sql.DB) error {
	role, err := dsqlIdentFromEnv("DSQL_RUNTIME_DATABASE_ROLE", defaultDSQLRuntimeRole)
	if err != nil {
		return err
	}
	schema, err := dsqlIdentFromEnv("DSQL_SCHEMA", defaultDSQLSchema)
	if err != nil {
		return err
	}

	arns, err := parseRuntimeIAMRoleARNs(os.Getenv("DSQL_RUNTIME_IAM_ROLE_ARNS"))
	if err != nil {
		return err
	}
	if len(arns) == 0 {
		return fmt.Errorf("DSQL_RUNTIME_IAM_ROLE_ARNS is required for dsql migrations")
	}

	slog.InfoContext(ctx, "migrate: ensuring dsql runtime access",
		"role", role, "schema", schema, "iam_arns", len(arns))

	if err := ensureDSQLRole(ctx, db, role); err != nil {
		return err
	}
	if err := ensureDSQLAppSchema(ctx, db, schema); err != nil {
		return err
	}
	if err := movePublicTablesToSchema(ctx, db, schema); err != nil {
		return err
	}
	if err := grantDSQLRuntimePrivileges(ctx, db, schema, role); err != nil {
		return err
	}
	if err := ensureDSQLIAMGrants(ctx, db, role, arns); err != nil {
		return err
	}

	slog.InfoContext(ctx, "migrate: dsql runtime access ready", "role", role, "schema", schema)
	return nil
}

func ensureDSQLRole(ctx context.Context, db *sql.DB, role string) error {
	var exists bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, role,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check dsql runtime role: %w", err)
	}
	if exists {
		return nil
	}
	slog.InfoContext(ctx, "migrate: creating dsql runtime role", "role", role)
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE ROLE %s WITH LOGIN`, role)); err != nil {
		return fmt.Errorf("create dsql runtime role %s: %w", role, err)
	}
	return nil
}

func ensureDSQLAppSchema(ctx context.Context, db *sql.DB, schema string) error {
	var exists bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`, schema,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check dsql schema: %w", err)
	}
	if exists {
		return nil
	}
	slog.InfoContext(ctx, "migrate: creating dsql schema", "schema", schema)
	// DSQL allows one DDL statement per transaction; Exec auto-commits.
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA %s`, schema)); err != nil {
		return fmt.Errorf("create dsql schema %s: %w", schema, err)
	}
	return nil
}

// movePublicTablesToSchema relocates application tables out of public.
// DSQL treats public as a system schema: custom roles cannot GRANT USAGE on it.
// schema_migrations stays in public so the migrator bookkeeping path is stable.
func movePublicTablesToSchema(ctx context.Context, db *sql.DB, schema string) error {
	tables, err := listSchemaUserTables(ctx, db, "public")
	if err != nil {
		return err
	}
	for _, table := range tables {
		if table == schemaMigrationsTable {
			continue
		}
		stmt := fmt.Sprintf(
			`ALTER TABLE public.%s SET SCHEMA %s`,
			quotePGIdent(table), schema,
		)
		slog.InfoContext(ctx, "migrate: moving table to dsql app schema",
			"table", table, "schema", schema)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("move public.%s to %s: %w", table, schema, err)
		}
	}
	return nil
}

func grantDSQLRuntimePrivileges(ctx context.Context, db *sql.DB, schema, role string) error {
	usage := fmt.Sprintf(`GRANT USAGE ON SCHEMA %s TO %s`, schema, role)
	slog.InfoContext(ctx, "migrate: dsql grant", "sql", usage)
	if _, err := db.ExecContext(ctx, usage); err != nil {
		return fmt.Errorf("grant schema usage on %s: %w", schema, err)
	}

	tables, err := listSchemaUserTables(ctx, db, schema)
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "migrate: granting dsql table privileges",
		"role", role, "schema", schema, "tables", len(tables))
	for _, table := range tables {
		stmt := fmt.Sprintf(
			`GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE %s.%s TO %s`,
			schema, quotePGIdent(table), role,
		)
		slog.InfoContext(ctx, "migrate: dsql grant", "schema", schema, "table", table)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("grant on %s.%s: %w", schema, table, err)
		}
	}
	return nil
}

func ensureDSQLIAMGrants(ctx context.Context, db *sql.DB, role string, arns []string) error {
	mapped := map[string]struct{}{}
	rows, err := db.QueryContext(ctx,
		`SELECT arn FROM sys.iam_pg_role_mappings WHERE pg_role_name = $1`, role,
	)
	if err != nil {
		return fmt.Errorf("list dsql iam mappings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var arn string
		if err := rows.Scan(&arn); err != nil {
			return fmt.Errorf("scan dsql iam mapping: %w", err)
		}
		mapped[strings.TrimSpace(arn)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list dsql iam mappings: %w", err)
	}

	for _, arn := range arns {
		if _, ok := mapped[arn]; ok {
			slog.InfoContext(ctx, "migrate: dsql iam grant already present", "role", role, "arn", arn)
			continue
		}
		slog.InfoContext(ctx, "migrate: applying dsql iam grant", "role", role, "arn", arn)
		stmt := fmt.Sprintf(`AWS IAM GRANT %s TO '%s'`, role, arn)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("AWS IAM GRANT %s TO %s: %w", role, arn, err)
		}
	}
	return nil
}

func listSchemaUserTables(ctx context.Context, db *sql.DB, schema string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname
		FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1
		  AND c.relkind = 'r'
		ORDER BY c.relname
	`, schema)
	if err != nil {
		return nil, fmt.Errorf("list tables in schema %s: %w", schema, err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table in schema %s: %w", schema, err)
		}
		if !dsqlIdentRE.MatchString(name) {
			return nil, fmt.Errorf("unexpected table name %q in schema %s", name, schema)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tables in schema %s: %w", schema, err)
	}
	return tables, nil
}

func dsqlIdentFromEnv(key, fallback string) (string, error) {
	name := strings.TrimSpace(os.Getenv(key))
	if name == "" {
		name = fallback
	}
	if !dsqlIdentRE.MatchString(name) {
		return "", fmt.Errorf("invalid %s %q", key, name)
	}
	return name, nil
}

func quotePGIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func parseRuntimeIAMRoleARNs(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		arn := strings.TrimSpace(part)
		if arn == "" {
			continue
		}
		if !iamRoleARNRE.MatchString(arn) {
			return nil, fmt.Errorf("invalid IAM role ARN in DSQL_RUNTIME_IAM_ROLE_ARNS: %q", arn)
		}
		if _, ok := seen[arn]; ok {
			continue
		}
		seen[arn] = struct{}{}
		out = append(out, arn)
	}
	return out, nil
}
