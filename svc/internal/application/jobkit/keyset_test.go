package jobkit

import (
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func TestKeysetCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, 9, 19, 12, 30, 45, 123456789, time.UTC)
	id := uuid.New()
	gotAt, gotID, ok := DecodeKeysetCursor(EncodeKeysetCursor(at, id))
	if !ok {
		t.Fatal("expected decode to succeed")
	}
	if !gotAt.Equal(at) {
		t.Errorf("received_at = %v, want %v", gotAt, at)
	}
	if gotID != id {
		t.Errorf("id = %v, want %v", gotID, id)
	}
}

func TestKeysetCursorRejectsGarbage(t *testing.T) {
	for _, cursor := range []*driven.JobCursor{
		nil,
		{Kind: "message_keyset", Value: ""},
		{Kind: "message_keyset", Value: "nope"},
		{Kind: "message_keyset", Value: "not-a-time|" + uuid.New().String()},
		{Kind: "message_keyset", Value: time.Now().UTC().Format(time.RFC3339Nano) + "|not-a-uuid"},
		// An old offset cursor must not decode as a keyset.
		{Kind: "message_keyset", Value: "25"},
	} {
		if _, _, ok := DecodeKeysetCursor(cursor); ok {
			t.Errorf("expected decode failure for %+v", cursor)
		}
	}
}
