package dispatch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// slotsTestHome is a receiver with cap and a tmux that reports no sessions,
// so only process records under Root() decide what is live.
func slotsTestHome(t *testing.T, cap string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if runtime.GOOS != "windows" {
		bin := t.TempDir()
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := filepath.Join(home, "target.toml")
	if err := os.WriteFile(cfg, []byte("cap="+cap+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	if err := os.MkdirAll(Root(), 0700); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// slotDir makes a directory under Root(); a run gets meta.json, and a live
// one a process record held by this test process.
func slotDir(t *testing.T, name string, run bool, record string, result *Result) {
	t.Helper()
	dir := filepath.Join(Root(), name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if run {
		if err := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: name, Host: "local"}, Start: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	if record != "" {
		if err := writeProcessRecord(filepath.Join(dir, record), os.Getpid()); err != nil {
			t.Fatal(err)
		}
	}
	if result != nil {
		if err := WriteJSON(filepath.Join(dir, "result.json"), result); err != nil {
			t.Fatal(err)
		}
	}
}

// The mini showed CAP 5 RUNNING 5 FREE 0 with two workers and three idle
// orchestrator supervisors. Only the workers hold slots; the orchestrators
// are reported on their own.
func TestOrchestratorSupervisorsTakeNoWorkerSlot(t *testing.T) {
	slotsTestHome(t, "5")
	end := time.Now().UTC()
	slotDir(t, "kit-site-07-docs", true, "supervisor.json", nil)
	slotDir(t, "story-refresh-finish", true, "admission.json", nil)
	// A run may itself be named orch-*: its meta.json makes it a worker.
	slotDir(t, "orch-named-run", true, "supervisor.json", nil)
	slotDir(t, "orch-kit-07-train2", false, "supervisor.json", nil)
	slotDir(t, "orch-story-refresh-finish", false, "supervisor.json", nil)
	slotDir(t, "orch-shell-trace-review", false, "supervisor.json", nil)
	// A stopped orchestrator has no live supervisor and is not counted.
	slotDir(t, "orch-stopped", false, "", nil)
	// Blocked and needs-review runs have no live process: no slot.
	slotDir(t, "blocked-run", true, "", &Result{End: end, Outcome: OutcomeBlocked, Reason: "waiting"})
	slotDir(t, "review-run", true, "", &Result{End: end, NeedsReview: true})

	live := map[string]bool{}
	if err := addReservations(live); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"orch-kit-07-train2", "orch-story-refresh-finish", "orch-shell-trace-review", "blocked-run", "review-run"} {
		if live[name] {
			t.Errorf("%s holds a worker slot", name)
		}
	}
	if len(live) != 3 {
		t.Fatalf("live = %v, want the three runs", live)
	}
	r, err := Handle(Request{Action: "list"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Cap != 5 || r.Running != 3 || r.Free != 2 || r.Orchestrators != 3 {
		t.Fatalf("list: cap=%d running=%d free=%d orchestrators=%d", r.Cap, r.Running, r.Free, r.Orchestrators)
	}
	inv := Inventory(Config{Hosts: map[string]Host{"target": {Local: true, Cap: 5}}}, "")
	if h := inv["target"]; h.Error != "" || h.Running != 3 || h.Free != 2 || h.Orchestrators != 3 {
		t.Fatalf("inventory: %+v", h)
	}
	if n, err := LiveOrchestrators(); err != nil || n != 3 {
		t.Fatalf("LiveOrchestrators = %d, %v", n, err)
	}
}

// Admission refuses only on worker slots: a host whose orchestrators alone
// would have filled it still admits a start.
func TestOrchestratorsDoNotFillAdmission(t *testing.T) {
	slotsTestHome(t, "1")
	slotDir(t, "orch-idle", false, "supervisor.json", nil)
	_, err := Handle(Request{Action: "start", Name: "second", Order: "invalid"})
	if err == nil || strings.Contains(err.Error(), "host full") {
		t.Fatalf("orchestrator filled the host: %v", err)
	}
	slotDir(t, "worker", true, "supervisor.json", nil)
	_, err = Handle(Request{Action: "start", Name: "third", Order: good})
	if err == nil || !strings.Contains(err.Error(), "invalid queued order") {
		t.Fatalf("worker slot not counted: %v", err)
	}
}

// A completion notification that failed is shown in ls, not only result.json,
// and keeps a finished run visible by default.
func TestNotifyFailureSurfacesInList(t *testing.T) {
	slotsTestHome(t, "2")
	slotDir(t, "mini-netwatch", true, "", &Result{End: time.Now().UTC(), NotifyError: "ssh: Could not resolve hostname adams-macbook-air"})
	slotDir(t, "quiet", true, "", &Result{End: time.Now().UTC()})
	r, err := Handle(Request{Action: "list"})
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, row := range r.Rows {
		switch row.Name {
		case "mini-netwatch":
			seen++
			if row.State != "done" || !strings.Contains(row.NotifyError, "adams-macbook-air") || !VisibleByDefault(row) {
				t.Fatalf("notify failure hidden: %+v", row)
			}
		case "quiet":
			seen++
			if row.NotifyError != "" || VisibleByDefault(row) {
				t.Fatalf("clean run changed: %+v", row)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("rows: %+v", r.Rows)
	}
}
