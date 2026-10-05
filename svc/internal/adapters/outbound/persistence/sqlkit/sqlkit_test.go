package sqlkit

import (
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func TestPlaceholderList(t *testing.T) {
	cases := map[int]string{0: "", 1: "?", 3: "?,?,?"}
	for n, want := range cases {
		if got := PlaceholderList(n); got != want {
			t.Errorf("PlaceholderList(%d) = %q, want %q", n, got, want)
		}
	}
	if got := PlaceholderList(-1); got != "" {
		t.Errorf("negative count = %q, want empty", got)
	}
}

func TestDedupeUUIDsPreservesOrder(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	got := DedupeUUIDs([]uuid.UUID{a, b, a, c, b, a})
	want := []uuid.UUID{a, b, c}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestChunkUUIDs(t *testing.T) {
	ids := make([]uuid.UUID, 7)
	for i := range ids {
		ids[i] = uuid.New()
	}
	chunks := ChunkUUIDs(ids, 3)
	if len(chunks) != 3 {
		t.Fatalf("chunks = %d, want 3", len(chunks))
	}
	if len(chunks[0]) != 3 || len(chunks[1]) != 3 || len(chunks[2]) != 1 {
		t.Fatalf("chunk sizes = %d/%d/%d, want 3/3/1", len(chunks[0]), len(chunks[1]), len(chunks[2]))
	}
	seen := 0
	for _, c := range chunks {
		seen += len(c)
	}
	if seen != len(ids) {
		t.Errorf("chunking lost ids: %d of %d", seen, len(ids))
	}
}

// An empty id set must produce no chunks, so callers issue no statement at all
// rather than an IN () that no dialect accepts.
func TestChunkUUIDsEmptyYieldsNoChunks(t *testing.T) {
	if got := ChunkUUIDs(nil, 10); len(got) != 0 {
		t.Fatalf("chunks = %d, want 0", len(got))
	}
	if got := ChunkUUIDs([]uuid.UUID{}, 10); len(got) != 0 {
		t.Fatalf("chunks = %d, want 0", len(got))
	}
}

func TestUUIDArgsKeepsOrder(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	got := UUIDArgs([]uuid.UUID{a, b})
	if len(got) != 2 || got[0] != a.String() || got[1] != b.String() {
		t.Fatalf("UUIDArgs = %v", got)
	}
	if len(UUIDArgs(nil)) != 0 {
		t.Fatal("nil ids must give no args")
	}
}

func TestLikeContainsEscapesWildcards(t *testing.T) {
	for in, want := range map[string]string{
		"p-03":   "%p-03%",
		"100%":   `%100\%%`,
		"a_b":    `%a\_b%`,
		`c:\tmp`: `%c:\\tmp%`,
	} {
		if got := LikeContains(in); got != want {
			t.Errorf("LikeContains(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMessageSearchQueryNeedsSomethingToSearchBy(t *testing.T) {
	q, args := MessageSearchQuery(uuid.New(), driven.MessageSearchFilter{Terms: []string{" ", ""}}, func(t time.Time) any { return t })
	if q != "" || args != nil {
		t.Fatalf("empty filter built a query: %q %v", q, args)
	}
}

func TestMessageSearchQueryArgsMatchPlaceholders(t *testing.T) {
	since := time.Now()
	q, args := MessageSearchQuery(uuid.New(), driven.MessageSearchFilter{
		Terms:      []string{"Seal", "seal", "p-03"},
		ProjectIDs: []uuid.UUID{uuid.New(), uuid.New()},
		ContactIDs: []uuid.UUID{uuid.New()},
		Since:      &since,
	}, func(t time.Time) any { return t })
	if got, want := strings.Count(q, "?"), len(args); got != want {
		t.Fatalf("%d placeholders, %d args", got, want)
	}
	// Two distinct terms after lowercasing and dedupe, each matched twice.
	if args[0] != "%seal%" || args[2] != "%p-03%" {
		t.Fatalf("term args = %v", args[:4])
	}
}
