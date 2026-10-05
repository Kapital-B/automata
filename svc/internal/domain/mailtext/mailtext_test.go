package mailtext

import "testing"

func TestStripHTML(t *testing.T) {
	in := `<html><body><p>Duty is 90&nbsp;kW &amp; rising</p></body></html>`
	if !LooksLikeHTML(in) {
		t.Fatal("expected HTML to be detected")
	}
	got := StripHTML(in)
	if want := "Duty is 90 kW & rising"; !contains(got, want) {
		t.Fatalf("StripHTML = %q, want it to contain %q", got, want)
	}
	if LooksLikeHTML("plain text, 3 < 4") {
		t.Fatal("plain text detected as HTML")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
