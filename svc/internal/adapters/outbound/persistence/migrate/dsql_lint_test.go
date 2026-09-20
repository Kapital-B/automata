package migrate

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/factory"
)

// CI has no Aurora DSQL cluster, so DSQL grammar violations are only found by
// a deploy failing. These lints encode the parts of the DSQL dialect we have
// actually been bitten by, so they fail in CI instead.
//
// Reference: https://docs.aws.amazon.com/aurora-dsql/latest/userguide/create-index-syntax-support.html
//
//	CREATE [ UNIQUE ] INDEX ASYNC [ [ IF NOT EXISTS ] name ] ON table_name
//	    ( { column_name | ( expression ) } [ NULLS { FIRST | LAST } ] [, ...] )
//	    [ INCLUDE ( column_name [, ...] ) ] [ NULLS [ NOT ] DISTINCT ] [ WHERE predicate ]
var (
	createIndexRE = regexp.MustCompile(`(?is)^\s*CREATE\s+(UNIQUE\s+)?INDEX\b`)
	sortOrderRE   = regexp.MustCompile(`(?i)(,|\(|\s)\s*[\w".]+\s+(ASC|DESC)\s*(,|\))`)
)

// dsqlStatements returns the DSQL migration set as the migrator itself
// classifies it, so the lints assert on what will actually be executed.
func dsqlStatements(t *testing.T) map[string][]classifiedStatement {
	t.Helper()
	migrations, err := List(factory.EngineDSQL)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]classifiedStatement{}
	for _, m := range migrations {
		out[m.Path] = classifyStatements(m.SQL)
	}
	if len(out) == 0 {
		t.Fatal("no DSQL migrations listed")
	}
	return out
}

// stripSQLComments removes -- line comments so commented-out examples and
// explanatory notes do not trip the lints.
func stripSQLComments(stmt string) string {
	lines := strings.Split(stmt, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// TestDSQLIndexesHaveNoSortOrder guards the failure that reached dev:
// "specifying sort order not supported for index keys (SQLSTATE 0A000)".
// Postgres accepts DESC on index keys; DSQL has no ASC/DESC in its grammar.
func TestDSQLIndexesHaveNoSortOrder(t *testing.T) {
	for path, statements := range dsqlStatements(t) {
		for _, cs := range statements {
			stmt := stripSQLComments(cs.SQL)
			if !createIndexRE.MatchString(stmt) {
				continue
			}
			keys := indexKeyList(stmt)
			if keys == "" {
				continue
			}
			if m := sortOrderRE.FindStringSubmatch(keys); m != nil {
				t.Errorf("%s: DSQL rejects %s on index keys; drop it (an ascending index still serves ORDER BY ... DESC)\n  %s",
					path, strings.ToUpper(m[2]), strings.Join(strings.Fields(stmt), " "))
			}
		}
	}
}

// TestDSQLIndexesAreAsync enforces the other half of the grammar: DSQL index
// creation is always asynchronous and the ASYNC keyword is mandatory.
func TestDSQLIndexesAreAsync(t *testing.T) {
	for path, statements := range dsqlStatements(t) {
		for _, cs := range statements {
			stmt := stripSQLComments(cs.SQL)
			if !createIndexRE.MatchString(stmt) {
				continue
			}
			if !regexp.MustCompile(`(?is)\bINDEX\s+ASYNC\b`).MatchString(stmt) {
				t.Errorf("%s: DSQL requires CREATE INDEX ASYNC\n  %s",
					path, strings.Join(strings.Fields(stmt), " "))
			}
		}
	}
}

// TestPostgresIndexesAreNotAsync is the mirror: ASYNC is DSQL-only syntax and
// vanilla Postgres rejects it.
func TestPostgresIndexesAreNotAsync(t *testing.T) {
	migrations, err := List(factory.EnginePostgres)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migrations {
		for _, raw := range splitStatements(m.SQL) {
			stmt := stripSQLComments(raw)
			if regexp.MustCompile(`(?is)\bINDEX\s+ASYNC\b`).MatchString(stmt) {
				t.Errorf("%s: CREATE INDEX ASYNC is DSQL-only\n  %s",
					m.Path, strings.Join(strings.Fields(stmt), " "))
			}
		}
	}
}

// indexKeyList returns the parenthesised key list of a CREATE INDEX statement,
// i.e. the group following the table name, excluding INCLUDE/WHERE.
func indexKeyList(stmt string) string {
	on := regexp.MustCompile(`(?is)\bON\s+[\w".]+\s*\(`).FindStringIndex(stmt)
	if on == nil {
		return ""
	}
	depth := 0
	start := on[1] - 1
	for i := start; i < len(stmt); i++ {
		switch stmt[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return stmt[start : i+1]
			}
		}
	}
	return ""
}

// TestDSQLAsyncIndexesAreClassifiedAsync is the guard for the second failure
// that reached dev.
//
// CREATE INDEX ASYNC returns a job id, so the migrator must send it down the
// query path. Classification keys off the start of the statement, and a
// migration opening with a `--` comment header used to fall through to the
// generic exec path, which DSQL rejects with "multiple ddl statements not
// supported in a transaction". Documenting a migration must not change how it
// is executed.
//
// This asserts on classifyStatements, which is what applyStatements iterates.
func TestDSQLAsyncIndexesAreClassifiedAsync(t *testing.T) {
	for path, statements := range dsqlStatements(t) {
		for _, cs := range statements {
			if !createIndexRE.MatchString(stripSQLComments(cs.SQL)) {
				continue
			}
			if !cs.AsyncIndex {
				t.Errorf("%s: index statement not classified async; it would be exec'd instead of queried\n  %s",
					path, strings.Join(strings.Fields(cs.SQL), " "))
			}
		}
	}
}

// TestClassifyStatementsIgnoresCommentHeaders pins the classification itself
// against the exact shape that broke: a documented migration.
func TestClassifyStatementsIgnoresCommentHeaders(t *testing.T) {
	got := classifyStatements(`
-- Some explanation of why this index exists.
-- Spanning several lines.
CREATE INDEX ASYNC IF NOT EXISTS idx_a ON t(a);
CREATE INDEX ASYNC IF NOT EXISTS idx_b ON t(b);
-- a trailing note with no statement
`)
	if len(got) != 2 {
		t.Fatalf("classified %d statements, want 2 (comment-only chunks dropped)", len(got))
	}
	for _, cs := range got {
		if !cs.AsyncIndex {
			t.Errorf("statement not classified async: %q", cs.SQL)
		}
		if strings.HasPrefix(cs.SQL, "--") {
			t.Errorf("comment header leaked into the executed SQL: %q", cs.SQL)
		}
	}
}

func TestStatementBodyStripsLeadingComments(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "CREATE INDEX ASYNC x ON t(a)", "CREATE INDEX ASYNC x ON t(a)"},
		{"comment header", "-- why\n-- more\nCREATE INDEX ASYNC x ON t(a)", "CREATE INDEX ASYNC x ON t(a)"},
		{"blank lines", "\n\n  \nCREATE INDEX ASYNC x ON t(a)", "CREATE INDEX ASYNC x ON t(a)"},
		{"comment only", "-- just a note\n", ""},
		{"empty", "", ""},
		// A trailing comment belongs to the statement and must survive.
		{"trailing comment kept", "-- lead\nCREATE INDEX ASYNC x ON t(a) -- tail", "CREATE INDEX ASYNC x ON t(a) -- tail"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statementBody(tc.in); got != tc.want {
				t.Errorf("statementBody(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
