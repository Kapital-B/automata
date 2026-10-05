// Package mailtext holds small text helpers for mail bodies, shared so that
// previews, prompts and search snippets read a body the same way.
package mailtext

import (
	"html"
	"strings"
)

// LooksLikeHTML reports whether a mail body appears to be HTML.
func LooksLikeHTML(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "<!doctype") ||
		strings.Contains(lower, "<html") ||
		strings.Contains(lower, "<head") ||
		strings.Contains(lower, "<body") ||
		strings.Contains(lower, "<div") ||
		strings.Contains(lower, "<span") ||
		strings.Contains(lower, "<p") ||
		strings.Contains(lower, "<a ") ||
		strings.Contains(lower, "<img") ||
		strings.Contains(lower, "<ul") ||
		strings.Contains(lower, "<li") ||
		strings.Contains(lower, "<table") ||
		strings.Contains(lower, "<br")
}

// StripHTML reduces an HTML body to its text, for previews, snippets and
// LLM prompts. It is deliberately minimal: tags become spaces and entities
// are unescaped.
func StripHTML(s string) string {
	// Minimal HTML fallback for previews and LLM prompts.
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			if b.Len() > 0 {
				b.WriteRune(' ')
			}
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return s
	}
	return html.UnescapeString(out)
}
