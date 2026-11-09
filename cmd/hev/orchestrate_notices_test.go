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

	"github.com/hev/kit/internal/dispatch"
)

const pullSpec = "---\nkind: orchestrator\nname: pulled\ngoal: Observe\nreply: owner\nnotify_route: pull\nmanages: {runs: {names: [task]}}\n---\nBrief"

func pullHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_ORCHESTRATOR", "")
	t.Setenv("PEERS_SELF", "build-1")
	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	if err := os.WriteFile(cfg, []byte("default_host = 'worker'\n[hosts.worker]\nssh = 'worker'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	if _, err := dispatch.ApplyOrchestrator(pullSpec, false); err != nil {
		t.Fatal(err)
	}
}

func dispatchRPC(t *testing.T, q dispatch.Request) dispatch.Response {
	t.Helper()
	b, _ := json.Marshal(q)
	cmd := newDispatchCommand()
	var out bytes.Buffer
	cmd.SetIn(bytes.NewReader(b))
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"rpc"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var r dispatch.Response
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || r.Error != "" {
		t.Fatal(out.String(), err)
	}
	return r
}

// K9: list and ack work over hev dispatch rpc.
func TestDispatchRPCOrchestratorNotices(t *testing.T) {
	pullHome(t)
	r := dispatchRPC(t, dispatch.Request{Action: "orchestrator-notices"})
	if len(r.Notices) != 1 || r.Notices[0].Status != "queued" || !strings.HasPrefix(r.Notices[0].ID, "build-1/o-pulled:") {
		t.Fatal(r.Notices)
	}
	dispatchRPC(t, dispatch.Request{Action: "orchestrator-notice-ack", IDs: []string{r.Notices[0].ID}})
	if r = dispatchRPC(t, dispatch.Request{Action: "orchestrator-notices", Name: "pulled"}); len(r.Notices) != 0 {
		t.Fatal("acked notice still pending", r.Notices)
	}
}

// K9: notices pull --deliver against a fake remote delivers once, even when
// the ack is lost.
func TestNoticesPullDeliverLostAck(t *testing.T) {
	pullHome(t)
	oldRPC, oldDeliver := orchestrateNoticeRPC, deliverPulledNotice
	t.Cleanup(func() { orchestrateNoticeRPC, deliverPulledNotice = oldRPC, oldDeliver })
	loseAck := true
	orchestrateNoticeRPC = func(_ context.Context, h dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
		if h.SSH != "worker" {
			t.Fatal(h)
		}
		if q.Action == "orchestrator-notice-ack" && loseAck {
			loseAck = false
			return dispatch.Response{}, errors.New("ssh: connection reset")
		}
		return dispatch.Handle(q)
	}
	inbox := map[string]int{}
	deliverPulledNotice = func(_ context.Context, n dispatch.OrchestratorNotice) error {
		inbox[n.ID]++ // The peers inbox keeps one row per ID.
		return nil
	}
	pull := func() error {
		cmd := newOrchestrateCommand()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetArgs([]string{"notices", "pull", "--host", "worker", "--deliver"})
		return cmd.Execute()
	}
	if err := pull(); err == nil || !strings.Contains(err.Error(), "ack failed") {
		t.Fatal("lost ack not reported", err)
	}
	if err := pull(); err != nil {
		t.Fatal(err)
	}
	if err := pull(); err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 {
		t.Fatal("inbox rows", inbox)
	}
	for id, deliveries := range inbox {
		if deliveries != 2 || !strings.HasPrefix(id, "build-1/o-pulled:lifecycle:") {
			t.Fatal("want one redelivery of the same ID", inbox)
		}
	}
	if r, _ := dispatch.Handle(dispatch.Request{Action: "orchestrator-notices"}); len(r.Notices) != 0 {
		t.Fatal(r.Notices)
	}
}

// K10: apply validates --seed-checkpoint before writing any state.
func TestOrchestrateApplySeedCheckpointValidation(t *testing.T) {
	pullHome(t)
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	seed := filepath.Join(dir, "seed.md")
	_ = os.WriteFile(spec, []byte(strings.Replace(pullSpec, "name: pulled", "name: seeded", 1)), 0600)
	_ = os.WriteFile(seed, []byte("no sections\n"), 0600)
	run := func(args ...string) (string, error) {
		cmd := newOrchestrateCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String(), err
	}
	if _, err := run("apply", "-f", spec, "--seed-checkpoint", seed); err == nil || !strings.Contains(err.Error(), "missing sections") {
		t.Fatal(err)
	}
	if _, err := dispatch.LoadOrchestrator("seeded"); err == nil {
		t.Fatal("invalid seed applied the spec")
	}
	_ = os.WriteFile(seed, []byte("Goal status: pending\nOpen threads: none\nNext action: wait\n"), 0600)
	if out, err := run("apply", "-f", spec, "--seed-checkpoint", seed, "--dry-run"); err != nil || !strings.Contains(out, "valid seeded") {
		t.Fatal(out, err)
	}
}

// ls and show report the undelivered owner notice backlog and the route.
func TestOrchestrateLsShowNoticeBacklog(t *testing.T) {
	pullHome(t)
	run := func(args ...string) string {
		cmd := newOrchestrateCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	if out := run("ls"); !strings.Contains(out, "NOTICES") || !strings.Contains(out, "1 (oldest 0s)") {
		t.Fatal(out)
	}
	if out := run("show", "pulled"); !strings.Contains(out, "Owner notices: route pull; undelivered 1 (oldest 0s)") {
		t.Fatal(out)
	}
}
