package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLayerRules(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		blocked  bool
	}{
		{"internal/domain/facts", "internal/domain/projects", false},
		{"internal/domain/facts", "internal/application/facts", true},
		{"internal/domain/facts", "internal/domainish/helper", true},
		{"internal/domain/facts", "github.com/google/uuid", false},
		{"internal/domain/facts", "github.com/aws/aws-sdk-go-v2/aws", true},
		{"internal/domain/facts", "net/http", true},
		{"internal/domain/facts", "database/sql/driver", true},
		{"internal/application/auth", "internal/domain/auth", false},
		{"internal/application/auth", "internal/application/accounts", false},
		{"internal/application/auth", "internal/configuration", true},
		{"internal/application/auth", "internal/composition", true},
		{"internal/application/auth", "internal/adapters/outbound/security", true},
		{"internal/application/auth", "golang.org/x/crypto/bcrypt", true},
		{"internal/application/auth", "internal/helper", true},
		{"internal/adapters/inbound/http", "internal/domain/accounts", false},
		{"internal/adapters/inbound/http", "internal/application/accounts", true},
		{"internal/adapters/inbound/http", "internal/adapters/outbound/security", true},
		{"internal/adapters/inbound/http", "internal/adapters/shared/json", false},
		{"internal/adapters/outbound/llm", "internal/adapters/outbound/mailmime", false},
		{"internal/adapters/outbound/llm", "internal/adapters/inbound/http", true},
		{"internal/adapters/outbound/llm", "internal/application/ports/driven", true},
		{"internal/composition", "internal/application/auth", false},
		{"cmd/server", "internal/composition", false},
		{"cmd/server", "internal/adapters/outbound/persistencetest", true},
	} {
		t.Run(tc.from+"->"+tc.to, func(t *testing.T) {
			if got := forbidden(tc.from, tc.to); got != tc.blocked {
				t.Fatalf("blocked=%v, want %v", got, tc.blocked)
			}
		})
	}
}

func fixturePackage(imports ...string) *sourcePackage {
	p := &sourcePackage{imports: map[string]string{}, tests: map[string]string{}, contracts: map[string]string{}}
	for _, dep := range imports {
		p.imports[dep] = "fixture.go:1"
	}
	return p
}

func TestTransitiveViolationsCannotInheritExceptions(t *testing.T) {
	packages := map[string]*sourcePackage{
		"internal/application/auth":   fixturePackage("internal/application/helper"),
		"internal/application/helper": fixturePackage("database/sql", "internal/application/auth"), // includes a cycle
	}
	base := baseline{Imports: []importException{{dependency{"internal/application/helper", "database/sql"}, "R2", "existing SQL coupling"}}}
	problems := baselineProblems(packages, base)
	if len(problems) != 1 || !strings.Contains(problems[0], "internal/application/auth -> internal/application/helper -> database/sql") {
		t.Fatalf("expected a new transitive violation, got %v", problems)
	}
}

func TestMigrationBaselineRejectsStaleAndDuplicateExceptions(t *testing.T) {
	packages := map[string]*sourcePackage{"internal/application/auth": fixturePackage("database/sql")}
	allowed := importException{dependency{"internal/application/auth", "database/sql"}, "R2", "SQL coupling"}
	if problems := baselineProblems(packages, baseline{Imports: []importException{allowed}}); len(problems) != 0 {
		t.Fatal(problems)
	}
	if problems := baselineProblems(packages, baseline{Imports: []importException{allowed, allowed}}); len(problems) != 1 || !strings.Contains(problems[0], "duplicate") {
		t.Fatalf("expected duplicate error, got %v", problems)
	}
	packages["internal/application/auth"] = fixturePackage()
	if problems := baselineProblems(packages, baseline{Imports: []importException{allowed}}); len(problems) != 1 || !strings.Contains(problems[0], "stale") {
		t.Fatalf("expected stale error, got %v", problems)
	}
}

func TestDomainAndTestSupportCannotHaveExceptions(t *testing.T) {
	for _, dep := range []dependency{
		{"internal/domain/facts", "database/sql"},
		{"internal/domain/facts (test)", "database/sql"},
		{"cmd/server", "internal/adapters/outbound/persistencetest"},
	} {
		t.Run(dep.From+"->"+dep.To, func(t *testing.T) {
			packages := map[string]*sourcePackage{dep.From: fixturePackage(dep.To)}
			problems := baselineProblems(packages, baseline{Imports: []importException{{dep, "R2", "not allowed"}}})
			if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"), "invalid exception") {
				t.Fatalf("expected unexemptable boundary, got %v", problems)
			}
		})
	}
}

func TestTestImportsStaySeparateFromProduction(t *testing.T) {
	app := fixturePackage()
	app.tests["internal/adapters/outbound/persistence/sqlite"] = "service_test.go:5"
	domain := fixturePackage()
	domain.tests["internal/application/facts"] = "fact_test.go:5"
	packages := map[string]*sourcePackage{
		"internal/application/facts": app,
		"internal/domain/facts":      domain,
	}
	got := violations(packages)
	want := dependency{"internal/domain/facts (test)", "internal/application/facts"}
	if len(got) != 1 || got[want] == "" {
		t.Fatalf("expected only domain test violation, got %v", got)
	}
	delete(packages, "internal/domain/facts")
	packages["internal/adapters/outbound/persistencetest"] = fixturePackage("internal/application/projects")
	if got := violations(packages); len(got) != 0 {
		t.Fatalf("test support should not become a production root: %v", got)
	}
}

func TestScanFindsContractsAndBuildVariants(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.org/service\n\ngo 1.24\n")
	write("cmd/server/main.go", "package main\n")
	write("internal/application/jobs/jobs.go", `package jobs
import "example.org/service/internal/domain/jobs"
type Store interface { Save(jobs.Record) error }
type Alias = Store
type Service struct { Execute interface { Run() error }; Data map[string]interface{} }
`)
	write("internal/application/jobs/jobs_windows.go", "//go:build windows\n\npackage jobs\nimport _ \"database/sql\"\n")
	write("internal/application/jobs/jobs_test.go", "package jobs_test\nimport _ \"example.org/service/internal/adapters/outbound/persistence/sqlite\"\n")
	write("internal/application/jobs/testdata/ignored.go", "this is not Go source")
	write("internal/domain/jobs/jobs.go", "package jobs\ntype Record struct{}\n")
	packages, err := scanSource(root)
	if err != nil {
		t.Fatal(err)
	}
	pkg := packages["internal/application/jobs"]
	if len(pkg.contracts) != 2 || pkg.contracts["Store"] == "" || pkg.contracts["Service.Execute"] == "" {
		t.Fatalf("unexpected contracts: %v", pkg.contracts)
	}
	if pkg.imports["database/sql"] == "" || pkg.imports["internal/domain/jobs"] == "" || len(pkg.tests) != 1 {
		t.Fatalf("incorrect import classification: %+v", pkg)
	}
	base := baseline{
		Imports: []importException{{dependency{"internal/application/jobs", "database/sql"}, "R2", "SQL coupling"}},
		Contracts: []contractException{
			{"internal/application/jobs", "Store", "R1", "port ownership"},
			{"internal/application/jobs", "Service.Execute", "R1", "inline port ownership"},
		},
	}
	if problems := baselineProblems(packages, base); len(problems) != 0 {
		t.Fatal(problems)
	}
	delete(pkg.contracts, "Store")
	if problems := baselineProblems(packages, base); len(problems) != 1 || !strings.Contains(problems[0], "stale contract") {
		t.Fatalf("expected stale contract exception, got %v", problems)
	}
	pkg.contracts["NewPort"] = "new.go:2"
	if problems := baselineProblems(packages, base); !strings.Contains(strings.Join(problems, "\n"), "new application-owned contract") {
		t.Fatalf("expected new port rejection, got %v", problems)
	}
}
