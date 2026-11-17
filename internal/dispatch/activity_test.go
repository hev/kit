package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// LocalActivity must agree with hev dispatch ls (Handle list) on the same
// fixture state; live Expediters count as orchestrators, as hev expedite
// status lists them.
func TestLocalActivityMatchesDispatchLsAndExpediteStatus(t *testing.T) {
	orchHome(t)
	if err := os.MkdirAll(Root(), 0700); err != nil {
		t.Fatal(err)
	}
	run := func(name string, live bool, m Meta, r *Result) {
		t.Helper()
		dir := filepath.Join(Root(), name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		m.Name = name
		if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
			t.Fatal(err)
		}
		if r != nil {
			if err := WriteJSON(filepath.Join(dir, "result.json"), r); err != nil {
				t.Fatal(err)
			}
		}
		if live {
			if err := writeProcessRecord(filepath.Join(dir, "supervisor.json"), os.Getpid()); err != nil {
				t.Fatal(err)
			}
		}
	}
	start := time.Now().UTC().Add(-time.Hour)
	run("busy-one", true, Meta{Start: start}, &Result{Start: start})
	run("busy-two", true, Meta{Start: start}, &Result{Start: start})
	run("finished", false, Meta{Start: start}, &Result{Start: start, End: start.Add(time.Minute)})
	run("crashed", false, Meta{Start: start}, &Result{Start: start})
	run("imported", true, Meta{Start: start, Imported: true}, &Result{Start: start})

	expediter := func(name string, until bool, control string) {
		t.Helper()
		text := strings.Replace(orchestratorExample, "name: release-train", "name: "+name, 1)
		s, err := ApplyOrchestrator(text, false)
		if err != nil {
			t.Fatal(err)
		}
		if until {
			legacy := "---\nname: " + name + "\nhost: test-host\nevery: 7m\nmax_lifetime: 2h\nuntil: all followed runs merged or abandoned\n---\nFollow runs.\n"
			if err = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "order.md"), []byte(legacy), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if control != "" {
			if err = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), control), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	expediter("expedite-live", true, "")
	expediter("expedite-stopped", true, "stop")
	expediter("expedite-paused", true, "pause")
	expediter("plain-orchestrator", false, "")
	// Live supervisors: one Expediter and the plain orchestrator. The stopped
	// and paused Expediters have none, so they are not live orchestrators.
	for _, name := range []string{"expedite-live", "plain-orchestrator"} {
		if err := writeProcessRecord(filepath.Join(OrchestratorDir(name), "supervisor.json"), os.Getpid()); err != nil {
			t.Fatal(err)
		}
	}

	got, err := LocalActivity()
	if err != nil {
		t.Fatal(err)
	}
	ls, err := Handle(Request{Action: "list"})
	if err != nil {
		t.Fatal(err)
	}
	var runs, expediteStatusRows int
	for _, row := range ls.Rows {
		if HeadlessRunRow(row) {
			runs++
		}
		// hev expedite status lists every row with this Until.
		if row.Until == "all followed runs merged or abandoned" {
			expediteStatusRows++
		}
	}
	if got.HeadlessRuns != 2 || runs != 2 {
		t.Fatalf("headless runs: LocalActivity=%d ls=%d, want 2; rows=%+v", got.HeadlessRuns, runs, ls.Rows)
	}
	if ls.Running != 3 {
		t.Fatalf("ls running %d: the imported run is live but hidden from rows", ls.Running)
	}
	if expediteStatusRows != 3 {
		t.Fatalf("expedite status rows %d, want 3", expediteStatusRows)
	}
	// orchestrators matches the count dispatch ls and hosts report.
	if got.Orchestrators != 2 || ls.Orchestrators != 2 {
		t.Fatalf("orchestrators: LocalActivity=%d ls=%d, want 2", got.Orchestrators, ls.Orchestrators)
	}
}

func TestLocalActivityWithoutDispatchState(t *testing.T) {
	orchHome(t)
	got, err := LocalActivity()
	if err != nil || got != (Activity{}) {
		t.Fatalf("%+v %v", got, err)
	}
}
