// Package architecture checks the source boundaries in the domain-owned-ports RFC.
// It parses all source variants, independently of the current platform/build tags.
package architecture

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type sourcePackage struct {
	imports   map[string]string // dependency -> first source location
	tests     map[string]string
	contracts map[string]string // named interface or struct.interfaceField -> location
}

type dependency struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type importException struct {
	dependency
	RemoveBy string `json:"remove_by"`
	Reason   string `json:"reason"`
}

type contractException struct {
	Package  string `json:"package"`
	Name     string `json:"name"`
	RemoveBy string `json:"remove_by"`
	Reason   string `json:"reason"`
}

type baseline struct {
	Imports   []importException   `json:"imports"`
	Contracts []contractException `json:"contracts"`
}

// These reusable integration suites live in ordinary .go files. Only these
// exact packages get test-support rules; production may never depend on them.
var testSupport = map[string]bool{
	"internal/adapters/outbound/jobstoretest":    true,
	"internal/adapters/outbound/mailboxtest":     true,
	"internal/adapters/outbound/persistencetest": true,
}

func within(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func forbidden(from, to string) bool {
	if testSupport[to] {
		return true
	}
	if within(to, "internal") || within(to, "cmd") {
		switch {
		case within(from, "internal/domain"):
			return !within(to, "internal/domain")
		case within(from, "internal/application"):
			return !within(to, "internal/domain") && !within(to, "internal/application")
		case within(from, "internal/adapters/inbound"):
			return !within(to, "internal/domain") && !within(to, "internal/adapters/inbound") && !within(to, "internal/adapters/shared")
		case within(from, "internal/adapters/outbound"):
			return !within(to, "internal/domain") && !within(to, "internal/adapters/outbound") && !within(to, "internal/adapters/shared")
		}
	}
	if within(from, "internal/domain") || within(from, "internal/application") {
		// UUID is an approved value-type library. Do not traverse its private
		// implementation dependencies (which include SQL conversion helpers).
		if strings.Contains(strings.Split(to, "/")[0], ".") {
			return to != "github.com/google/uuid"
		}
		return within(to, "database/sql") || within(to, "net/http")
	}
	return false
}

func scanSource(root string) (map[string]*sourcePackage, error) {
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(mod))
	if len(fields) < 2 || fields[0] != "module" {
		return nil, fmt.Errorf("cannot identify module in %s/go.mod", root)
	}
	prefix := fields[1] + "/"
	packages := map[string]*sourcePackage{}
	for _, subtree := range []string{"internal", "cmd"} {
		err = filepath.WalkDir(filepath.Join(root, subtree), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "testdata" || strings.HasPrefix(entry.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			fset := token.NewFileSet()
			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			name := filepath.ToSlash(filepath.Dir(rel))
			pkg := packages[name]
			if pkg == nil {
				pkg = &sourcePackage{imports: map[string]string{}, tests: map[string]string{}, contracts: map[string]string{}}
				packages[name] = pkg
			}
			isTest := strings.HasSuffix(path, "_test.go")
			imports := pkg.imports
			if isTest {
				imports = pkg.tests
			}
			location := func(pos token.Pos) string {
				return fmt.Sprintf("%s:%d", filepath.ToSlash(rel), fset.Position(pos).Line)
			}
			for _, imp := range file.Imports {
				dep, unquoteErr := strconv.Unquote(imp.Path.Value)
				if unquoteErr != nil {
					return unquoteErr
				}
				dep = strings.TrimPrefix(dep, prefix)
				if _, exists := imports[dep]; !exists {
					imports[dep] = location(imp.Pos())
				}
			}
			if isTest || !within(name, "internal/application") {
				return nil
			}
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range gen.Specs {
					typ, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if iface, ok := typ.Type.(*ast.InterfaceType); ok && len(iface.Methods.List) > 0 {
						pkg.contracts[typ.Name.Name] = location(typ.Pos())
					}
					if st, ok := typ.Type.(*ast.StructType); ok {
						for _, field := range st.Fields.List {
							if iface, ok := field.Type.(*ast.InterfaceType); ok && len(iface.Methods.List) > 0 {
								for _, fieldName := range field.Names {
									pkg.contracts[typ.Name.Name+"."+fieldName.Name] = location(field.Pos())
								}
							}
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return packages, nil
}

// Walk only Automata-owned production packages. Check every reachable import
// against the originating layer, stopping at the first forbidden boundary.
// Baseline permissions are deliberately not consulted during traversal: a new
// caller must not inherit another package's migration exception.
func violations(packages map[string]*sourcePackage) map[dependency]string {
	found := map[dependency]string{}
	for origin, pkg := range packages {
		if testSupport[origin] {
			continue
		}
		walk := func(imports map[string]string, label string) {
			type step struct {
				imports map[string]string
				path    []string
			}
			queue := []step{{imports, []string{label}}}
			seen := map[string]bool{origin: true}
			for len(queue) > 0 {
				cur := queue[0]
				queue = queue[1:]
				for _, dep := range sortedKeys(cur.imports) {
					path := append(append([]string{}, cur.path...), dep)
					if forbidden(origin, dep) {
						key := dependency{label, dep}
						if _, exists := found[key]; !exists {
							found[key] = strings.Join(path, " -> ") + " (" + cur.imports[dep] + ")"
						}
						continue
					}
					if next := packages[dep]; next != nil && !seen[dep] {
						seen[dep] = true
						queue = append(queue, step{next.imports, path})
					}
				}
			}
		}
		walk(pkg.imports, origin)
		// Non-domain tests may wire several layers as integration fixtures.
		// Domain tests have the same isolation requirements as production.
		if within(origin, "internal/domain") {
			walk(pkg.tests, origin+" (test)")
		}
	}
	return found
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func baselineProblems(packages map[string]*sourcePackage, expected baseline) []string {
	actual := violations(packages)
	problems := []string{}
	seenImports := map[dependency]bool{}
	validPhase := func(phase string) bool { return phase == "R1" || phase == "R2" || phase == "R3" || phase == "R4" }
	for _, allowed := range expected.Imports {
		key := allowed.dependency
		if within(key.From, "internal/domain") || testSupport[key.To] || !validPhase(allowed.RemoveBy) || allowed.Reason == "" {
			problems = append(problems, fmt.Sprintf("invalid exception: %s -> %s", key.From, key.To))
		}
		if seenImports[key] {
			problems = append(problems, fmt.Sprintf("duplicate exception: %s -> %s", key.From, key.To))
		}
		seenImports[key] = true
		if _, exists := actual[key]; !exists {
			problems = append(problems, fmt.Sprintf("remove stale exception: %s -> %s", key.From, key.To))
		}
	}
	for key, path := range actual {
		if !seenImports[key] {
			problems = append(problems, "new dependency violation: "+path)
		}
	}
	contracts := map[string]string{}
	for name, pkg := range packages {
		for contract, location := range pkg.contracts {
			contracts[name+":"+contract] = location
		}
	}
	seenContracts := map[string]bool{}
	for _, allowed := range expected.Contracts {
		key := allowed.Package + ":" + allowed.Name
		if !within(allowed.Package, "internal/application") || allowed.RemoveBy != "R1" || allowed.Reason == "" {
			problems = append(problems, "invalid contract exception: "+key)
		}
		if seenContracts[key] {
			problems = append(problems, "duplicate contract exception: "+key)
		}
		seenContracts[key] = true
		if _, exists := contracts[key]; !exists {
			problems = append(problems, "remove stale contract exception: "+key)
		}
	}
	for key, location := range contracts {
		if !seenContracts[key] {
			problems = append(problems, "new application-owned contract: "+key+" ("+location+")")
		}
	}
	sort.Strings(problems)
	return problems
}

func TestServiceArchitecture(t *testing.T) {
	// Go runs each package's tests with that package directory as cwd.
	packages, err := scanSource(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/migration_exceptions.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected baseline
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	for _, problem := range baselineProblems(packages, expected) {
		t.Error(problem)
	}
}
