package orders

import (
	"testing"

	"github.com/hev/kit/internal/steer"
)

func TestSteerReceiptProjection(t *testing.T) {
	mark := &steer.Mark{Event: "tool", Turn: 3, ToolCall: 12}
	receipt := steer.Message{Redirected: "rework", Injected: mark, Acked: mark, Escalated: mark}
	got := steerReceipt(receipt)
	if got["redirected"] != true {
		t.Fatalf("successful redirect: %v", got["redirected"])
	}
	for _, key := range []string{"injected", "acked", "escalated"} {
		fields := got[key].(map[string]any)
		if fields["turn"] != "3" || fields["tool_call"] != "12" || fields["event"] != "tool" {
			t.Errorf("%s: %v", key, fields)
		}
	}
	failed := steerReceipt(steer.Message{Redirected: "rework failed: internal error"})
	if failed["redirected"] != false {
		t.Fatalf("failed redirect: %v", failed["redirected"])
	}
	queued := steerReceipt(steer.Message{Injected: &steer.Mark{}})
	if _, ok := queued["redirected"]; ok {
		t.Error("an unattempted redirect must omit redirected")
	}
	if _, ok := queued["acked"]; ok {
		t.Error("an absent mark must be omitted")
	}
	fields := queued["injected"].(map[string]any)
	for _, key := range []string{"turn", "tool_call"} {
		if _, ok := fields[key]; ok {
			t.Errorf("zero %s must be omitted", key)
		}
	}
}
