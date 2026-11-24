//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	peers "github.com/hev/kit/pkg/peers/v1"
)

func TestRelayPeerRolesAndOfflineDelivery(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is unavailable")
	}
	home := t.TempDir()
	socketDir, err := os.MkdirTemp("/tmp", "hev-relay-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("HEV_ORDERS_DIR", filepath.Join(home, "orders"))
	t.Setenv("HEV_PEERS_STATE", filepath.Join(home, "peers"))
	t.Setenv("HEV_WORKSPACE", "")
	t.Setenv("PEERS_SELF", "local")
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("TMUX_TMPDIR", socketDir)

	socket := filepath.Join(socketDir, "tmux-"+strconv.Itoa(os.Getuid()), "default")
	if err := os.Mkdir(filepath.Dir(socket), 0700); err != nil {
		t.Fatal(err)
	}

	tmux := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		args = append([]string{"-S", socket}, args...)
		if out, err := exec.CommandContext(ctx, "tmux", args...).CombinedOutput(); err != nil {
			t.Fatalf("tmux %v: %v (%s)", args, err, out)
		}
	}
	roles := map[string]string{"desk": "foh", "desk-other": "boh", "quiet": "offline"}
	for name, role := range roles {
		tmux("-f", "/dev/null", "new-session", "-d", "-s", name, "-c", home, "/bin/sleep", "90")
		t.Cleanup(func() { tmux("kill-session", "-t", "="+name+":") })
		tmux("set-option", "-t", "="+name+":", "@peers-inbox", "on")
		tmux("set-option", "-t", "="+name+":", "@peers-role", role)
	}

	backend := peers.RPCBackend{"team": {Host: "local"}}
	ctx := context.Background()
	call := func(req peers.Request, want int) peers.Reply {
		t.Helper()
		out, err := backend.Execute(ctx, "team", req)
		if err != nil || out.Status != want {
			t.Fatalf("%s %s: status %d, want %d, error %v", req.Action, req.ID, out.Status, want, err)
		}
		return out
	}
	out := call(peers.Request{Action: "list"}, 200)
	var rows []struct{ Name, Role string }
	if err := json.Unmarshal(out.Body, &rows); err != nil {
		t.Fatal(err)
	}
	discovered := map[string]string{}
	for _, row := range rows {
		discovered[row.Name] = row.Role
	}
	if !reflect.DeepEqual(discovered, roles) {
		t.Fatalf("discovered roles = %v, want %v", discovered, roles)
	}

	message := json.RawMessage(`{"id":"role-test","from":"local/desk-other","message":"hello"}`)
	for _, action := range []string{"send", "deliver"} {
		call(peers.Request{Action: action, ID: "local:quiet", CallerHost: "local", Body: message}, 403)
	}
	call(peers.Request{Action: "send", ID: "local:desk", CallerHost: "local", Body: message}, 201)
	call(peers.Request{Action: "deliver", ID: "local:desk", CallerHost: "local", Body: message}, 200)

	type heartbeat struct{ Path, Role string }
	published := make(chan heartbeat, len(roles))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Role string }
		if r.Method != http.MethodPut || json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		published <- heartbeat{r.URL.Path, body.Role}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := relayPeerPresence(ctx, server.URL, "local", "synthetic-key"); err != nil {
		t.Fatal(err)
	}
	close(published)
	heartbeats := map[string]string{}
	for h := range published {
		heartbeats[h.Path] = h.Role
	}
	want := map[string]string{"/v1/peers/local:desk": "foh", "/v1/peers/local:desk-other": "boh"}
	if !reflect.DeepEqual(heartbeats, want) {
		t.Fatalf("published roles = %v, want %v", heartbeats, want)
	}
}
