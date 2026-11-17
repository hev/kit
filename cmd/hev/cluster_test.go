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

func clusterWait(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if check() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("deadline: %s", what)
		case <-tick.C:
		}
	}
}
func TestClusterCLIThreeMembers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix endpoint fixture; pipe compile checked separately")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key := []byte("0123456789abcdef0123456789abcdef")
	if len(key) != 32 {
		t.Fatal("bad fixture")
	}
	dir := t.TempDir()
	endpoint := filepath.Join(dir, "cluster", "agent.sock")
	t.Setenv(cluster.EndpointEnv, endpoint)
	cfg := &cluster.Config{Bind: "127.0.0.1:0", Name: "alpha", KeyRef: "secret://fixture"}
	loader := func(context.Context, string) ([]byte, time.Time, error) { return key, time.Time{}, nil }
	a := cluster.NewService(ctx, cfg, loader, cluster.LocalFacts("alpha", true, 2), nil)
	defer a.Close()
	clusterWait(t, "alpha running", func() bool { return a.Status().Running })
	seeds := []string{a.Status().Bound}
	b := cluster.NewService(ctx, &cluster.Config{Bind: "127.0.0.1:0", Name: "bravo", KeyRef: "secret://fixture", Seeds: seeds}, loader, cluster.LocalFacts("bravo", true, 3), nil)
	defer b.Close()
	c := cluster.NewService(ctx, &cluster.Config{Bind: "127.0.0.1:0", Name: "charlie", KeyRef: "secret://fixture", Seeds: seeds}, loader, cluster.LocalFacts("charlie", false, 0), nil)
	defer c.Close()
	clusterWait(t, "all three alive", func() bool {
		return a.Status().Counts["alive"] == 3 && b.Status().Counts["alive"] == 3 && c.Status().Counts["alive"] == 3 && a.Status().CountersReady && b.Status().CountersReady && c.Status().CountersReady
	})
	now := time.Now().UTC().Truncate(time.Second)
	for i, svc := range []*cluster.Service{a, b, c} {
		facts, e := svc.Counters()
		if e != nil {
			t.Fatal(e)
		}
		if e = facts.SetOwn("requests", cluster.MinuteWindow(now), uint64(i+1)); e != nil {
			t.Fatal(e)
		}
		if e = facts.SetMax("1password", now.Add(time.Duration(i+1)*time.Hour)); e != nil {
			t.Fatal(e)
		}
		if e = facts.SetMeter("1password", cluster.Meter{Label: "account", Limit: 1000, Used: uint64(i + 1), Remaining: 997, Taken: now}); e != nil {
			t.Fatal(e)
		}
	}
	clusterWait(t, "status facts converged", func() bool {
		for _, svc := range []*cluster.Service{a, b, c} {
			facts, e := svc.Counters()
			if e != nil || facts.RollingSum("requests", time.Hour) != 6 || facts.Meter("1password").Node != "charlie" || !facts.Max("1password").Equal(now.Add(3*time.Hour)) {
				return false
			}
		}
		return true
	})
	at := time.Now().UTC().Truncate(time.Second)
	n := func(v int) *int { return &v }
	if e := a.SetActivity(cluster.Activity{AgentsWorking: n(2), AgentsIdle: n(1), HeadlessRuns: n(3), Expediters: n(1), At: at}); e != nil {
		t.Fatal(e)
	}
	// bravo stands in for a Windows node: no agent counts.
	if e := b.SetActivity(cluster.Activity{HeadlessRuns: n(0), Expediters: n(0), At: at}); e != nil {
		t.Fatal(e)
	}
	// charlie stands in for an older build: it never publishes activity.
	clusterWait(t, "bravo activity reached alpha", func() bool {
		for _, m := range a.Members() {
			if m.Name == "bravo" && m.Activity != nil {
				return true
			}
		}
		return false
	})
	stop, e := cluster.Serve(ctx, a)
	if e != nil {
		t.Fatal(e)
	}
	defer stop()
	config := filepath.Join(dir, "config.toml")
	if e = os.WriteFile(config, []byte("[cluster]\nbind = \"127.0.0.1:0\"\nkey_ref = \"secret://fixture\"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HEV_CONFIG", config)
	run := func(args ...string) string {
		t.Helper()
		cmd := newClusterCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if e := cmd.Execute(); e != nil {
			t.Fatalf("%v: %v (%s)", args, e, out.String())
		}
		if strings.Contains(out.String(), string(key)) {
			t.Fatal("CLI leaked key")
		}
		return out.String()
	}
	ls := run("ls")
	status := run("status")
	t.Logf("real CLI ls:\n%s\nreal CLI status:\n%s", ls, status)
	if !strings.Contains(ls, "charlie") {
		t.Fatal("ls missing member")
	}
	var members []cluster.Member
	lsJSON := run("ls", "--json")
	if json.Unmarshal([]byte(lsJSON), &members) != nil || len(members) != 3 {
		t.Fatal("bad JSON ls")
	}
	selves := 0
	for _, m := range members {
		if m.Self {
			selves++
			if m.Name != "alpha" {
				t.Fatalf("self is %s", m.Name)
			}
		}
		switch m.Name {
		case "alpha":
			if m.Activity == nil || m.Activity.Short() != "2w/1i 3r 1x" || !m.Activity.At.Equal(at) {
				t.Fatalf("alpha activity %+v", m.Activity)
			}
		case "bravo":
			if m.Activity == nil || m.Activity.AgentsWorking != nil || m.Activity.Short() != "0r 0x" {
				t.Fatalf("bravo activity %+v", m.Activity)
			}
		case "charlie":
			if m.Activity != nil {
				t.Fatalf("charlie activity %+v", m.Activity)
			}
		}
	}
	if selves != 1 || strings.Count(lsJSON, `"self":true`) != 1 {
		t.Fatalf("self marked %d times: %s", selves, lsJSON)
	}
	if !strings.Contains(ls, "ACTIVITY") || !strings.Contains(ls, "alpha*") || !strings.Contains(ls, "2w/1i 3r 1x") {
		t.Fatalf("human ls:\n%s", ls)
	}
	var st cluster.Status
	if json.Unmarshal([]byte(run("status", "--json")), &st) != nil || !st.KeyLoaded || st.Counts["alive"] != 3 || st.Self != "alpha" {
		t.Fatal("bad JSON status")
	}
	if !strings.Contains(status, "Self: alpha") {
		t.Fatalf("human status:\n%s", status)
	}
	run("leave")
	clusterWait(t, "left visible", func() bool { return b.Status().Counts["left"] == 1 })
	run("join", b.Status().Bound)
	clusterWait(t, "CLI join restored", func() bool { return b.Status().Counts["alive"] == 3 })
}
func TestClusterCLIDisabledHint(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing.toml")
	t.Setenv("HEV_CONFIG", p)
	t.Setenv(cluster.EndpointEnv, "invalid-must-not-contact")
	for _, args := range [][]string{{"ls"}, {"ls", "--json"}, {"status"}, {"status", "--json"}, {"join", "192.0.2.1:7946"}, {"leave"}} {
		cmd := newClusterCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if e := cmd.Execute(); e == nil {
			t.Fatal("disabled success")
		}
		if strings.TrimSpace(out.String()) != clusterHint {
			t.Fatalf("unexpected disabled output %q", out.String())
		}
	}
}
