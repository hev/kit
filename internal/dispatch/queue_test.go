package dispatch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func queueTestHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg := filepath.Join(home, "target.toml")
	if err := os.WriteFile(cfg, []byte("cap=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	if err := os.MkdirAll(Root(), 0700); err != nil {
		t.Fatal(err)
	}
}
func queueRequest(name string) Request {
	text := strings.Replace(good, "example", name, 1)
	o, _ := Parse(text)
	return Request{Action: "start", Name: name, Order: text, Meta: Meta{Order: o}, Cap: 1}
}
func TestQueueConcurrentFIFOAndCancel(t *testing.T) {
	queueTestHome(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, name := range []string{"first", "second"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			unlock, err := lockRoot()
			if err != nil {
				errs <- err
				return
			}
			defer unlock()
			_, err = enqueue(queueRequest(name))
			errs <- err
		}(name)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	entries, err := queueEntries()
	if err != nil || len(entries) != 2 {
		t.Fatalf("%+v %v", entries, err)
	}
	if entries[0].Enqueued.After(entries[1].Enqueued) {
		t.Fatal("FIFO reversed")
	}
	r, found, err := controlQueued(Request{Action: "show", Name: entries[1].Request.Name})
	if err != nil || !found || !strings.Contains(r.Message, "position 2") {
		t.Fatalf("%+v %v", r, err)
	}
	if _, found, err := controlQueued(Request{Action: "stop", Name: entries[0].Request.Name}); !found || err != nil {
		t.Fatal(err)
	}
	entries, err = queueEntries()
	if err != nil || len(entries) != 1 {
		t.Fatalf("%+v %v", entries, err)
	}
	r, found, err = controlQueued(Request{Action: "show", Name: entries[0].Request.Name})
	if err != nil || !found || !strings.Contains(r.Message, "position 1") {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestQueueHostSelectionPrefersFree(t *testing.T) {
	c := Config{DefaultHost: "a", Hosts: map[string]Host{"a": {Cap: 1}, "b": {Cap: 1}}}
	inv := map[string]Response{"a": {Cap: 1, Running: 1}, "b": {Cap: 1}}
	if n, err := SelectQueueHost(c, inv); err != nil || n != "b" {
		t.Fatalf("%s %v", n, err)
	}
	inv["b"] = Response{Cap: 1, Running: 1}
	if n, err := SelectQueueHost(c, inv); err != nil || n != "a" {
		t.Fatalf("%s %v", n, err)
	}
}

// Real repository setup and admission exercise the same lock and reservations
// as RPCs. Only the external harness launcher is replaced.
func TestQueueStartOnSlotFreeAndCrashSweep(t *testing.T) {
	queueTestHome(t)
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "init", "-b", "main"},
		{"git", "-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "--allow-empty", "-m", "initial"},
	} {
		if _, err := command(source, args...); err != nil {
			t.Fatal(err)
		}
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if _, err := command("", "git", "clone", source, repo); err != nil {
		t.Fatal(err)
	}
	busy := filepath.Join(Root(), "busy")
	if err := os.MkdirAll(busy, 0700); err != nil {
		t.Fatal(err)
	}
	if err := reserve(busy); err != nil {
		t.Fatal(err)
	}
	original := runLauncher
	defer func() { runLauncher = original }()
	child := exec.Command(os.Args[0], "-test.run=^TestQueueSupervisorHelper$")
	child.Env = append(os.Environ(), "DISPATCH_SIGNAL_HELPER=1", "QUEUE_SUPERVISOR_HELPER=1")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill() })
	var started []string
	runLauncher = func(dir, name string) error {
		started = append(started, name)
		if name == "first" {
			return writeProcessRecord(filepath.Join(dir, "supervisor.json"), child.Process.Pid)
		}
		return writePID(dir)
	}
	for i, name := range []string{"first", "second"} {
		q := queueRequest(name)
		q.Order = strings.Replace(q.Order, "repo: kit", "repo: "+filepath.ToSlash(repo), 1)
		q.Meta.Order, _ = Parse(q.Order)
		r, err := Handle(q)
		if err != nil || !strings.Contains(r.Message, "position "+string(rune('1'+i))) {
			t.Fatalf("%+v %v", r, err)
		}
	}
	if err := SweepQueue(); err != nil {
		t.Fatal(err)
	}
	if len(started) != 0 {
		t.Fatal("started while full")
	}
	if err := os.Remove(filepath.Join(busy, "admission.json")); err != nil {
		t.Fatal(err)
	}
	if err := SweepQueue(); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 || started[0] != "first" {
		t.Fatalf("%v", started)
	}
	// Kill a real supervisor process without a completion callback. The next
	// backstop sweep must recognize its stale process record and free capacity.
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	if err := SweepQueue(); err != nil {
		t.Fatal(err)
	}
	if len(started) != 2 || started[1] != "second" {
		t.Fatalf("%v", started)
	}
	entries, err := queueEntries()
	if err != nil || len(entries) != 0 {
		t.Fatalf("%+v %v", entries, err)
	}
}

func TestQueueSupervisorHelper(t *testing.T) {
	if os.Getenv("QUEUE_SUPERVISOR_HELPER") != "1" {
		return
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

func TestQueueRecoversInterruptedAdmission(t *testing.T) {
	queueTestHome(t)
	unlock, err := lockRoot()
	if err != nil {
		t.Fatal(err)
	}
	_, err = enqueue(queueRequest("interrupted"))
	unlock()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(Root(), "interrupted")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := SweepQueue(); err != nil {
		t.Fatal(err)
	}
	entries, err := queueEntries()
	if err != nil || len(entries) != 0 {
		t.Fatalf("%+v %v", entries, err)
	}
	var result Result
	if err := readJSON(filepath.Join(dir, "result.json"), &result); err != nil || !result.AdmissionFailed || result.End.IsZero() {
		t.Fatalf("%+v %v", result, err)
	}
	var m Meta
	if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil || m.Name != "interrupted" {
		t.Fatalf("%+v %v", m, err)
	}
}
