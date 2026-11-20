package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An orchestrator stops a managed run only when its spec grants stop; the
// stop is recorded on the run like the orchestrator's other verbs.
func TestOrchestratorStopAuthority(t *testing.T) {
	orchHome(t)
	applyOrch(t)
	run := func(name string) string {
		t.Helper()
		dir := filepath.Join(Root(), name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		m := Meta{Order: Order{Name: name, Repo: "app", Issue: "REL-1"}, TraceID: "0123456789abcdef0123456789abcdef"}
		if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	// The example spec grants say, rework, retry, validate and dispatch.
	without := run("held")
	_, err := Handle(Request{Action: "stop", Name: "held", Orchestrator: "release-train"})
	if err == nil || !strings.Contains(err.Error(), "lacks authority stop") {
		t.Fatalf("stop without authority: %v", err)
	}
	if exists(filepath.Join(without, "stop")) {
		t.Fatal("a refused stop wrote the stop file")
	}

	text := strings.Replace(orchestratorExample, "authority: [say, rework, retry, validate, dispatch]", "authority: [say, stop]", 1)
	if _, err := ApplyOrchestrator(text, false); err != nil {
		t.Fatal(err)
	}
	with := run("stuck")
	r, err := Handle(Request{Action: "stop", Name: "stuck", Orchestrator: "release-train", Note: "worker done; poller hung"})
	if err != nil || r.Message != "stopping stuck" {
		t.Fatalf("stop with authority: %+v %v", r, err)
	}
	if !exists(filepath.Join(with, "stop")) {
		t.Fatal("stop file not written")
	}
	var m Meta
	if err := readJSON(filepath.Join(with, "meta.json"), &m); err != nil {
		t.Fatal(err)
	}
	last := m.Asks[len(m.Asks)-1]
	if last.Action != "stop" || last.Text != "stopped by orchestrator release-train: worker done; poller hung" || last.Caller.Sender != "orchestrator:release-train" {
		t.Fatalf("stop ask %+v", last)
	}
	log, _ := os.ReadFile(filepath.Join(with, "log"))
	if !strings.Contains(string(log), `"type":"dispatch.stop"`) || !strings.Contains(string(log), `"by":"orchestrator release-train"`) {
		t.Fatalf("run log %s", log)
	}

	// Scope still applies: stop authority does not reach an unmanaged run.
	other := run("elsewhere")
	m = Meta{Order: Order{Name: "elsewhere", Repo: "app", Issue: "OTHER-1"}}
	_ = WriteJSON(filepath.Join(other, "meta.json"), m)
	if _, err := Handle(Request{Action: "stop", Name: "elsewhere", Orchestrator: "release-train"}); err == nil || !strings.Contains(err.Error(), "outside manages scope") {
		t.Fatalf("stop outside scope: %v", err)
	}
}
