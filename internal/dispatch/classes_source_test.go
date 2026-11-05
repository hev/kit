package dispatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

const classSourceTemplate = "---\nname: {{.RunName}}\nrepo: app\nreply: owner\nclass: code\n---\n{{.Action}} {{.Event.URL}}\n\n## Acceptance\n- Complete requested work.\n"

func classSourceText(name string) string {
	text := strings.Replace(orchestratorExample, "name: release-train", "name: "+name, 1)
	text = regexp.MustCompile(`selector: "[^"]*"`).ReplaceAllString(text, "selector: mine")
	return strings.Replace(text, "source: none", "agent: off\nsource:\n  plugin: fake\n  rules:\n    - {status: Ready, action: fix, template: fix.md, mode: live}", 1)
}

// classSourceSpec applies an agent: off source orchestrator whose one rule
// renders a class: code order.
func classSourceSpec(t *testing.T, name string) OrchestratorSpec {
	t.Helper()
	s, e := ApplyOrchestrator(classSourceText(name), false)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "fix.md"), []byte(classSourceTemplate), 0600); e != nil {
		t.Fatal(e)
	}
	return s
}

// classSourceHost prepares this root as a classes.code = 1 target with a
// fake source that reports one ready event, and returns a direct start.
func classSourceHost(t *testing.T) (direct func(name string) error, cfg string) {
	t.Helper()
	orchHome(t)
	workspace := classWorkspace(t)
	cfg = filepath.Join(t.TempDir(), "dispatch.toml")
	if e := os.WriteFile(cfg, []byte("cap = 10\nclasses = {code = 1}\ndefault_host = 'worker'\n[hosts.worker]\nlocal = true\nworkspace = '"+filepath.ToSlash(workspace)+"'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	fakeLiveLaunches(t)
	old := sourceCommand
	t.Cleanup(func() { sourceCommand = old })
	sourceCommand = func(_ context.Context, _ OrchestratorSpec, inv sourceInvocation) ([]byte, error) {
		switch inv.Args[0] {
		case "--hev-describe":
			return []byte(`{"source":true,"ack":true}`), nil
		case "events":
			return []byte("{\"id\":\"event-1\",\"kind\":\"issue\",\"url\":\"https://example.com/1\",\"fields\":{\"status\":\"Ready\"}}\n{\"cursor\":\"next\"}\n"), nil
		case "ack":
			return nil, nil
		}
		return nil, errors.New("unexpected plugin command")
	}
	direct = func(name string) error {
		text := classOrder(name, "code")
		o, e := Parse(text)
		if e != nil {
			t.Fatal(e)
		}
		_, e = Handle(Request{Action: "start", Name: name, Order: text, Meta: Meta{Order: o}, Workspace: workspace})
		return e
	}
	return direct, cfg
}

func sourceRound(s OrchestratorSpec) error {
	st, _ := orchestratorState(s.Name)
	d := OrchestratorDriver{Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }}
	return RunOrchestratorRound(context.Background(), s, &st, d)
}

func onlyReceipt(t *testing.T, s OrchestratorSpec) sourceReceipt {
	t.Helper()
	receipts, e := loadSourceReceipts(s)
	if e != nil || len(receipts) != 1 {
		t.Fatal(receipts, e)
	}
	for _, r := range receipts {
		return r
	}
	return sourceReceipt{}
}

func codeOccupancy(t *testing.T) int {
	t.Helper()
	unlock, e := lockRoot()
	if e != nil {
		t.Fatal(e)
	}
	defer unlock()
	live, e := liveNames()
	if e == nil {
		e = addReservations(live)
	}
	if e != nil {
		t.Fatal(e)
	}
	n, e := classOccupancy("code", "", live)
	if e != nil {
		t.Fatal(e)
	}
	return n
}

// An attempt whose start was lost holds its class slot on the target, does
// not block its own start, and lapses with the attempt grace.
func TestDispatchClassSourceAttemptingReceiptOccupiesSlot(t *testing.T) {
	direct, _ := classSourceHost(t)
	s := classSourceSpec(t, "alpha")
	old := sourceDispatch
	t.Cleanup(func() { sourceDispatch = old })
	sourceDispatch = func(_ context.Context, _ Host, q Request) (Response, error) {
		if q.AdmissionOnly || q.Action == "list" {
			return Handle(q)
		}
		return Response{}, transportError{errors.New("connection lost")}
	}
	if e := sourceRound(s); e == nil {
		t.Fatal("uncertain attempt accepted")
	}
	r := onlyReceipt(t, s)
	if r.Mode != "attempting" || r.Class != "code" {
		t.Fatalf("receipt %+v", r)
	}
	if e := direct("after"); !classFull(e) {
		t.Fatalf("attempting receipt did not occupy its class: %v", e)
	}
	if n := codeOccupancy(t); n != 1 {
		t.Fatalf("attempt occupies %d code slots", n)
	}
	// The attempt itself is admitted against its own slot, which its run
	// directory then replaces.
	if e := direct(r.Run); e != nil {
		t.Fatalf("attempting receipt blocked its own run: %v", e)
	}
	if exists(classReservationPath(r.Run)) {
		t.Fatal("start did not consume its class reservation")
	}
	if n := codeOccupancy(t); n != 1 {
		t.Fatalf("promoted attempt occupies %d code slots", n)
	}
}

// A lapsed reservation, past the attempt grace, frees its slot.
func TestDispatchClassReservationLapses(t *testing.T) {
	direct, _ := classSourceHost(t)
	s := classSourceSpec(t, "alpha")
	old := sourceDispatch
	t.Cleanup(func() { sourceDispatch = old })
	sourceDispatch = func(_ context.Context, _ Host, q Request) (Response, error) {
		if q.AdmissionOnly || q.Action == "list" {
			return Handle(q)
		}
		return Response{}, transportError{errors.New("connection lost")}
	}
	_ = sourceRound(s)
	r := onlyReceipt(t, s)
	var c classReservation
	if e := readJSON(classReservationPath(r.Run), &c); e != nil {
		t.Fatal(e)
	}
	c.At = c.At.Add(-sourceAttemptGrace - time.Minute)
	if e := WriteJSON(classReservationPath(r.Run), c); e != nil {
		t.Fatal(e)
	}
	if e := direct("after"); e != nil {
		t.Fatalf("lapsed reservation still held its slot: %v", e)
	}
}

// Two source orchestrators and a direct start race for classes.code = 1.
// Admission and the class reservation are one step under the root lock, so
// at most one wins at every point, one always wins, and the winner's own
// start is never refused by the losers.
func TestDispatchClassSourceReservationRace(t *testing.T) {
	for _, order := range []string{"probes-then-direct", "all-at-once"} {
		t.Run(order, func(t *testing.T) {
			direct, _ := classSourceHost(t)
			specs := []OrchestratorSpec{classSourceSpec(t, "alpha"), classSourceSpec(t, "beta")}
			var probesReady, probed, admitted sync.WaitGroup
			probesReady.Add(len(specs))
			probed.Add(len(specs))
			admitted.Add(len(specs) + 1)
			release := make(chan struct{})
			reserved := map[string]bool{}
			var mu sync.Mutex
			old := sourceDispatch
			t.Cleanup(func() { sourceDispatch = old })
			sourceDispatch = func(_ context.Context, _ Host, q Request) (Response, error) {
				switch {
				case q.Action == "list":
					return Handle(q)
				case q.AdmissionOnly:
					probesReady.Done()
					probesReady.Wait()
					r, e := Handle(q)
					mu.Lock()
					reserved[q.Orchestrator] = e == nil && r.ClassReserved
					mu.Unlock()
					probed.Done()
					admitted.Done()
					return r, e
				}
				<-release
				return Handle(q)
			}
			var wg sync.WaitGroup
			for _, s := range specs {
				wg.Add(1)
				go func() { defer wg.Done(); _ = sourceRound(s) }()
			}
			var directErr error
			wg.Add(1)
			go func() {
				defer wg.Done()
				if order == "probes-then-direct" {
					// The window the reservation closes: between a passed
					// probe and the attempt's start.
					probed.Wait()
				}
				directErr = direct("cli-one")
				admitted.Done()
			}()
			waitTimeout(t, &admitted)
			if n := codeOccupancy(t); n != 1 {
				t.Fatalf("after admission %d code slots are held", n)
			}
			close(release)
			waitTimeout(t, &wg)
			winners := 0
			if directErr == nil {
				winners++
			} else if !classFull(directErr) {
				t.Fatal(directErr)
			}
			for _, s := range specs {
				r := onlyReceipt(t, s)
				switch {
				case r.Mode == "live":
					winners++
				case reserved[s.Name]:
					t.Fatalf("%s reserved a slot, then its start was refused: %+v", s.Name, r)
				case r.Mode != "rejected" || !strings.Contains(r.Error, "class code full"):
					t.Fatalf("%s receipt %+v", s.Name, r)
				}
			}
			if winners != 1 {
				t.Fatalf("%d launches won classes.code = 1", winners)
			}
			if n := codeOccupancy(t); n != 1 {
				t.Fatalf("after launch %d code slots are held", n)
			}
		})
	}
}

func waitTimeout(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("racing launches did not finish")
	}
}

// withRoot switches this process to another host's dispatch root and config.
func withRoot(home, cfg string, f func()) {
	saved := map[string]string{}
	for _, k := range []string{"HOME", "USERPROFILE", "HEV_DISPATCH_CONFIG"} {
		saved[k] = os.Getenv(k)
	}
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("USERPROFILE", home)
	_ = os.Setenv("HEV_DISPATCH_CONFIG", cfg)
	defer func() {
		for k, v := range saved {
			_ = os.Setenv(k, v)
		}
	}()
	f()
}

// An owner dispatching to another host's root reserves the class slot on
// that target, so a lost start still occupies the target's slot.
func TestDispatchClassRemoteAttemptOccupiesTarget(t *testing.T) {
	direct, targetCfg := classSourceHost(t)
	targetHome := os.Getenv("HOME")
	ownerHost := Host{SSH: "owner.example"}
	if _, e := RegisterOrchestratorScope(classSourceText("alpha"), ownerHost, false); e != nil {
		t.Fatal(e)
	}
	// The owner is a separate root whose worker host is reached over SSH.
	ownerHome := t.TempDir()
	ownerCfg := filepath.Join(t.TempDir(), "dispatch.toml")
	if e := os.WriteFile(ownerCfg, []byte("default_host = 'worker'\n[hosts.worker]\nssh = 'worker.example'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	var s OrchestratorSpec
	withRoot(ownerHome, ownerCfg, func() { s = classSourceSpec(t, "alpha") })

	oldDispatch, oldRoute := sourceDispatch, routeRPC
	t.Cleanup(func() { sourceDispatch, routeRPC = oldDispatch, oldRoute })
	starts := 0
	sourceDispatch = func(_ context.Context, h Host, q Request) (Response, error) {
		if h.SSH != "worker.example" {
			t.Fatalf("dispatch to %+v", h)
		}
		if !q.AdmissionOnly && q.Action == "start" {
			starts++
			return Response{}, transportError{errors.New("connection lost")}
		}
		var r Response
		var e error
		withRoot(targetHome, targetCfg, func() { r, e = Handle(q) })
		return r, e
	}
	routeRPC = func(_ context.Context, h Host, q Request) (Response, error) {
		if h.SSH != ownerHost.SSH {
			t.Fatalf("route to %+v", h)
		}
		var r Response
		var e error
		withRoot(ownerHome, ownerCfg, func() { r, e = Handle(q) })
		return r, e
	}
	var r sourceReceipt
	withRoot(ownerHome, ownerCfg, func() {
		e := sourceRound(s)
		r = onlyReceipt(t, s)
		if e == nil {
			t.Fatalf("uncertain attempt accepted: %+v", r)
		}
	})
	if starts != 1 || r.Mode != "attempting" || r.Host != "worker" {
		t.Fatalf("starts %d, receipt %+v", starts, r)
	}
	if exists(filepath.Join(ownerHome, "dispatch", ".class-reservations")) {
		t.Fatal("owner reserved a slot it does not hold")
	}
	if e := direct("cli-one"); !classFull(e) {
		t.Fatalf("remote attempting receipt did not occupy the target's class: %v", e)
	}
}

// A start the target definitively rejects after its probe reserved the class
// frees the slot at once, whichever check refused it: global cap, budget or
// orchestrator lifecycle.
func TestDispatchClassDefinitiveRejectionReleasesSlot(t *testing.T) {
	for name, before := range map[string]func(t *testing.T, s OrchestratorSpec, cfg string) func(){
		"global-cap": func(t *testing.T, _ OrchestratorSpec, cfg string) func() {
			b, e := os.ReadFile(cfg)
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(cfg, []byte(strings.Replace(string(b), "cap = 10", "cap = 1", 1)), 0600); e != nil {
				t.Fatal(e)
			}
			end := fakeLiveLaunches(t)
			docs := classOrder("docs", "docs")
			o, _ := Parse(docs)
			if _, e = Handle(Request{Action: "start", Name: "docs", Order: docs, Meta: Meta{Order: o}, Workspace: classWorkspace(t)}); e != nil {
				t.Fatal(e)
			}
			return func() { end("docs") }
		},
		"budget": func(t *testing.T, s OrchestratorSpec, _ string) func() {
			day := map[string]any{"day": time.Now().UTC().Format("2006-01-02"), "runs": s.Budget.RunsPerDay}
			if e := WriteJSON(filepath.Join(OrchestratorDir(s.Name), "budget.json"), day); e != nil {
				t.Fatal(e)
			}
			return func() {}
		},
		"pause": func(t *testing.T, s OrchestratorSpec, _ string) func() {
			if e := os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "pause"), []byte("pause"), 0600); e != nil {
				t.Fatal(e)
			}
			return func() {}
		},
		"stop": func(t *testing.T, s OrchestratorSpec, _ string) func() {
			if e := os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "stop"), []byte("stop"), 0600); e != nil {
				t.Fatal(e)
			}
			return func() {}
		},
	} {
		t.Run(name, func(t *testing.T) {
			direct, cfg := classSourceHost(t)
			s := classSourceSpec(t, "alpha")
			old := sourceDispatch
			t.Cleanup(func() { sourceDispatch = old })
			reserved, after := false, func() {}
			sourceDispatch = func(_ context.Context, _ Host, q Request) (Response, error) {
				if !q.AdmissionOnly && q.Action == "start" {
					reserved = exists(classReservationPath(q.Name))
					after = before(t, s, cfg)
				}
				return Handle(q)
			}
			_ = sourceRound(s)
			r := onlyReceipt(t, s)
			if !reserved || r.Mode != "rejected" {
				t.Fatalf("reserved %v, receipt %+v", reserved, r)
			}
			t.Logf("start rejected: %s", r.Error)
			if exists(classReservationPath(r.Run)) {
				t.Fatalf("definitive %s rejection left the class reserved: %s", name, r.Error)
			}
			after()
			if e := direct("after"); e != nil {
				t.Fatalf("code slot still held after a %s rejection: %v", name, e)
			}
		})
	}
}

// The reviewer's probe: a probe reserves, another class fills the global
// cap, and the definitive cap rejection releases the reservation.
func TestReviewDefinitiveRejectionReleasesClass(t *testing.T) {
	direct, cfg := classSourceHost(t)
	text := classOrder("reserved", "code")
	r, e := Handle(Request{Action: "start", Name: "reserved", Order: text, AdmissionOnly: true, ReserveClass: true})
	if e != nil || !r.ClassReserved {
		t.Fatal(r, e)
	}
	if e = direct("other"); !classFull(e) {
		t.Fatal(e)
	}
	b, e := os.ReadFile(cfg)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(cfg, []byte(strings.Replace(string(b), "cap = 10", "cap = 1", 1)), 0600); e != nil {
		t.Fatal(e)
	}
	docs := classOrder("docs", "docs")
	o, _ := Parse(docs)
	if _, e = Handle(Request{Action: "start", Name: "docs", Order: docs, Meta: Meta{Order: o}, Workspace: classWorkspace(t)}); e != nil {
		t.Fatal(e)
	}
	o, _ = Parse(text)
	if _, e = Handle(Request{Action: "start", Name: "reserved", Order: text, Meta: Meta{Order: o}}); e == nil {
		t.Fatal("expected global cap rejection")
	}
	if exists(classReservationPath("reserved")) {
		t.Fatal("definitive global cap rejection left class reservation occupied")
	}
}

// A target that cannot reserve a class (an older hev) is refused before the
// attempting receipt or the start, rather than bypassing the cap.
func TestDispatchClassOldTargetRefused(t *testing.T) {
	classSourceHost(t)
	s := classSourceSpec(t, "alpha")
	old := sourceDispatch
	t.Cleanup(func() { sourceDispatch = old })
	starts := 0
	sourceDispatch = func(_ context.Context, _ Host, q Request) (Response, error) {
		if q.AdmissionOnly {
			// An older target admits the probe but ignores ReserveClass.
			return Response{}, nil
		}
		if q.Action == "start" {
			starts++
		}
		return Handle(q)
	}
	_ = sourceRound(s)
	r := onlyReceipt(t, s)
	if starts != 0 || r.Mode != "rejected" || !strings.Contains(r.Error, "cannot reserve class code") {
		t.Fatalf("old target not refused: starts %d, receipt %+v", starts, r)
	}
}
