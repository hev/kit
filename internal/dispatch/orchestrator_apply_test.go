package dispatch

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Apply on a running orchestrator refuses the same way on every platform and
// changes nothing: not state.json, not the spec, not the round in progress.
func TestWindowsApplyRunningOrchestratorKeepsStateAndRound(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	dir := OrchestratorDir(s.Name)
	st, err := orchestratorState(s.Name)
	if err != nil {
		t.Fatal(err)
	}
	st.State, st.Rounds, st.Digest, st.Next = "running", 2, "round-two", time.Now().UTC().Add(time.Hour)
	if err = WriteJSON(filepath.Join(dir, "state.json"), st); err != nil {
		t.Fatal(err)
	}
	round := filepath.Join(dir, "rounds", "002")
	if err = os.MkdirAll(round, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(round, "packet.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	files := []string{filepath.Join(dir, "state.json"), filepath.Join(dir, "spec.md"), filepath.Join(OrchestratorConfigDir(), s.Name+".md"), filepath.Join(round, "packet.json")}
	snapshot := func() [][]byte {
		var out [][]byte
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, b)
		}
		return out
	}
	before := snapshot()
	changed := strings.Replace(orchestratorExample, "goal: Keep the release green", "goal: Keep the release greener", 1)
	// The supervisor is live; then only the round's harness is. The harness
	// is a child: a record naming this process would make it the round.
	harness := exec.Command("sleep", "60")
	if runtime.GOOS == "windows" {
		harness = exec.Command("cmd.exe", "/c", "ping -n 60 127.0.0.1 >NUL")
	}
	startTestChild(t, harness)
	for record, pid := range map[string]int{"supervisor.json": os.Getpid(), "harness.json": harness.Process.Pid} {
		if err = writeProcessRecord(filepath.Join(dir, record), pid); err != nil {
			t.Fatal(err)
		}
		_, err = ApplyOrchestrator(changed, false)
		if err == nil || !strings.Contains(err.Error(), "stop orchestrator before applying changes") {
			t.Fatalf("%s live: apply error %v", record, err)
		}
		for i, b := range snapshot() {
			if !bytes.Equal(b, before[i]) {
				t.Fatalf("%s live: apply changed %s:\n%s", record, files[i], b)
			}
		}
		if err = os.Remove(filepath.Join(dir, record)); err != nil {
			t.Fatal(err)
		}
	}
	// Once nothing is live, a reapply starts a new lifetime and keeps rounds.
	if _, err = ApplyOrchestrator(changed, false); err != nil {
		t.Fatal(err)
	}
	if st, err = orchestratorState(s.Name); err != nil || st.State != "ready" || st.Rounds != 2 || st.Goal != "Keep the release greener" {
		t.Fatalf("reapply after stop: %+v %v", st, err)
	}
	if _, err = os.Stat(round); err != nil {
		t.Fatal(err)
	}
}
