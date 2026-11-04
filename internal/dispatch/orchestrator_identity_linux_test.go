package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrchestratorClearedEnvironmentCannotBypassAdmission(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	if e := writeProcessRecord(filepath.Join(OrchestratorDir(s.Name), "harness.json"), os.Getpid()); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HEV_ORCHESTRATOR", "")
	q := Request{Action: "say", Name: "outside"}
	requestOrchestratorIdentity(&q)
	if q.Orchestrator != s.Name {
		t.Fatalf("lost process identity: %+v", q)
	}
	if _, e := Handle(q); e == nil {
		t.Fatal("out of scope call admitted with environment cleared")
	}
}

func TestOrchestratorNestedIdentityUsesNearestHarness(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	parent := strings.Replace(orchestratorExample, "name: release-train", "name: earlier-parent", 1)
	if _, e := ApplyOrchestrator(parent, false); e != nil {
		t.Fatal(e)
	}
	if e := writeProcessRecord(filepath.Join(OrchestratorDir("earlier-parent"), "harness.json"), os.Getppid()); e != nil {
		t.Fatal(e)
	}
	if e := writeProcessRecord(filepath.Join(OrchestratorDir(s.Name), "harness.json"), os.Getpid()); e != nil {
		t.Fatal(e)
	}
	if n := roundProcessIdentity(); n != s.Name {
		t.Fatalf("nearest harness lost: %q", n)
	}
}
