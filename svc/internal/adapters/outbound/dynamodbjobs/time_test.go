package dynamodbjobs

import (
	"testing"
	"time"
)

func TestParseTimeFractionalPrecision(t *testing.T) {
	cases := []string{
		"2026-09-01T14:23:08.345183Z",      // microsecond (ops / Python %f)
		"2026-09-01T14:23:08.345183000Z",   // nanosecond padded
		"2026-09-01T14:23:08.345183123Z",   // full nanosecond
		"2026-09-01T14:23:08Z",             // whole second
		formatTime(time.Date(2026, 9, 1, 14, 23, 8, 345183000, time.UTC)),
	}
	for _, raw := range cases {
		got, err := parseTime(raw)
		if err != nil {
			t.Fatalf("parseTime(%q): %v", raw, err)
		}
		if got.IsZero() {
			t.Fatalf("parseTime(%q): zero time", raw)
		}
	}
}
