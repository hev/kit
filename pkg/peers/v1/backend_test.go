package peers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRPCBackendSeparatesSentAndInbox(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "orders")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_ORDERS_DIR", root)
	t.Setenv("HEV_PEERS_STATE", filepath.Join(home, "peers"))
	t.Setenv("HEV_WORKSPACE", "")
	t.Setenv("PEERS_SELF", "worker")
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	socketDir, err := os.MkdirTemp("/tmp", "hev-peer-api-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	t.Setenv("TMUX_TMPDIR", socketDir)
	for _, name := range []string{"one", "two"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(`{"name":"`+name+`","harness":"codex"}`), 0600); err != nil {
			t.Fatal(err)
		}
	}

	backend := RPCBackend{"team": {Host: "worker"}}
	call := func(q Request, want int) Reply {
		t.Helper()
		out, err := backend.Execute(context.Background(), "team", q)
		if err != nil || out.Status != want {
			t.Fatalf("%s %s: status %d, want %d, error %v", q.Action, q.ID, out.Status, want, err)
		}
		return out
	}
	send := func(from, to, id string) {
		t.Helper()
		body, err := json.Marshal(map[string]string{"id": id, "from": "worker/d-" + from, "message": id})
		if err != nil {
			t.Fatal(err)
		}
		call(Request{Action: "send", ID: "worker:d-" + to, CallerHost: "worker", Body: body}, 201)
	}
	send("one", "two", "first-outbound")
	send("two", "one", "inbound")
	send("one", "two", "second-outbound")
	read := func(box string, want ...string) {
		t.Helper()
		out := call(Request{Action: "messages", ID: "worker:d-one", Box: box}, 200)
		var body struct {
			Data []struct{ ID string }
		}
		if err := json.Unmarshal(out.Body, &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Data) != len(want) {
			t.Fatalf("%q has %d messages, want %d", box, len(body.Data), len(want))
		}
		for i, id := range want {
			if body.Data[i].ID != id {
				t.Fatalf("%q message %d = %q, want %q", box, i, body.Data[i].ID, id)
			}
		}
	}
	read("inbox", "inbound")
	read("sent", "second-outbound", "first-outbound")
}
