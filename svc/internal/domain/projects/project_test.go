package projects_test

import (
	"testing"

	"github.com/Kapital-B/automata/svc/internal/domain/projects"
)

func TestNormalizeAndValidCode(t *testing.T) {
	if got := projects.NormalizeCode(" dc01 "); got != "DC01" {
		t.Fatalf("normalize=%q", got)
	}
	cases := map[string]bool{
		"DC01":      true,
		"A1":        true,
		"ABCDEFGH":  true,
		"1DC":       false,
		"D":         false,
		"TOOLONG01": false,
		"dc-01":     false,
		"":          false,
	}
	for code, want := range cases {
		if got := projects.ValidCode(code); got != want {
			t.Fatalf("ValidCode(%q)=%v want %v", code, got, want)
		}
	}
}
