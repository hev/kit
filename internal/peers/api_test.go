package peers

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAPIInboxRequiresCurrentIncarnationAndConsumeBeforeAck(t *testing.T) {
	e, _ := fixture(t)
	ctx := context.Background()
	const peerID = "local:desk"
	status, body := e.apiExecute(ctx, apiRequest{Action: "get", ID: peerID})
	if status != 200 {
		t.Fatalf("get peer: %d", status)
	}
	incarnation := body.(Message)["recipient_incarnation"].(string)
	send := func(identity string) int {
		body, _ := json.Marshal(Message{"id": "api-test", "from": "remote/sender", "message": "hello", "recipient_incarnation": identity})
		status, _ := e.apiExecute(ctx, apiRequest{Action: "deliver", ID: peerID, CallerHost: "remote", Body: body})
		return status
	}
	if got := send("old-incarnation"); got != 409 {
		t.Fatalf("stale send returned %d", got)
	}
	if got := send(incarnation); got != 201 {
		t.Fatalf("new send returned %d", got)
	}
	if got := send(incarnation); got != 200 {
		t.Fatalf("replayed send returned %d", got)
	}
	receipt := func(kind string) Message {
		body, _ := json.Marshal(Message{"kind": kind, "ids": []string{"api-test"}, "recipient_incarnation": incarnation})
		status, value := e.apiExecute(ctx, apiRequest{Action: "receipts", ID: peerID, CallerHost: "local", Body: body})
		if status != 200 {
			t.Fatalf("receipt response %d", status)
		}
		return value.(Message)
	}
	if got := receipt("acknowledged")["rejected"].([]Message); len(got) != 1 {
		t.Fatalf("acknowledge without consume was accepted")
	}
	if got := receipt("consumed")["recorded"].([]string); len(got) != 1 {
		t.Fatalf("consume not recorded")
	}
	if got := receipt("acknowledged")["recorded"].([]string); len(got) != 1 {
		t.Fatalf("acknowledge not recorded")
	}
}
