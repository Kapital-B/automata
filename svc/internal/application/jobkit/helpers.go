package jobkit

import (
	"strconv"
	"strings"

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
