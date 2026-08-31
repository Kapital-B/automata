package migrate

import (
	"os"
	"strings"
	"testing"
)

func TestSplitStatementsHandlesEmptyStringDefaults(t *testing.T) {
	b, err := os.ReadFile("common/001_baseline.sql")
	if err != nil {
		t.Fatal(err)
	}
	stmts := splitStatements(string(b))
	if len(stmts) < 40 {
		t.Fatalf("expected baseline to split into many statements, got %d", len(stmts))
	}
	creates := 0
	for i, stmt := range stmts {
		upper := strings.ToUpper(stmt)
		n := strings.Count(upper, "CREATE TABLE")
		if n > 1 {
			t.Fatalf("statement %d contains %d CREATE TABLE clauses", i, n)
		}
		if n == 1 {
			creates++
		}
	}
	if creates < 40 {
		t.Fatalf("expected many CREATE TABLE statements, got %d from %d total", creates, len(stmts))
	}
}

func TestSplitStatementsEmptyStringLiteral(t *testing.T) {
	sqlText := "CREATE TABLE t (label TEXT NOT NULL DEFAULT '');\nINSERT INTO t (label) VALUES ('');"
	stmts := splitStatements(sqlText)
	if len(stmts) != 2 {
		t.Fatalf("got %d statements: %#v", len(stmts), stmts)
	}
}
