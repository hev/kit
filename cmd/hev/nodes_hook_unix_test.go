//go:build !windows

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/hooks"
	"github.com/hev/kit/internal/nodes"
	"golang.org/x/crypto/ssh"
)

func TestClusterCommandsEmitOnlyAcceptedMembershipChanges(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "hooks.toml"))
	t.Setenv("HEV_HOOKS", "")
	t.Setenv("PATH", home)
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(home, "host.pub")
	if err := os.WriteFile(keyFile, ssh.MarshalAuthorizedKey(key), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(wantError bool, args ...string) string {
		t.Helper()
		cmd := newClusterCommand()
		cmd.SilenceErrors, cmd.SilenceUsage = true, true
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); (err != nil) != wantError {
			t.Fatalf("%s: %v", args[0], err)
		}
		return strings.TrimSpace(out.String())
	}
	invite := func(name string) string {
		return run(false, "invite", name, "--long", "--address", "example.test", "--user", "bot", "--host-key", keyFile)
	}
	run(false, "invite", "unused", "--address", "example.test", "--user", "bot", "--host-key", keyFile)
	run(false, "remove", "unused")
	run(true, "remove", "unused")
	var i nodes.Invite
	if err := nodes.Decode(invite("member"), &i); err != nil {
		t.Fatal(err)
	}
	request, err := nodes.Encode(nodes.Request{Invite: i, Key: string(ssh.MarshalAuthorizedKey(key))})
	if err != nil {
		t.Fatal(err)
	}
	run(false, "accept", request)
	run(true, "accept", request)
	run(false, "remove", "member")
	run(true, "remove", "member")
	raw, err := os.ReadFile(filepath.Join(home, ".hev", "hooks", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cluster-invite/unused", "cluster-invite/member", "cluster-join/member", "cluster-remove/member"}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
	if len(lines) != len(want) {
		t.Fatalf("unexpected event count: %s", raw)
	}
	for j, line := range lines {
		var ev hooks.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			t.Fatal(err)
		}
		name, _ := ev.Data["name"].(string)
		if ev.Event+"/"+name != want[j] || ev.Data["kind"] != "member" || len(ev.Data) != 2 {
			t.Fatalf("event %d: %s", j, line)
		}
	}
}
