package jobkit

import (
	"strconv"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

var deterministicNamespace = uuid.MustParse("92f8f88d-153a-4414-8348-4a4e8b86b7a2")

func DeterministicID(runID uuid.UUID, scope string, parts ...string) uuid.UUID {
	key := runID.String() + ":" + strings.TrimSpace(scope)
	for _, part := range parts {
		key += ":" + strings.TrimSpace(part)
	}
	return uuid.NewSHA1(deterministicNamespace, []byte(key))
}

func DecodeOffsetCursor(cursor *driven.JobCursor) int {
	if cursor == nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(cursor.Value))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func EncodeOffsetCursor(offset int) *driven.JobCursor {
	if offset < 0 {
		offset = 0
	}
	return &driven.JobCursor{Kind: "message_keyset", Value: strconv.Itoa(offset)}
}

// EncodeKeysetCursor encodes a position in a received_at DESC, id DESC scan.
//
// Offset paging is unsafe here: assignment mutates the set between chunks, so
// rows shift under the cursor and messages get skipped. A keyset is stable
// because it names the last row seen rather than a count of rows behind it.
func EncodeKeysetCursor(receivedAt time.Time, id uuid.UUID) *driven.JobCursor {
	return &driven.JobCursor{
		Kind:  "message_keyset",
		Value: receivedAt.UTC().Format(time.RFC3339Nano) + "|" + id.String(),
	}
}

// DecodeKeysetCursor reverses EncodeKeysetCursor. ok is false for a missing or
// unparseable cursor, which callers treat as "start from the beginning".
func DecodeKeysetCursor(cursor *driven.JobCursor) (receivedAt time.Time, id uuid.UUID, ok bool) {
	if cursor == nil {
		return time.Time{}, uuid.Nil, false
	}
	raw := strings.TrimSpace(cursor.Value)
	sep := strings.LastIndex(raw, "|")
	if sep <= 0 {
		return time.Time{}, uuid.Nil, false
	}
	at, err := time.Parse(time.RFC3339Nano, raw[:sep])
	if err != nil {
		return time.Time{}, uuid.Nil, false
	}
	parsedID, err := uuid.Parse(raw[sep+1:])
	if err != nil {
		return time.Time{}, uuid.Nil, false
	}
	return at.UTC(), parsedID, true
}
