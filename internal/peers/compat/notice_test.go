package compat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoticeRelayUsesBeaconAndRecipientReceipt(t *testing.T) {
	state := t.TempDir()
	t.Setenv("HEV_PEERS_STATE", state)
	path := filepath.Join(state, "hosts", "macbook.json")
	snap := NewObj("host", "macbook", "at", now(), "panes", []any{NewObj("address", "owner", "inbox", true, "recipient_incarnation", "one", "role", "foh")})
	if e := writeJSON(path, snap); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		handled, e := RelayNotice(state, "Owners-MacBook", "owner", "worker/o-test", "stable", "review ready")
		if !handled || e == nil || !strings.Contains(e.Error(), "relay pending") {
			t.Fatal(handled, e)
		}
	}
	if n := len(glob(filepath.Join(state, "outbox", "*.json"))); n != 1 {
		t.Fatal(n)
	}
	snap.Set("relayed", []any{NewObj("id", "stable", "from_host", "worker", "to", "owner", "status", "held unread in inbox; recipient acknowledgment is not yet recorded")})
	if e := writeJSON(path, snap); e != nil {
		t.Fatal(e)
	}
	if handled, e := RelayNotice(state, "Owners-MacBook", "owner", "worker/o-test", "stable", "review ready"); !handled || e != nil {
		t.Fatal(handled, e)
	}
	snap.Set("relayed", []any{}).Set("at", now()-stale-1)
	if e := writeJSON(path, snap); e != nil {
		t.Fatal(e)
	}
	if handled, e := RelayNotice(state, "macbook", "owner", "worker/o-test", "new", "review ready"); !handled || e == nil || !strings.Contains(e.Error(), "pull required") {
		t.Fatal(handled, e)
	}
	if handled, e := RelayNotice(state, "absent", "owner", "worker/o-test", "new", "review ready"); handled || e != nil {
		t.Fatal(handled, e)
	}
	_ = os.Remove(path)
}
