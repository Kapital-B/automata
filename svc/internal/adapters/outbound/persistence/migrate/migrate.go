package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"sort"
	"strings"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/factory"
)

//go:embed common/*.sql postgres/*.sql dsql/*.sql
var migrationsFS embed.FS

var asyncIndexNameRE = regexp.MustCompile(`(?i)CREATE\s+INDEX\s+ASYNC\s+(?:IF\s+NOT\s+EXISTS\s+)?(\S+)`)

type Migration struct {
	Version  string
	Path     string
	Checksum string
	SQL      string
}

func Apply(ctx context.Context, db *sql.DB, engine factory.Engine) error {
	engine, err := factory.ParseEngine(string(engine))
	if err != nil {
		return err
	}
	if engine != factory.EnginePostgres && engine != factory.EngineDSQL {
		return fmt.Errorf("engine %q is not supported by the postgres/dsql migrator", engine)
	}
	slog.InfoContext(ctx, "migrate: starting", "engine", engine)
	if err := ensureHistoryTable(ctx, db); err != nil {
		return err
	}
	migrations, err := List(engine)
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "migrate: listed files", "count", len(migrations))
	applied, skipped := 0, 0
	for _, m := range migrations {
		ok, err := alreadyApplied(ctx, db, m)
		if err != nil {
			return err
		}
		if ok {
			skipped++
			slog.InfoContext(ctx, "migrate: already applied", "path", m.Path)
			continue
		}
		slog.InfoContext(ctx, "migrate: applying", "path", m.Path)
		if err := applyStatements(ctx, db, engine, m); err != nil {
			return fmt.Errorf("%s: %w", m.Path, err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO schema_migrations (version, checksum, applied_at)
			VALUES ($1, $2, NOW())
		`, m.Version, m.Checksum); err != nil {
			return fmt.Errorf("record migration %s: %w", m.Path, err)
		}
		applied++
		slog.InfoContext(ctx, "migrate: recorded", "path", m.Path)
	}
	slog.InfoContext(ctx, "migrate: complete", "applied", applied, "skipped", skipped)
	if engine == factory.EngineDSQL {
		if err := ensureDSQLRuntimeAccess(ctx, db); err != nil {
			return err
		}
	}
	return nil
}

func List(engine factory.Engine) ([]Migration, error) {
	common, err := readDir("common")
	if err != nil {
		return nil, err
	}
	engineMigrations, err := readDir(string(engine))
	if err != nil {
		return nil, err
	}
	out := append(common, engineMigrations...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Version < out[j].Version
	})
	return out, nil
}

func CombinedChecksum(engine factory.Engine) (string, error) {
	migrations, err := List(engine)
	if err != nil {
		return "", err
	}
	sum := sha256.New()
	for _, m := range migrations {
		_, _ = sum.Write([]byte(m.Version))
		_, _ = sum.Write([]byte{'\n'})
		_, _ = sum.Write([]byte(m.Checksum))
		_, _ = sum.Write([]byte{'\n'})
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func ensureHistoryTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func alreadyApplied(ctx context.Context, db *sql.DB, m Migration) (bool, error) {
	var checksum string
	err := db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = $1`, m.Version).Scan(&checksum)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if checksum != m.Checksum {
		return false, fmt.Errorf("checksum mismatch for %s: db=%s file=%s", m.Version, checksum, m.Checksum)
	}
	return true, nil
}

func applyStatements(ctx context.Context, db *sql.DB, engine factory.Engine, m Migration) error {
	// Submit all CREATE INDEX ASYNC jobs first so DSQL can build them concurrently,
	// then wait. Waiting after each submit serialized ~68 index builds and blew the
	// 900s Lambda timeout.
	stmts := splitStatements(m.SQL)
	slog.InfoContext(ctx, "migrate: statements", "path", m.Path, "count", len(stmts))

	type indexJob struct {
		Name string
		ID   string
	}
	var indexJobs []indexJob
	submitted, existed, other := 0, 0, 0

	for i, stmt := range stmts {
		if strings.HasPrefix(strings.ToUpper(stmt), "CREATE INDEX ASYNC") {
			if engine != factory.EngineDSQL {
				return fmt.Errorf("async index found in non-DSQL migration")
			}
			name := asyncIndexName(stmt)
			jobID, err := submitAsyncIndex(ctx, db, stmt)
			if err != nil {
				return err
			}
			if jobID == "" {
				existed++
				slog.InfoContext(ctx, "migrate: index already exists",
					"path", m.Path, "index", name, "step", i+1, "of", len(stmts))
				continue
			}
			submitted++
			indexJobs = append(indexJobs, indexJob{Name: name, ID: jobID})
			slog.InfoContext(ctx, "migrate: index job submitted",
				"path", m.Path, "index", name, "job_id", jobID, "step", i+1, "of", len(stmts))
			continue
		}
		other++
		slog.InfoContext(ctx, "migrate: exec",
			"path", m.Path, "step", i+1, "of", len(stmts), "sql", truncateSQL(stmt, 80))
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	slog.InfoContext(ctx, "migrate: waiting for index jobs",
		"path", m.Path, "jobs", len(indexJobs), "submitted", submitted, "already_existed", existed, "other_stmts", other)
	for i, job := range indexJobs {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("timed out waiting for %d remaining DSQL index job(s): %w", len(indexJobs)-i, err)
		}
		slog.InfoContext(ctx, "migrate: waiting for index job",
			"path", m.Path, "index", job.Name, "job_id", job.ID, "step", i+1, "of", len(indexJobs))
		// wait_for_job is a procedure, not a function — must use CALL.
		if _, err := db.ExecContext(ctx, `CALL sys.wait_for_job($1)`, job.ID); err != nil {
			return fmt.Errorf("wait for DSQL index job %s (%s): %w", job.Name, job.ID, err)
		}
		slog.InfoContext(ctx, "migrate: index job complete",
			"path", m.Path, "index", job.Name, "job_id", job.ID, "step", i+1, "of", len(indexJobs))
	}
	return nil
}

func asyncIndexName(stmt string) string {
	if m := asyncIndexNameRE.FindStringSubmatch(stmt); len(m) == 2 {
		return m[1]
	}
	return "unknown"
}

func truncateSQL(stmt string, max int) string {
	oneLine := strings.Join(strings.Fields(stmt), " ")
	if len(oneLine) <= max {
		return oneLine
	}
	return oneLine[:max] + "…"
}

// submitAsyncIndex runs CREATE INDEX ASYNC and returns the job id.
// An empty job id means the index already existed (IF NOT EXISTS).
func submitAsyncIndex(ctx context.Context, db *sql.DB, stmt string) (string, error) {
	rows, err := db.QueryContext(ctx, stmt)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", err
		}
		return "", nil
	}
	var jobID sql.NullString
	if err := rows.Scan(&jobID); err != nil {
		return "", err
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	id := strings.TrimSpace(jobID.String)
	if !jobID.Valid || id == "" {
		return "", nil
	}
	return id, nil
}

func readDir(dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(migrationsFS, dir)
	if err != nil {
		return nil, err
	}
	out := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		path := dir + "/" + entry.Name()
		body, err := migrationsFS.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(body)
		out = append(out, Migration{
			Version:  path,
			Path:     path,
			Checksum: hex.EncodeToString(sum[:]),
			SQL:      string(body),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Version < out[j].Version
	})
	return out, nil
}

func splitStatements(sqlText string) []string {
	var (
		out      []string
		buf      strings.Builder
		inSingle bool
	)
	runes := []rune(sqlText)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\'' {
			buf.WriteRune(r)
			if inSingle {
				// SQL escaped quote: ''
				if i+1 < len(runes) && runes[i+1] == '\'' {
					buf.WriteRune('\'')
					i++
					continue
				}
				inSingle = false
			} else {
				inSingle = true
			}
			continue
		}
		if r == ';' && !inSingle {
			stmt := strings.TrimSpace(buf.String())
			if stmt != "" {
				out = append(out, stmt)
			}
			buf.Reset()
			continue
		}
		buf.WriteRune(r)
	}
	if stmt := strings.TrimSpace(buf.String()); stmt != "" {
		out = append(out, stmt)
	}
	return out
}
