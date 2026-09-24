package http

import "testing"

// Two-character slugs used to be rejected, so a category called "HR" or "IT"
// could not be created at all.
func TestCategorySlugsOfAnyLengthUpTo64(t *testing.T) {
	ok := []string{"a", "hr", "it", "finance", "travel-expenses", "a_b"}
	bad := []string{"", "-hr", "hr-", "_x", "HR!", "has space"}
	for _, s := range ok {
		if _, err := normalizeCategoryInput(categoryUpsertBody{Slug: s, DisplayName: "x"}); err != nil {
			t.Errorf("%q refused: %v", s, err)
		}
	}
	for _, s := range bad {
		if _, err := normalizeCategoryInput(categoryUpsertBody{Slug: s, DisplayName: "x"}); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := normalizeCategoryInput(categoryUpsertBody{Slug: string(long), DisplayName: "x"}); err == nil {
		t.Error("a 65-character slug was accepted")
	}
}
