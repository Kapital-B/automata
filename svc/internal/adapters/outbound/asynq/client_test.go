package asynqadapter

import (
	"encoding/json"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	"github.com/google/uuid"
	"testing"
)

func TestLegacyTaskPayloadPreservesWireProtocol(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	raw, err := json.Marshal(taskPayload(domainjobs.LegacyJobRequest{RunID: id, UserID: id, AccountID: id, TriggerKind: "api"}))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema_version":1,"run_id":"11111111-1111-1111-1111-111111111111","user_id":"11111111-1111-1111-1111-111111111111","account_id":"11111111-1111-1111-1111-111111111111","connector_account_id":"00000000-0000-0000-0000-000000000000","trigger_kind":"api"}`
	if string(raw) != want {
		t.Fatalf("wire payload changed: %s", raw)
	}
}
