//go:build !windows

package dispatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestSwarmEndToEnd runs a whole swarm through real dispatch admission on a
// temporary root: real git worktrees and branches against a local origin, a
// fake harness in the test's private tmux server, and the reconcile tick
// with the swarm's own identity. No model is called.
func TestSwarmEndToEnd(t *testing.T) {
	swarmHome(t)
	home := os.Getenv("HOME")
	git := func(dir string, args ...string) string {
		t.Helper()
		out, e := command(dir, append([]string{"git", "-c", "user.name=Swarm Test", "-c", "user.email=swarm@example.com"}, args...)...)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	origin := filepath.Join(home, "origin", "app.git")
	repo := filepath.Join(home, "workspace", "app")
	for _, d := range []string{origin, repo} {
		if e := os.MkdirAll(d, 0700); e != nil {
			t.Fatal(e)
		}
	}
	git(origin, "init", "--bare", "-b", "main")
	git(repo, "init", "-b", "main")
	git(repo, "commit", "--allow-empty", "-m", "initial")
	git(repo, "remote", "add", "origin", origin)
	git(repo, "push", "-u", "origin", "main")

	// The fake harness is a tmux session that runs until its run is stopped
	// or the test finishes it.
	oldLauncher := runLauncher
	t.Cleanup(func() { runLauncher = oldLauncher })
	launched := map[string]int{}
	runLauncher = func(dir, name string) error {
		launched[name]++
		_ = os.Remove(filepath.Join(dir, "finish"))
		script := fmt.Sprintf("while [ ! -e %s ] && [ ! -e %s ]; do sleep 0.05; done", Quote(filepath.Join(dir, "stop")), Quote(filepath.Join(dir, "finish")))
		return tmuxCommand("new-session", "-d", "-s", "d-"+name, "-c", filepath.Join(dir, "wt"), script).Run()
	}
	t.Cleanup(func() { _ = tmuxCommand("kill-server").Run() })
	gone := func(name string) {
		t.Helper()
		for i := 0; i < 100; i++ {
			live, e := liveNames()
			if e != nil {
				t.Fatal(e)
			}
			if !live[name] {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("%s still running", name)
	}
	// finish commits and pushes the unit's work, records done and exits.
	finish := func(name string) {
		t.Helper()
		dir := filepath.Join(Root(), name)
		wt := filepath.Join(dir, "wt")
		if e := os.WriteFile(filepath.Join(wt, name+".txt"), []byte(name+"\n"), 0600); e != nil {
			t.Fatal(e)
		}
		git(wt, "add", ".")
		git(wt, "commit", "-m", name)
		git(wt, "push", "origin", "d/"+name)
		head := git(wt, "rev-parse", "HEAD")
		if e := WriteJSON(filepath.Join(dir, "result.json"), Result{ExitCode: 0, Start: time.Now().UTC(), End: time.Now().UTC(), Branch: "d/" + name, Head: head}); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(dir, "finish"), nil, 0600); e != nil {
			t.Fatal(e)
		}
		gone(name)
	}
	// kill crashes the worker: its supervisor records failed, exit 1.
	kill := func(name string) {
		t.Helper()
		dir := filepath.Join(Root(), name)
		if e := WriteJSON(filepath.Join(dir, "result.json"), Result{ExitCode: 1, Start: time.Now().UTC(), End: time.Now().UTC(), Branch: "d/" + name, Error: "harness exited 1"}); e != nil {
			t.Fatal(e)
		}
		if e := tmuxCommand("kill-session", "-t", "=d-"+name).Run(); e != nil {
			t.Fatal(e)
		}
		gone(name)
	}

	text := swarmPhase1Example(t)
	file := filepath.Join(t.TempDir(), "logfix.md")
	if e := os.WriteFile(file, []byte(text), 0600); e != nil {
		t.Fatal(e)
	}
	var tr strings.Builder
	say := func(format string, args ...any) { fmt.Fprintf(&tr, format+"\n", args...) }
	ps := func() SwarmView {
		t.Helper()
		v, e := SwarmPS("logfix")
		if e != nil {
			t.Fatal(e)
		}
		say("$ hev swarm ps logfix")
		WriteSwarmPS(&tr, v)
		return v
	}
	now := time.Now().UTC()
	begin := now
	ops := newLocalSwarmOps("logfix") // the swarm's own identity
	reconcile := func(note string) {
		t.Helper()
		say("# reconcile tick at +%s: %s", now.Sub(begin).Round(time.Second), note)
		if e := SwarmTick(context.Background(), "logfix", ops, now); e != nil {
			t.Fatal(e)
		}
	}
	running := func() []string {
		live, e := liveNames()
		if e != nil {
			t.Fatal(e)
		}
		var out []string
		for n := range live {
			if strings.HasPrefix(n, "logfix-") {
				out = append(out, n)
			}
		}
		slices.Sort(out)
		return out
	}

	say("$ hev swarm up -f logfix.md")
	plan, e := SwarmUp(text, false)
	if e != nil {
		t.Fatal(e)
	}
	WriteSwarmPlan(&tr, plan, false)
	ps()
	reconcile("launch up to replicas")
	if r := running(); len(r) != 3 {
		t.Fatalf("running %v", r)
	}
	reconcile("observe")
	ps()

	finish("logfix-migrate-api-g1")
	say("# api finishes: commit, push, exit 0")
	kill("logfix-migrate-cli-g1")
	say("# the cli worker is killed: exit 1")
	reconcile("api done, cli stopped and confirmed, storage launched")
	// cli keeps its replica slot while its repair waits out the backoff.
	if r := running(); !slices.Equal(r, []string{"logfix-migrate-storage-g1", "logfix-migrate-worker-g1"}) {
		t.Fatalf("running %v", r)
	}
	v := ps()
	cli := v.Units[slices.IndexFunc(v.Units, func(u SwarmUnit) bool { return u.Unit == "cli" })]
	if cli.State != "failed" || !cli.NextTry.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("cli %+v", cli)
	}
	now = now.Add(time.Minute)
	reconcile("cli backoff not elapsed")
	if launched["logfix-migrate-cli-g1"] != 1 {
		t.Fatal("retried before backoff")
	}
	now = now.Add(time.Minute)
	reconcile("backoff elapsed: retry cli in place")
	if launched["logfix-migrate-cli-g1"] != 2 {
		t.Fatalf("cli launches %d", launched["logfix-migrate-cli-g1"])
	}
	// The retry kept the worktree and the branch: same generation, same run.
	order, _ := os.ReadFile(filepath.Join(Root(), "logfix-migrate-cli-g1", "order.md"))
	if !strings.Contains(string(order), "## Rework") || !strings.Contains(string(order), "Swarm repair in place") {
		t.Fatalf("retry note missing:\n%s", order)
	}
	ps()

	for _, n := range []string{"logfix-migrate-worker-g1", "logfix-migrate-storage-g1", "logfix-migrate-cli-g1"} {
		finish(n)
		say("# %s finishes", n)
	}
	now = now.Add(2 * time.Minute)
	reconcile("three pieces done; web launched")
	if slices.Contains(running(), "logfix-integrate-g1") {
		t.Fatal("integrate started before the partition finished")
	}
	ps()
	finish("logfix-migrate-web-g1")
	say("# logfix-migrate-web-g1 finishes")
	now = now.Add(2 * time.Minute)
	reconcile("partition complete: integrate starts")
	if r := running(); !slices.Equal(r, []string{"logfix-integrate-g1"}) {
		t.Fatalf("running %v", r)
	}
	order, _ = os.ReadFile(filepath.Join(Root(), "logfix-integrate-g1", "order.md"))
	for _, piece := range []string{"api", "worker", "cli", "storage", "web"} {
		if !strings.Contains(string(order), "d/logfix-migrate-"+piece+"-g1") {
			t.Fatalf("integrate order lacks %s:\n%s", piece, order)
		}
	}
	now = now.Add(2 * time.Minute)
	reconcile("observe")
	ps()

	say("$ hev swarm down logfix")
	rep, e := SwarmDown(context.Background(), "logfix", nil)
	if e != nil {
		t.Fatal(e)
	}
	WriteSwarmDown(&tr, rep)
	if len(running()) != 0 || !slices.Equal(rep.Stopped, []string{"logfix-integrate-g1"}) || len(rep.Branches) != 6 {
		t.Fatalf("down %+v, running %v", rep, running())
	}
	v = ps()
	for _, u := range v.Units {
		want := "done"
		if u.Role == "integrate" {
			want = "held"
		}
		if u.State != want || u.Gen != 1 {
			t.Fatalf("after down %+v", u)
		}
	}
	if total := len(launched); total != 6 {
		t.Fatalf("runs %v", launched)
	}
	t.Log("\n" + tr.String())
	if p := os.Getenv("SWARM_E2E_TRANSCRIPT"); p != "" {
		_ = os.WriteFile(p, []byte(tr.String()), 0600)
	}
}
