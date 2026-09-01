package migrate

import "testing"

func TestParseRuntimeIAMRoleARNs(t *testing.T) {
	arns, err := parseRuntimeIAMRoleARNs(
		"arn:aws:iam::123456789012:role/automata-api-role-dev, arn:aws:iam::123456789012:role/automata-api-role-dev,arn:aws:iam::123456789012:role/automata-worker-role-dev",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(arns) != 2 {
		t.Fatalf("got %d arns, want 2: %#v", len(arns), arns)
	}

	if _, err := parseRuntimeIAMRoleARNs("not-an-arn"); err == nil {
		t.Fatal("expected invalid ARN error")
	}
	if _, err := parseRuntimeIAMRoleARNs("arn:aws:iam::123456789012:user/alice"); err == nil {
		t.Fatal("expected user ARN rejection")
	}
}

func TestQuotePGIdent(t *testing.T) {
	if got := quotePGIdent(`users`); got != `"users"` {
		t.Fatalf("got %q", got)
	}
	if got := quotePGIdent(`weird"name`); got != `"weird""name"` {
		t.Fatalf("got %q", got)
	}
}

func TestDSQLIdentFromEnvFallback(t *testing.T) {
	t.Setenv("DSQL_SCHEMA", "")
	got, err := dsqlIdentFromEnv("DSQL_SCHEMA", defaultDSQLSchema)
	if err != nil {
		t.Fatal(err)
	}
	if got != defaultDSQLSchema {
		t.Fatalf("got %q", got)
	}
	t.Setenv("DSQL_SCHEMA", "not valid")
	if _, err := dsqlIdentFromEnv("DSQL_SCHEMA", defaultDSQLSchema); err == nil {
		t.Fatal("expected invalid ident error")
	}
}
