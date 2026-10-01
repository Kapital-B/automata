package postgres

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This adapter also serves Aurora DSQL, which CI cannot run. Row-value
// comparison such as (received_at, id) < (?, ?) is not documented as
// supported there, and request-time SQL is not exercised by a deploy, so a
// regression would only surface as a failing page on dev. Keyset predicates
// are written out as a < ? OR (a = ? AND b < ?) instead.
var rowValueComparisonRE = regexp.MustCompile(`\(\s*[\w.]+\s*,\s*[\w.]+\s*\)\s*(<=|>=|<|>)\s*\(`)

func TestNoRowValueComparisons(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if rowValueComparisonRE.MatchString(line) {
				t.Errorf("%s:%d: row-value comparison; expand it for DSQL: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}
