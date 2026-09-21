package factory

import (
	"net/url"
	"strings"
	"testing"
)

func TestDSQLDSNDropsAnyPassword(t *testing.T) {
	// The regression that took the scheduler down: a token frozen into the DSN
	// authenticates the first connections and is refused by every connection
	// opened after it expires (~15 minutes), which on a warm container is all
	// of them. The DSN must carry no credential at all.
	t.Setenv("DSQL_SCHEMA", "automata")
	got, err := dsqlDSN("postgres://automata_runtime:sometoken@cluster.dsql.eu-west-1.on.aws:5432/postgres")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "sometoken") {
		t.Fatalf("dsn carries a credential: %s", got)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		t.Errorf("dsn has a password set: %s", got)
	}
	if u.User.Username() != "automata_runtime" {
		t.Errorf("username = %q, want automata_runtime", u.User.Username())
	}
}

func TestDSQLDSNSetsTLSAndSearchPath(t *testing.T) {
	t.Setenv("DSQL_SCHEMA", "automata")
	got, err := dsqlDSN("postgres://automata_runtime@cluster.dsql.eu-west-1.on.aws:5432/postgres")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("sslmode") != "require" {
		t.Errorf("sslmode = %q, want require", q.Get("sslmode"))
	}
	// Custom roles cannot be granted usage on public, so the app schema has to
	// lead the search path.
	if q.Get("search_path") != "automata,public" {
		t.Errorf("search_path = %q, want automata,public", q.Get("search_path"))
	}
}

func TestDSQLDSNKeepsExplicitSSLMode(t *testing.T) {
	got, err := dsqlDSN("postgres://u@cluster.dsql.eu-west-1.on.aws:5432/postgres?sslmode=verify-full")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(got)
	if u.Query().Get("sslmode") != "verify-full" {
		t.Errorf("sslmode = %q, want verify-full", u.Query().Get("sslmode"))
	}
}

func TestDSQLDSNRejectsBadSchema(t *testing.T) {
	t.Setenv("DSQL_SCHEMA", "drop table; --")
	if _, err := dsqlDSN("postgres://u@cluster.dsql.eu-west-1.on.aws:5432/postgres"); err == nil {
		t.Fatal("expected an error for an invalid schema identifier")
	}
}

func TestDSQLDSNRequiresHost(t *testing.T) {
	if _, err := dsqlDSN("not-a-url"); err == nil {
		t.Fatal("expected an error for a url without a host")
	}
}
