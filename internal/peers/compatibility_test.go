package peers

import (
	"strings"
	"testing"
)

func TestSendProvenanceRetryConflict(t *testing.T) {
	e, call := fixture(t)
	args := []string{"send", "desk", "  hello\n", "--from", "sender", "--id", "stable", "--order", "order-1", "--run", "run-1"}
	if _, err := call("", args...); err != nil {
		t.Fatal(err)
	}
	ms, err := rows(e.path("desk"))
	if err != nil || len(ms) != 1 {
		t.Fatal(ms, err)
	}
	if ms[0]["message"] != "hello" || ms[0]["order"] != "order-1" || ms[0]["run"] != "run-1" {
		t.Fatal(ms)
	}
	args[len(args)-1] = "run-2"
	if _, err := call("", args...); err == nil {
		t.Fatal("retry changed provenance")
	}
}

func TestNativeRuntimeTokenFailsClosed(t *testing.T) {
	_, call := fixture(t)
	t.Setenv("PEERS_INBOX_TOKEN", strings.Repeat("a", 64))
	if _, err := call("", "inbox", "owner"); err == nil {
		t.Fatal("unsupported token fell back to pane ownership")
	}
	if _, err := call("", "inbox", "--session", "desk", "--peek", "--json"); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidReceiptTimestampDoesNotAcknowledge(t *testing.T) {
	e, call := fixture(t)
	if _, err := call("hello", "inbox", "hold", "--id", "one"); err != nil {
		t.Fatal(err)
	}
	ms, err := rows(e.path("desk"))
	if err != nil {
		t.Fatal(err)
	}
	if err := appendRow(e.path("desk")+".receipts.jsonl", Message{"id": "one", "recipient_incarnation": ms[0]["recipient_incarnation"], "kind": "acknowledged", "ts": "invalid"}); err != nil {
		t.Fatal(err)
	}
	out, err := call("", "inbox", "--peek", "--pending", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if decode(t, out)["unacknowledged"] != float64(1) {
		t.Fatal(out)
	}
}

func TestPythonCompatibleIdentifierAndBatchBounds(t *testing.T) {
	_, call := fixture(t)
	for _, args := range [][]string{{"send", "desk", "body", "--id", "bad/id", "--from", "sender"}, {"inbox", "--limit", "65"}} {
		if _, err := call("", args...); err == nil {
			t.Fatal(args)
		}
	}
}

func TestInboxOnlyOpaqueNoticeID(t *testing.T) {
	_, call := fixture(t)
	if _, err := call("", "send", "desk", "notice", "--from", "sender", "--inbox-only", "--id", "host/o-task:reply:stable"); err != nil {
		t.Fatal(err)
	}
}
