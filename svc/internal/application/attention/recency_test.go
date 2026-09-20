package attention

import (
	"testing"
	"time"
)

// TestSortItemsOrdersBySeverityThenRecency pins the ordering Home depends on:
// severity still wins, but within a group the newest item comes first. Before
// this the list was stable-sorted by title, so "recent outstanding actions"
// could not be expressed.
func TestSortItemsOrdersBySeverityThenRecency(t *testing.T) {
	now := time.Now().UTC()
	items := []Item{
		{ID: "c-old", WhyMe: WhyOpenContradiction, Title: "aaa old", OccurredAt: now.Add(-48 * time.Hour)},
		{ID: "f-new", WhyMe: WhyProvisionalFact, Title: "zzz new fact", OccurredAt: now},
		{ID: "c-new", WhyMe: WhyOpenContradiction, Title: "zzz new", OccurredAt: now.Add(-time.Minute)},
		{ID: "f-old", WhyMe: WhyProvisionalFact, Title: "aaa old fact", OccurredAt: now.Add(-72 * time.Hour)},
	}
	sortItems(items)

	got := []string{items[0].ID, items[1].ID, items[2].ID, items[3].ID}
	want := []string{"c-new", "c-old", "f-new", "f-old"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v (contradictions before facts, newest first within each)", got, want)
		}
	}
}

// Items sharing a timestamp must still order deterministically, so a list
// cannot shuffle between refreshes.
func TestSortItemsIsStableOnEqualTimestamps(t *testing.T) {
	at := time.Now().UTC()
	build := func() []Item {
		return []Item{
			{ID: "b", WhyMe: WhyOpenContradiction, Title: "beta", OccurredAt: at},
			{ID: "a", WhyMe: WhyOpenContradiction, Title: "alpha", OccurredAt: at},
			{ID: "c", WhyMe: WhyOpenContradiction, Title: "gamma", OccurredAt: at},
		}
	}
	first := build()
	sortItems(first)
	for i := 0; i < 10; i++ {
		again := build()
		sortItems(again)
		for j := range first {
			if first[j].ID != again[j].ID {
				t.Fatalf("ordering varied at %d: %s vs %s", j, first[j].ID, again[j].ID)
			}
		}
	}
	if first[0].Title != "alpha" {
		t.Errorf("equal timestamps should fall back to title, got %q", first[0].Title)
	}
}
