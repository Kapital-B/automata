package sqlkit

import (
	"testing"

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
