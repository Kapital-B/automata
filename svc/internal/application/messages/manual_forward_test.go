package messages

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestManualForwardMessageAllowlisted(t *testing.T) {
	graph := &fakeForwardGraph{}
	db, svc, _, userID, _, messageID := setupForwardRulesService(t, graph)
	err := svc.ManualForwardMessage(context.Background(), userID, messageID, "bills@example.com", "fyi")
	if err != nil {
		t.Fatal(err)
	}
	if graph.forwardCalls != 1 {
		t.Fatalf("want 1 graph forward, got %d", graph.forwardCalls)
	}
	var forwarded int
	if err := db.QueryRow(`SELECT COUNT(1) FROM manual_forward_audit WHERE message_id = ? AND status = 'forwarded'`, messageID.String()).Scan(&forwarded); err != nil {
		t.Fatal(err)
	}
	if forwarded != 1 {
		t.Fatalf("want 1 forwarded manual audit, got %d", forwarded)
	}
}

func TestManualForwardMessageRejectsNotOnAllowlist(t *testing.T) {
	graph := &fakeForwardGraph{}
	_, svc, _, userID, _, _ := setupForwardRulesService(t, graph)
	err := svc.ManualForwardMessage(context.Background(), userID, uuid.Nil, "other@example.com", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if graph.forwardCalls != 0 {
		t.Fatalf("expected no graph calls, got %d", graph.forwardCalls)
	}
}

func TestManualForwardMessageFailedGraphWritesAudit(t *testing.T) {
	graph := &fakeForwardGraph{forwardErr: errors.New("graph down")}
	db, svc, _, userID, _, messageID := setupForwardRulesService(t, graph)
	err := svc.ManualForwardMessage(context.Background(), userID, messageID, "bills@example.com", "")
	if err == nil {
		t.Fatal("expected error")
	}
	var failed int
	if err := db.QueryRow(`SELECT COUNT(1) FROM manual_forward_audit WHERE message_id = ? AND status = 'failed'`, messageID.String()).Scan(&failed); err != nil {
		t.Fatal(err)
	}
	if failed != 1 {
		t.Fatalf("want 1 failed manual audit, got %d", failed)
	}
}
