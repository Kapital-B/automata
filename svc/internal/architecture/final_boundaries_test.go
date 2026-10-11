package architecture

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFinalMigrationHasNoCompatibilitySeams(t *testing.T) {
	data, err := os.ReadFile("testdata/migration_exceptions.json")
	if err != nil {
		t.Fatal(err)
	}
	var debt baseline
	if err = json.Unmarshal(data, &debt); err != nil {
		t.Fatal(err)
	}
	if len(debt.Imports) != 0 || len(debt.Contracts) != 0 {
		t.Fatal("completed migration must have no exceptions")
	}
	appRoot := filepath.Join("..", "application")
	if _, err = os.Stat(filepath.Join(appRoot, "ports")); !os.IsNotExist(err) {
		t.Fatal("application/ports compatibility package must be removed")
	}
	err = filepath.WalkDir(appRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		domains := map[string]bool{}
		for _, im := range f.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			if strings.Contains(p, "/internal/domain/") {
				alias := filepath.Base(p)
				if im.Name != nil {
					alias = im.Name.Name
				}
				domains[alias] = true
			}
		}
		for _, decl := range f.Decls {
			g, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range g.Specs {
				var name string
				var value ast.Expr
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Assign.IsValid() {
						name = s.Name.Name
						value = s.Type
					}
				case *ast.ValueSpec:
					if len(s.Names) == 1 && len(s.Values) == 1 {
						name = s.Names[0].Name
						value = s.Values[0]
					}
				}
				if name == "_" {
					continue
				}
				if sel, ok := value.(*ast.SelectorExpr); ok {
					if x, ok := sel.X.(*ast.Ident); ok && domains[x.Name] {
						t.Errorf("%s: residual domain compatibility symbol %s", path, name)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
