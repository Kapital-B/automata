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

// TestSplitStatementsApostropheInComment is the root cause of the DSQL deploy
// failures on PR #9.
//
// The splitter tracked string literals but not comments, so the apostrophe in
// "DSQL's" opened a literal that never closed. Every subsequent `;` was then
// treated as text and the whole file collapsed into one statement, which the
// driver rejected as multiple commands in a prepared statement.
func TestSplitStatementsApostropheInComment(t *testing.T) {
	sqlText := "-- DSQL's grammar has no ASC/DESC on index keys.\n" +
		"CREATE INDEX ASYNC a ON t(x);\n" +
		"CREATE INDEX ASYNC b ON t(y);\n"
	stmts := splitStatements(sqlText)
	if len(stmts) != 2 {
		t.Fatalf("got %d statements, want 2: %#v", len(stmts), stmts)
	}
	for _, stmt := range stmts {
		if strings.Count(strings.ToUpper(stmt), "CREATE INDEX") != 1 {
			t.Errorf("statement carries more than one command: %q", stmt)
		}
	}
}

func TestSplitStatementsCommentHandling(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"apostrophe in line comment", "-- it's fine\nSELECT 1;\nSELECT 2;", 2},
		{"semicolon in line comment", "-- a; b; c\nSELECT 1;\nSELECT 2;", 2},
		{"block comment", "/* it's a; note */\nSELECT 1;\nSELECT 2;", 2},
		{"apostrophe in block comment", "/* DSQL's note */ SELECT 1; SELECT 2;", 2},
		// Real string literals must still suppress splitting.
		{"semicolon in string", "INSERT INTO t VALUES ('a;b');\nSELECT 1;", 2},
		{"escaped quote in string", "INSERT INTO t VALUES ('it''s');\nSELECT 1;", 2},
		{"comment marker inside string", "INSERT INTO t VALUES ('-- not a comment; really');\nSELECT 1;", 2},
		{"quoted identifier with semicolon", `CREATE TABLE "odd;name" (a INT);` + "\nSELECT 1;", 2},
		{"comment only", "-- nothing to run\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := splitStatements(tc.in); len(got) != tc.want {
				t.Errorf("split into %d, want %d: %#v", len(got), tc.want, got)
			}
		})
	}
}
