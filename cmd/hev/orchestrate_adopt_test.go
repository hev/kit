package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
)

func adoptHome(t *testing.T, config string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_ORCHESTRATOR", "")
	t.Setenv("PEERS_SELF", "build-1")
	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	if err := os.WriteFile(cfg, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
}

// adoptSpecText uses the pull route so no test sends a real peers notice.
func adoptSpecText(name, reply string) string {
	return "---\nkind: orchestrator\nname: " + name + "\ngoal: Observe\nreply: " + reply + "\nnotify_route: pull\nmode: loop\nevery: 20m\nmax_lifetime: 1d\nmanages: {runs: {names: [task]}}\n---\nBrief\n"
}

func runOrchestrate(args ...string) (string, string, error) {
	cmd := newOrchestrateCommand()
	var out, errout bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errout)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errout.String(), err
}

// Acceptance: handoff without --yes from a non-TTY refuses and names adopt;
// retire --yes reaches handed-off.
func TestOrchestrateHandoffRefusesRetireEnds(t *testing.T) {
	adoptHome(t, "")
	oldLaunch := orchestrateLaunch
	t.Cleanup(func() { orchestrateLaunch = oldLaunch })
	launched := 0
	orchestrateLaunch = func(string) error { launched++; return nil }
	if _, err := dispatch.ApplyOrchestrator(adoptSpecText("train", "desk/foh-a"), false); err != nil {
		t.Fatal(err)
	}
	handoff := filepath.Join(dispatch.OrchestratorDir("train"), "handoff.json")
	_, errout, err := runOrchestrate("handoff", "train", "--to", "desk/foh-b")
	if err == nil || !strings.Contains(err.Error(), "hev orchestrate adopt train") || !strings.Contains(errout, handoffDeprecation) {
		t.Fatalf("%v\n%s", err, errout)
	}
	if _, _, err = runOrchestrate("retire", "train", "--to", "desk/foh-b"); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatal(err)
	}
	if _, err := os.Stat(handoff); !os.IsNotExist(err) || launched != 0 {
		t.Fatal("refused retire wrote handoff.json or launched")
	}
	out, errout, err := runOrchestrate("retire", "train", "--to", "desk/foh-b", "--yes")
	if err != nil || !strings.Contains(errout, "terminal") || !strings.Contains(out, "retire train") || launched != 1 {
		t.Fatalf("%v %q %q", err, out, errout)
	}
	d := dispatch.OrchestratorDriver{
		Now:    time.Now,
		Notify: func(context.Context, dispatch.OrchestratorNotice) error { return nil },
		Gather: func(context.Context, dispatch.OrchestratorSpec) ([]dispatch.OrchestratorEvidence, error) {
			return nil, nil
		},
		Session: func(context.Context, dispatch.OrchestratorSpec, string, dispatch.OrchestratorPacket) error {
			t.Fatal("retire ran a model round")
			return nil
		},
		Wait: func(context.Context) error { t.Fatal("retire waited"); return nil },
	}
	if err := dispatch.SuperviseOrchestrator(context.Background(), "train", d); err != nil {
		t.Fatal(err)
	}
	rows, _ := dispatch.ListOrchestrators()
	if len(rows) != 1 || rows[0].State != "handed-off" {
		t.Fatalf("%+v", rows)
	}
	// The deprecated alias still works with --yes, on a fresh orchestrator.
	if _, err := dispatch.ApplyOrchestrator(adoptSpecText("other", "desk/foh-a"), false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runOrchestrate("handoff", "other", "--to", "desk/foh-b", "--yes"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dispatch.OrchestratorDir("other"), "handoff.json")); err != nil {
		t.Fatal(err)
	}
}

// fakeRemotes answers list, show and adopt for orchestrators on fake hosts.
func fakeRemotes(t *testing.T, hosts map[string][]string) *[]dispatch.Request {
	t.Helper()
	old := orchestrateRPC
	t.Cleanup(func() { orchestrateRPC = old })
	var calls []dispatch.Request
	orchestrateRPC = func(_ context.Context, h dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
		names, ok := hosts[h.SSH]
		if !ok {
			return dispatch.Response{}, errors.New("ssh: unreachable")
		}
		if q.Action == "list" {
			var r dispatch.Response
			for _, n := range names {
				r.Rows = append(r.Rows, dispatch.Row{Kind: "orch", Meta: dispatch.Meta{Order: dispatch.Order{Name: "o-" + n}}, State: "waiting"})
			}
			return r, nil
		}
		q.Note = h.SSH // Record which host was asked.
		calls = append(calls, q)
		switch q.Action {
		case "show":
			return dispatch.Response{Files: map[string]string{"spec.md": "SPEC-" + h.SSH}, Names: []string{"001"}}, nil
		case "adopt":
			return dispatch.Response{Adopt: &dispatch.AdoptResult{Name: strings.TrimPrefix(q.Name, "o-"), From: "desk/foh-a", To: q.To, Redelivered: 2}}, nil
		}
		return dispatch.Response{}, nil
	}
	return &calls
}

// Acceptance: --host routes adopt and show to the remote host; without
// --host the name is looked up across hosts and an ambiguous one errors.
func TestOrchestrateHostRouting(t *testing.T) {
	adoptHome(t, "[hosts.mini]\nssh = 'mini'\n[hosts.oven]\nssh = 'oven'\n[hosts.laptop]\nssh = 'laptop'\n")
	calls := fakeRemotes(t, map[string][]string{"mini": {"train"}, "oven": {"train", "solo"}})
	oldOwn := ownerOwnAddress
	t.Cleanup(func() { ownerOwnAddress = oldOwn })
	ownerOwnAddress = func(context.Context) (string, error) { return "desk/me", nil }

	out, _, err := runOrchestrate("show", "train", "--host", "mini")
	if err != nil || !strings.Contains(out, "SPEC-mini") || !strings.Contains(out, "Host: mini") || !strings.Contains(out, "Rounds: 001") {
		t.Fatalf("%v %s", err, out)
	}
	out, _, err = runOrchestrate("adopt", "train", "--host", "mini", "--to", "desk/foh-b")
	if err != nil || !strings.Contains(out, "adopted orchestrator train: owner desk/foh-a -> desk/foh-b; 2 undelivered") {
		t.Fatalf("%v %s", err, out)
	}
	for _, args := range [][]string{{"show", "train"}, {"adopt", "train", "--to", "desk/foh-b"}} {
		if _, _, err = runOrchestrate(args...); err == nil || !strings.Contains(err.Error(), "more than one host (mini, oven)") || !strings.Contains(err.Error(), "--host") {
			t.Fatal(args, err)
		}
	}
	// Found on exactly one reachable host; --to defaults to the caller.
	out, _, err = runOrchestrate("adopt", "solo")
	if err != nil || !strings.Contains(out, "-> desk/me") {
		t.Fatalf("%v %s", err, out)
	}
	want := []dispatch.Request{
		{Action: "show", Name: "o-train", Note: "mini"},
		{Action: "adopt", Name: "o-train", To: "desk/foh-b", Note: "mini"},
		{Action: "adopt", Name: "o-solo", To: "desk/me", Note: "oven"},
	}
	if len(*calls) != len(want) {
		t.Fatalf("%+v", *calls)
	}
	for i, w := range want {
		got := (*calls)[i]
		if got.Action != w.Action || got.Name != w.Name || got.To != w.To || got.Note != w.Note {
			t.Fatalf("call %d: %+v want %+v", i, got, w)
		}
	}
	if _, _, err = runOrchestrate("show", "missing"); err == nil || !strings.Contains(err.Error(), "unreachable: laptop") {
		t.Fatal(err)
	}
}

// fakeOwnerPeers: desk answers with session alive; laptop is asleep.
func fakeOwnerPeers(t *testing.T) {
	t.Helper()
	old := ownerPeers
	t.Cleanup(func() { ownerPeers = old })
	ownerPeers = func(_ context.Context, host string) ([]string, error) {
		switch host {
		case "desk":
			return []string{"alive", "d-run"}, nil
		case "laptop":
			return nil, errors.New("ssh: connect to host laptop: Operation timed out")
		}
		return nil, errors.New("unknown host " + host)
	}
}

func orphanFixture() map[string]dispatch.Response {
	row := func(kind, name, reply, state string) dispatch.Row {
		return dispatch.Row{Kind: kind, Meta: dispatch.Meta{Order: dispatch.Order{Name: name, Reply: reply}}, State: state}
	}
	return map[string]dispatch.Response{
		"oven": {Rows: []dispatch.Row{
			row("orch", "o-live", "desk/alive", "waiting"),
			row("orch", "o-asleep", "laptop/foh-1", "waiting"),
			row("orch", "o-gone", "desk/foh-7", "waiting"),
			row("orch", "o-finished", "desk/foh-7", "complete"),
			row("run", "run-gone", "desk/foh-12", "needs-review"),
			row("run", "run-done", "desk/foh-12", "done"),
			row("run", "run-asleep", "laptop/foh-1", "running"),
			row("run", "run-live", "desk/alive", "running"),
		}},
		"mini": {Error: "ssh: unreachable"},
	}
}

// Acceptance: --orphaned lists only gone owners, never asleep or live ones,
// with the exact adopt command.
func TestOrphanedListsOnlyGoneOwners(t *testing.T) {
	adoptHome(t, "[hosts.oven]\nssh = 'oven'\n")
	fakeOwnerPeers(t)
	oldInv, oldDispatchInv := orphanInventory, dispatchInventory
	t.Cleanup(func() { orphanInventory, dispatchInventory = oldInv, oldDispatchInv })
	orphanInventory = orphanFixture
	dispatchInventory = func(dispatch.Config, string) map[string]dispatch.Response { return orphanFixture() }

	out, _, err := runOrchestrate("ls", "--orphaned", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []orphanRow
	if err = json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err, out)
	}
	if len(rows) != 1 || rows[0].Name != "o-gone" || rows[0].OwnerStatus != ownerGone || rows[0].Owner != "desk/foh-7" || rows[0].Adopt != "hev orchestrate adopt gone --host oven" {
		t.Fatalf("%+v", rows)
	}

	cmd := newDispatchCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"ls", "--orphaned"})
	if err = cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	for _, want := range []string{"hev orchestrate adopt gone --host oven", "hev dispatch adopt run-gone --host oven", "owner desk/foh-12 is gone"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q:\n%s", want, text)
		}
	}
	for _, not := range []string{"live", "asleep", "finished", "run-done"} {
		if strings.Contains(text, not) {
			t.Fatalf("listed %q:\n%s", not, text)
		}
	}
}

// orchestrate ls shows each owner and whether it is live, gone or asleep.
func TestOrchestrateLsOwnerColumn(t *testing.T) {
	adoptHome(t, "")
	fakeOwnerPeers(t)
	for name, reply := range map[string]string{"one": "desk/alive", "two": "laptop/foh-1", "three": "desk/foh-7"} {
		if _, err := dispatch.ApplyOrchestrator(adoptSpecText(name, reply), false); err != nil {
			t.Fatal(err)
		}
	}
	out, _, err := runOrchestrate("ls")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"OWNER", "desk/alive (live)", "laptop/foh-1 (asleep)", "desk/foh-7 (gone)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	out, _, err = runOrchestrate("ls", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err = json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r["name"].(string)] = r["owner"].(string) + " " + r["owner_status"].(string)
	}
	if got["one"] != "desk/alive live" || got["two"] != "laptop/foh-1 asleep" || got["three"] != "desk/foh-7 gone" {
		t.Fatal(got)
	}
}

// Review regression: a local spec is one match, not the answer. The same name
// on a reachable remote host is ambiguous, and nothing changes locally until
// a host is chosen. An unreachable remote leaves the local match standing.
func TestOrchestrateLocalRemoteAmbiguity(t *testing.T) {
	adoptHome(t, "[hosts.remote]\nssh = 'fake-remote'\n[hosts.asleep]\nssh = 'asleep'\n")
	calls := fakeRemotes(t, map[string][]string{"fake-remote": {"dup"}})
	for _, name := range []string{"dup", "only-here"} {
		if _, err := dispatch.ApplyOrchestrator(adoptSpecText(name, "desk/foh-a"), false); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err := runOrchestrate("adopt", "dup", "--to", "desk/foh-b")
	if err == nil || !strings.Contains(err.Error(), "more than one host (local, remote)") || !strings.Contains(err.Error(), "--host") {
		t.Fatalf("expected ambiguity refusal; got %v", err)
	}
	if s, err := dispatch.LoadOrchestrator("dup"); err != nil || s.Reply != "desk/foh-a" {
		t.Fatalf("local owner changed despite ambiguity: %+v %v", s, err)
	}
	if len(*calls) != 0 {
		t.Fatalf("remote adopted despite ambiguity: %+v", *calls)
	}
	// Choosing the local host resolves it.
	if _, _, err = runOrchestrate("adopt", "dup", "--host", "local", "--to", "desk/foh-b"); err != nil {
		t.Fatal(err)
	}
	if s, _ := dispatch.LoadOrchestrator("dup"); s.Reply != "desk/foh-b" {
		t.Fatalf("%+v", s)
	}
	// Only local (the other host is unreachable): no ambiguity.
	if _, _, err = runOrchestrate("adopt", "only-here", "--to", "desk/foh-b"); err != nil {
		t.Fatal(err)
	}
	if s, _ := dispatch.LoadOrchestrator("only-here"); s.Reply != "desk/foh-b" {
		t.Fatalf("%+v", s)
	}
}
