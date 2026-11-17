package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpediteIdleStallAndSingleNotice(t *testing.T) {
	orchHome(t)
	s, e := ApplyOrchestrator(strings.Replace(orchestratorExample, "release-train", "expedite-stall", 1), false)
	if e != nil {
		t.Fatal(e)
	}
	st, _ := orchestratorState(s.Name)
	head := "first"
	sessions := 0
	notices := 0
	d := OrchestratorDriver{
		Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
			return []OrchestratorEvidence{{Host: "worker", Row: Row{Head: head, State: "done"}}}, nil
		},
		Session: func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
			sessions++
			return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("## Named waits and release conditions\n- CI: checks must pass\n[needs you] resolve CI failure\n"), 0600)
		},
		Notify: func(_ context.Context, n OrchestratorNotice) error {
			if strings.Contains(n.Message, "stalled:") {
				notices++
			}
			return nil
		},
	}
	for i := 0; i < 7; i++ {
		if e = RunOrchestratorRound(context.Background(), s, &st, d); e != nil {
			t.Fatal(e)
		}
	}
	if sessions != 1 || st.Rounds != 1 || st.State != "stalled" || notices != 1 || st.StallReason != "[needs you] resolve CI failure" {
		t.Fatal(sessions, st, notices)
	}
	head = "second"
	if e = RunOrchestratorRound(context.Background(), s, &st, d); e != nil {
		t.Fatal(e)
	}
	if sessions != 2 || st.Stalls != 0 || st.State != "waiting" {
		t.Fatal(sessions, st)
	}
	if e = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "inbox.md"), []byte("owner message"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = RunOrchestratorRound(context.Background(), s, &st, d); e != nil {
		t.Fatal(e)
	}
	if sessions != 3 {
		t.Fatal("inbox did not wake round", sessions)
	}
}

func TestScopeRegistrationRPCNeverLaunchesSupervisor(t *testing.T) {
	orchHome(t)
	owner := Host{SSH: "owner"}
	r, e := Handle(Request{Action: "orchestrator-apply", AdmissionOnly: true, ScopeOwner: &owner, Order: orchestratorExample})
	if e != nil || !r.ScopeRegistered {
		t.Fatal(r, e)
	}
	dir := OrchestratorDir("release-train")
	if !exists(filepath.Join(dir, "scope-owner.json")) || exists(filepath.Join(dir, "supervisor.json")) {
		t.Fatal("scope registration launched a supervisor")
	}
	if _, e = Handle(Request{Action: "orchestrator-scope-remove", Name: "release-train"}); e != nil {
		t.Fatal(e)
	}
	if _, e = Handle(Request{Action: "orchestrator-scope-remove", Name: "release-train"}); e != nil {
		t.Fatal("repeated removal:", e)
	}
	if _, e = LoadOrchestrator("release-train"); e == nil {
		t.Fatal("scope persisted after stop cleanup")
	}
	s := applyOrch(t)
	if _, e = Handle(Request{Action: "orchestrator-scope-remove", Name: s.Name}); e == nil {
		t.Fatal("deleted a durable owner")
	}
}

func TestStallReasonRequiresExplicitRequest(t *testing.T) {
	for _, tc := range []struct{ checkpoint, want string }{
		{"No duplicate [needs you] notice.\n- [needs you] Choose disposition.\n[needs you] Later request.", "[needs you] Choose disposition."},
		{"No duplicate [needs you] notice.", "[needs you] followed runs and named waits are unchanged"},
		{"  [needs you] Resolve the blocked check.  ", "[needs you] Resolve the blocked check."},
	} {
		if got := firstNeedsYou(tc.checkpoint); got != tc.want {
			t.Errorf("firstNeedsYou(%q) = %q; want %q", tc.checkpoint, got, tc.want)
		}
	}
}
