//go:build !windows

package dispatch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Regression coverage adapted from the independent PR #167 review.

func r167Busy(t *testing.T) string {
	t.Helper()
	busy := filepath.Join(Root(), "busy")
	if err := os.MkdirAll(busy, 0700); err != nil {
		t.Fatal(err)
	}
	if err := reserve(busy); err != nil {
		t.Fatal(err)
	}
	return busy
}

func r167Validator() Request {
	o, _ := Parse(good)
	m := Meta{Order: o, Parent: "example", ParentHostName: "src", Revision: "abc"}
	m.Name = "example-v1"
	return Request{Action: "start", Name: "example-v1", Order: good, Meta: m}
}

func TestR167ValidatorOnFullHost(t *testing.T) {
	queueTestHome(t)
	r167Busy(t)
	_, err := Handle(r167Validator())
	t.Logf("validator start on full host, empty queue: %v", err)
	if err == nil || !strings.Contains(err.Error(), "host full") {
		t.Errorf("validator full-host error lost its host-full classification: %v", err)
	}
}

func TestR167ValidatorWithFreeSlotButQueue(t *testing.T) {
	queueTestHome(t)
	cfg := os.Getenv("HEV_DISPATCH_CONFIG")
	if err := os.WriteFile(cfg, []byte("cap=2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	unlock, _ := lockRoot()
	q := queueRequest("waiting")
	q.Cap = 0
	_, err := enqueue(q)
	unlock()
	if err != nil {
		t.Fatal(err)
	}
	original := runLauncher
	defer func() { runLauncher = original }()
	runLauncher = func(dir, name string) error { return writePID(dir) }
	_, err = Handle(r167Validator())
	t.Logf("validator start with 2 free slots and 1 queued order: %v", err)
	if err != nil && strings.Contains(err.Error(), "invalid queued order") {
		t.Errorf("validator refused as queued order despite free capacity: %v", err)
	}
}

func r167Orch(t *testing.T) OrchestratorSpec {
	t.Helper()
	oldSender := orchestratorNoticeSender
	orchestratorNoticeSender = func(context.Context, OrchestratorNotice) error { return nil }
	t.Cleanup(func() { orchestratorNoticeSender = oldSender })
	t.Setenv("PEERS_SELF", "test-host")
	t.Setenv("HEV_ORCHESTRATOR", "")
	return applyOrch(t)
}

func r167OrchRequest(s OrchestratorSpec, name string) Request {
	text := "---\nname: " + name + "\nhost: buildbox\nrepo: app\nissue: REL-1\n---\n## Acceptance\n- done\n"
	o, _ := Parse(text)
	return Request{Action: "start", Name: name, Order: text, Meta: Meta{Order: o}, Cap: 1, Orchestrator: s.Name}
}

func r167Budget(t *testing.T, s OrchestratorSpec) int {
	var day struct {
		Runs int `json:"runs"`
	}
	_ = readJSON(filepath.Join(OrchestratorDir(s.Name), "budget.json"), &day)
	return day.Runs
}

func TestR167OrchestratorBudgetChargedTwice(t *testing.T) {
	queueTestHome(t)
	s := r167Orch(t)
	busy := r167Busy(t)
	q := r167OrchRequest(s, "work1")
	r, err := Handle(q)
	if err != nil || !strings.Contains(r.Message, "queued") {
		t.Fatalf("%+v %v", r, err)
	}
	if n := r167Budget(t, s); n != 0 {
		t.Fatalf("enqueue consumed %d attempts", n)
	}
	_ = os.Remove(filepath.Join(busy, "admission.json"))
	original := runLauncher
	defer func() { runLauncher = original }()
	runLauncher = func(dir, name string) error { return writePID(dir) }
	err = SweepQueue() // fails later at git fetch: repo "app" does not exist; budget is already charged
	t.Logf("sweep: %v", err)
	if n := r167Budget(t, s); n != 1 {
		t.Errorf("one admitted order consumed %d runs_per_day attempts", n)
	}
}

func TestR167PausedOrchestratorHeadStallsFIFO(t *testing.T) {
	queueTestHome(t)
	s := r167Orch(t)
	busy := r167Busy(t)
	if _, err := Handle(r167OrchRequest(s, "work1")); err != nil {
		t.Fatal(err)
	}
	plain := queueRequest("plain")
	if r, err := Handle(plain); err != nil || !strings.Contains(r.Message, "position 2") {
		t.Fatalf("%+v %v", r, err)
	}
	if err := os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "pause"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(busy, "admission.json"))
	var started []string
	original := runLauncher
	defer func() { runLauncher = original }()
	runLauncher = func(dir, name string) error { started = append(started, name); return writePID(dir) }
	for i := 0; i < 1; i++ {
		err := SweepQueue()
		t.Logf("sweep %d: %v", i, err)
	}
	entries, _ := queueEntries()
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Request.Name)
	}
	t.Logf("started=%v queue=%v", started, names)
	if len(entries) != 0 {
		t.Errorf("free slot, but paused orchestrator head blocks every later order")
	}
}

func TestR167ReworkDuringFinishingSupervisor(t *testing.T) {
	queueTestHome(t)
	dir := filepath.Join(Root(), "done1")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	o, _ := Parse(strings.Replace(good, "example", "done1", 1))
	_ = os.WriteFile(filepath.Join(dir, "order.md"), []byte(good), 0600)
	_ = WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: o, Start: time.Now().UTC()})
	_ = WriteJSON(filepath.Join(dir, "result.json"), Result{Start: time.Now().UTC(), End: time.Now().UTC(), ExitCode: 0})
	// The finishing supervisor still owns d-done1 while its deferred sweep runs.
	if out, err := tmuxCommand("new-session", "-d", "-s", "d-done1", "sleep 60").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	defer tmuxCommand("kill-session", "-t", "=d-done1").Run()
	live, _ := liveNames()
	t.Logf("tmux live=%v", live)
	_ = addReservations(live)
	t.Logf("after addReservations live=%v", live)
	_, err := Handle(Request{Action: "rework", Name: "done1", Note: "again"})
	t.Logf("rework while session alive: %v", err)
	_, statErr := os.Stat(filepath.Join(dir, "result.json"))
	t.Logf("result.json present after failed rework: %v", statErr == nil)
	if err == nil || !strings.Contains(err.Error(), "stop running order before rework") {
		t.Errorf("rework admitted while the run's tmux session is still alive: %v", err)
	}
}

func TestR167ReaperKillsStalePIDGroup(t *testing.T) {
	queueTestHome(t)
	if err := os.MkdirAll(filepath.Join(Root(), ".queue"), 0700); err != nil {
		t.Fatal(err)
	}
	// An unrelated process-group leader that happens to hold a PID left in a
	// stale harness.json (supervisor SIGKILLed, harness later exited).
	other := exec.Command("sleep", "30")
	other.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- other.Wait() }()
	dir := filepath.Join(Root(), "crashed")
	_ = os.MkdirAll(dir, 0700)
	_ = WriteJSON(filepath.Join(dir, "harness.json"), pidRecord{PID: other.Process.Pid, Created: "stale-identity"})
	if err := SweepQueue(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		t.Errorf("sweep SIGKILLed the unrelated process group holding the recorded PID: %v", err)
	case <-time.After(time.Second):
		_ = other.Process.Kill()
		<-done
		if exists(filepath.Join(dir, "harness.json")) {
			t.Fatal("stale harness record retained")
		}
	}
}

func TestR167ConcurrentSweepersRespectCapAndFIFO(t *testing.T) {
	queueTestHome(t)
	cfg := os.Getenv("HEV_DISPATCH_CONFIG")
	_ = os.WriteFile(cfg, []byte("cap=2\n"), 0600)
	source := filepath.Join(t.TempDir(), "source")
	_ = os.MkdirAll(source, 0700)
	for _, args := range [][]string{{"git", "init", "-b", "main"}, {"git", "-c", "user.name=T", "-c", "user.email=t@e.test", "commit", "--allow-empty", "-m", "i"}} {
		if _, err := command(source, args...); err != nil {
			t.Fatal(err)
		}
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if _, err := command("", "git", "clone", source, repo); err != nil {
		t.Fatal(err)
	}
	busy := r167Busy(t)
	busy2 := filepath.Join(Root(), "busy2")
	_ = os.MkdirAll(busy2, 0700)
	_ = reserve(busy2)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			q := queueRequest("q" + string(rune('a'+i)))
			q.Cap = 0
			q.Order = strings.Replace(q.Order, "repo: kit", "repo: "+filepath.ToSlash(repo), 1)
			q.Meta.Order, _ = Parse(q.Order)
			if _, err := Handle(q); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	entries, _ := queueEntries()
	order := []string{}
	for _, e := range entries {
		order = append(order, e.Request.Name)
	}
	var mu sync.Mutex
	started := []string{}
	original := runLauncher
	defer func() { runLauncher = original }()
	runLauncher = func(dir, name string) error {
		mu.Lock()
		started = append(started, name)
		mu.Unlock()
		return writePID(dir)
	}
	_ = os.Remove(filepath.Join(busy, "admission.json"))
	_ = os.Remove(filepath.Join(busy2, "admission.json"))
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = SweepQueue() }()
	}
	wg.Wait()
	t.Logf("queue=%v started=%v", order, started)
	// git fetch fails (repo "kit" absent), so admitted heads become failed runs
	// and free their slot; what matters is no name starts twice and order holds.
	if len(started) != 2 || started[0] != order[0] || started[1] != order[1] {
		t.Errorf("expected FIFO heads %v to start, got %v", order[:2], started)
	}
	seen := map[string]bool{}
	for _, n := range started {
		if seen[n] {
			t.Errorf("double start %s", n)
		}
		seen[n] = true
	}
}

func TestR167StopDuringQueuedAdmissionIsLost(t *testing.T) {
	queueTestHome(t)
	source := filepath.Join(t.TempDir(), "source")
	_ = os.MkdirAll(source, 0700)
	for _, args := range [][]string{{"git", "init", "-b", "main"}, {"git", "-c", "user.name=T", "-c", "user.email=t@e.test", "commit", "--allow-empty", "-m", "i"}} {
		if _, err := command(source, args...); err != nil {
			t.Fatal(err)
		}
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if _, err := command("", "git", "clone", source, repo); err != nil {
		t.Fatal(err)
	}
	real, _ := exec.LookPath("git")
	bin := t.TempDir()
	_ = os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\ncase \"$*\" in *fetch*) touch \"$HOME/fetch-entered\"; while [ ! -f \"$HOME/fetch-release\" ]; do sleep 0.01; done;; esac\nexec "+real+" \"$@\"\n"), 0700)
	busy := r167Busy(t)
	q := queueRequest("victim")
	q.Order = strings.Replace(q.Order, "repo: kit", "repo: "+filepath.ToSlash(repo), 1)
	q.Meta.Order, _ = Parse(q.Order)
	if r, err := Handle(q); err != nil || !strings.Contains(r.Message, "queued") {
		t.Fatalf("%+v %v", r, err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var started []string
	original := runLauncher
	defer func() { runLauncher = original }()
	runLauncher = func(dir, name string) error { started = append(started, name); return writePID(dir) }
	_ = os.Remove(filepath.Join(busy, "admission.json"))
	done := make(chan error, 1)
	go func() { done <- SweepQueue() }()
	deadline := time.Now().Add(5 * time.Second)
	for !exists(filepath.Join(os.Getenv("HOME"), "fetch-entered")) {
		if time.Now().After(deadline) {
			t.Fatal("admission did not reach fetch")
		}
		time.Sleep(10 * time.Millisecond)
	}
	r, err := Handle(Request{Action: "stop", Name: "victim"})
	t.Logf("stop reply: %q %v", r.Message, err)
	_ = os.WriteFile(filepath.Join(os.Getenv("HOME"), "fetch-release"), nil, 0600)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	t.Logf("started=%v stop file present=%v", started, exists(filepath.Join(Root(), "victim", "stop")))
	if len(started) == 1 && err == nil {
		t.Errorf("stop accepted during admission, but the order was launched anyway")
	}
}

func TestQueueBackstopUsesReceiverEntryPoint(t *testing.T) {
	queueTestHome(t)
	_ = os.MkdirAll(filepath.Join(Root(), ".queue"), 0700)
	wrapper := filepath.Join(t.TempDir(), "receiver")
	marker := filepath.Join(t.TempDir(), "called")
	t.Setenv("RECEIVER_TEST_MARKER", marker)
	t.Setenv("HEV_DISPATCH_RECEIVER", wrapper)
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\n[ \"$1 $2\" = \"dispatch sweep\" ] || exit 12\nprintf receiver > \"$RECEIVER_TEST_MARKER\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := SweepQueueBackstop(); err != nil {
		t.Fatal(err)
	}
	if !exists(marker) {
		t.Fatal("receiver wrapper not invoked")
	}
}
