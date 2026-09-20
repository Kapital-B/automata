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

func dsqlStatements(t *testing.T) map[string][]string {
	t.Helper()
	migrations, err := List(factory.EngineDSQL)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, m := range migrations {
		out[m.Path] = splitStatements(m.SQL)
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
		for _, raw := range statements {
			stmt := stripSQLComments(raw)
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
		for _, raw := range statements {
			stmt := stripSQLComments(raw)
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
