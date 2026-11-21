//go:build !windows

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/hooks"
	"github.com/hev/kit/internal/nodes"
	"github.com/hev/kit/internal/testenv"
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

func TestEncryptedClusterHooksOnlyOnMembershipChanges(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HEV_HOOKS", "")
	t.Setenv("PATH", home)
	config := filepath.Join(home, "config.toml")
	t.Setenv("HEV_CONFIG", config)
	t.Setenv(cluster.EndpointEnv, filepath.Join(testenv.SocketDir(t), "cluster.sock"))
	if err := os.WriteFile(config, []byte("[cluster]\nname='test-member'\nbind='127.0.0.1:0'\nkey_ref='op://fixture/cluster/key'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := cluster.LoadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	loader := func(context.Context, string) ([]byte, time.Time, error) {
		return bytes.Repeat([]byte{42}, 32), time.Time{}, nil
	}
	seedCfg := *cfg
	seedCfg.Name = "test-seed"
	seed := cluster.NewService(ctx, &seedCfg, loader, cluster.LocalFacts(seedCfg.Name, false, 0), nil)
	defer seed.Close()
	member := cluster.NewService(ctx, cfg, loader, cluster.LocalFacts(cfg.Name, false, 0), nil)
	defer member.Close()
	stop, err := cluster.Serve(ctx, member)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	for !seed.Status().Running || !member.Status().Running {
		select {
		case <-ctx.Done():
			t.Fatal("cluster fixture did not start")
		case <-time.After(10 * time.Millisecond):
		}
	}
	seedAddr := seed.Status().Bound
	for _, args := range [][]string{
		{"join", seedAddr}, {"join", seedAddr}, {"leave"}, {"leave"},
		{"join", seedAddr}, {"leave"},
	} {
		cmd := newClusterCommand()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetArgs(args)
		if err := cmd.ExecuteContext(ctx); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(home, ".hev", "hooks", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cluster-join", "cluster-remove", "cluster-join", "cluster-remove"}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
	if len(lines) != len(want) {
		t.Fatalf("unexpected membership event count: %s", raw)
	}
	for i, line := range lines {
		var ev hooks.Event
		if err := json.Unmarshal(line, &ev); err != nil || ev.Event != want[i] ||
			ev.Data["name"] != "test-member" || ev.Data["kind"] != "node" {
			t.Fatalf("event %d: %s (%v)", i, line, err)
		}
	}
}
