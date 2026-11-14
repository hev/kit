package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/cluster"
)

func TestClusterFactsRenderingAndPrivacy(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	f := cluster.NodeFacts{Version: 1, At: now, Dispatch: &cluster.DispatchFacts{At: now, Cap: 4, Running: 1, Free: 3, Queue: 2}, Sessions: &cluster.SessionFacts{At: now, Harness: map[string]cluster.SessionCount{"claude": {Working: 1, Idle: 2}}}, Build: &cluster.BuildFacts{At: now, Commit: "abcdef012345", Outcome: "ok"}, Credentials: &cluster.CredentialFacts{At: now, Items: map[string]cluster.CredentialFact{"claude": {State: "unknown"}}}}
	m := cluster.Member{Facts: cluster.Facts{Name: "alpha"}, State: "alive", NodeFacts: &f}
	s, slots, build, health, n, free := clusterColumns(m, now)
	if s != "1/3" || slots != "3/4 (2)" || build != "abcdef012345" || health != "ok" || n != 3 || free != 3 {
		t.Fatal(s, slots, build, health, n, free)
	}
	s, slots, build, health, n, free = clusterColumns(m, now.Add(cluster.FactsStale+time.Second))
	if s != "stale" || slots != "stale" || build != "stale" || !strings.Contains(health, "sessions:stale") || n != 0 || free != 0 {
		t.Fatal(s, slots, build, health, n, free)
	}
	var out bytes.Buffer
	if e := printClusterMembers(&out, []cluster.Member{m}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "Total: 1 alive;") {
		t.Fatal(out.String())
	}
	raw, _ := json.Marshal(m)
	out.Write(raw)
	for _, bad := range []string{"planted-session", "/private/path", "repo-private", "op://private/reference", "raw-error"} {
		if strings.Contains(out.String(), bad) {
			t.Fatal("leak", bad)
		}
	}
}

func TestClusterFactsCommandForms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix endpoint fixture")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir, err := os.MkdirTemp("/tmp", "hev-c-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv(cluster.EndpointEnv, filepath.Join(dir, "cluster", "agent.sock"))
	config := filepath.Join(dir, "config.toml")
	if e := os.WriteFile(config, []byte("[cluster]\nbind=\"127.0.0.1:0\"\nkey_ref=\"secret://fixture\"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HEV_CONFIG", config)
	cfg := &cluster.Config{Bind: "127.0.0.1:0", Name: "alpha", KeyRef: "secret://fixture"}
	svc := cluster.NewService(ctx, cfg, func(context.Context, string) ([]byte, time.Time, error) {
		return []byte("0123456789abcdef0123456789abcdef"), time.Time{}, nil
	}, cluster.LocalFacts("alpha", false, 0), nil)
	defer svc.Close()
	clusterWait(t, "facts service ready", func() bool { return svc.Status().CountersReady })
	c, e := svc.Counters()
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	f := cluster.NodeFacts{Version: 1, At: now, Sessions: &cluster.SessionFacts{At: now.Add(-4 * time.Minute), Harness: map[string]cluster.SessionCount{"codex": {Working: 1}}}, Credentials: &cluster.CredentialFacts{At: now, Items: map[string]cluster.CredentialFact{"codex": {State: "unknown"}}}}
	if e := c.SetNodeFacts(f); e != nil {
		t.Fatal(e)
	}
	stop, e := cluster.Serve(ctx, svc)
	if e != nil {
		t.Fatal(e)
	}
	defer stop()
	for _, args := range [][]string{{"ls"}, {"ls", "--json"}, {"facts"}, {"facts", "alpha"}, {"facts", "--json"}, {"facts", "alpha", "--json"}, {"status"}, {"status", "--json"}} {
		cmd := newClusterCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if e := cmd.Execute(); e != nil {
			t.Fatal(args, e, out.String())
		}
		if !strings.Contains(strings.ToLower(out.String()), "stale") {
			t.Fatal(args, "missing stale marking", out.String())
		}
		for _, bad := range []string{"planted-session", "/private/path", "repo-private", "op://private/reference", "raw-error"} {
			if strings.Contains(out.String(), bad) {
				t.Fatal(args, "leak", bad)
			}
		}
		if strings.Join(args, " ") == "ls --json" {
			var members []map[string]any
			if e := json.Unmarshal(out.Bytes(), &members); e != nil {
				t.Fatal(e)
			}
			for _, key := range []string{"name", "address", "state", "changed", "since", "version", "channel", "facts", "ages", "stale"} {
				if _, ok := members[0][key]; !ok {
					t.Fatal("lost JSON field", key)
				}
			}
		}
	}
}

func TestClusterRateLimitedAndRejectedStatus(t *testing.T) {
	now := time.Now().UTC()
	f := cluster.NodeFacts{At: now, Credentials: &cluster.CredentialFacts{At: now, Items: map[string]cluster.CredentialFact{"op": {At: now, State: "rate-limited"}}}}
	_, _, _, health, _, _ := clusterColumns(cluster.Member{NodeFacts: &f}, now)
	if !strings.Contains(health, "rate-limited") {
		t.Fatal(health)
	}
	var out bytes.Buffer
	printClusterStatus(&out, &cluster.Status{FactsRejected: 2})
	if !strings.Contains(out.String(), "Facts rejected: 2") {
		t.Fatal(out.String())
	}
}
