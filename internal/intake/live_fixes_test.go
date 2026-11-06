package intake

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func rejectionError(t *testing.T, message string, code int) error {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestClassificationHelper$")
	cmd.Env = append(os.Environ(), "INTAKE_REJECTION="+message, "INTAKE_EXIT="+strconv.Itoa(code))
	_, err := cmd.Output()
	return err
}
func TestClassificationHelper(t *testing.T) {
	if code := os.Getenv("INTAKE_EXIT"); code != "" {
		fmt.Fprintln(os.Stderr, os.Getenv("INTAKE_REJECTION"))
		n, _ := strconv.Atoi(code)
		os.Exit(n)
	}
}
func TestNarrowDispatchRejections(t *testing.T) {
	for _, tc := range []struct {
		message string
		code    int
		reject  bool
	}{
		{"invalid name", 1, true}, {"worker: 0 free\nhost full (1/1)", 1, true}, {"host full (1/1)", 1, true},
		{"invalid remote response; for mutations the outcome is unknown", 1, false},
		{"invalid timeout", 2, false}, {"transport: invalid name", 1, false},
	} {
		t.Run(tc.message+strconv.Itoa(tc.code), func(t *testing.T) {
			f := setup(t)
			f.live(t)
			err := rejectionError(t, tc.message, tc.code)
			f.p.command = func(_ context.Context, args ...string) ([]byte, error) { return nil, err }
			name := lease(f.issue(), f.p.cfg.Status[0], f.entered)
			if e := f.p.launch(context.Background(), f.issue(), f.p.cfg.Status[0], name); e == nil {
				t.Fatal("expected failure")
			}
			if (len(f.p.state.Pending) == 0) != tc.reject {
				t.Fatal("wrong reservation classification")
			}
		})
	}
}
func TestPendingReconciliation(t *testing.T) {
	for _, observed := range []bool{true, false} {
		t.Run(strconv.FormatBool(observed), func(t *testing.T) {
			f := setup(t)
			name := "test-1-triage-abcdef"
			if e := f.p.reservation(name, "code", "reserve"); e != nil {
				t.Fatal(e)
			}
			now := f.p.now()
			f.p.now = func() time.Time { return now.Add(pendingGrace) }
			inventory := `{"local":{"rows":[]}}`
			if observed {
				inventory = fmt.Sprintf(`{"local":{"rows":[{"name":%q,"state":"done"}]}}`, name)
			}
			f.p.command = func(context.Context, ...string) ([]byte, error) { return []byte(inventory), nil }
			_, running, e := f.p.inventory(context.Background())
			if e != nil || running != 0 || len(f.p.state.Pending) != 0 {
				t.Fatal(e, running)
			}
			if observed {
				if !f.p.admitted[name] {
					t.Fatal("missing ledger")
				}
			} else {
				if f.p.state.NeedsReconcile[name] != "code" {
					t.Fatal("missing needs-reconcile")
				}
				// Replaying the journal after a crash must preserve the operator gate.
				p, e := newPoller(f.p.cfg, "", f.out)
				if e != nil {
					t.Fatal(e)
				}
				p.now = f.p.now
				if p.state.NeedsReconcile[name] != "code" {
					t.Fatal("lost gate")
				}
				if e = p.reconcile(name, false); e != nil {
					t.Fatal(e)
				}
				if len(p.state.NeedsReconcile) != 0 || p.day().Dispatches != 1 {
					t.Fatal("incorrect release")
				}
			}
		})
	}
	t.Run("confirmed", func(t *testing.T) {
		f := setup(t)
		name := "test-1-triage-abcdef"
		if e := f.p.reservation(name, "read", "needs-reconcile"); e != nil {
			t.Fatal(e)
		}
		if e := f.p.reconcile(name, true); e != nil {
			t.Fatal(e)
		}
		p, e := newPoller(f.p.cfg, "", f.out)
		if e != nil || !p.admitted[name] {
			t.Fatal(e)
		}
	})
}
func TestTicketFailureIsolationAndBackoff(t *testing.T) {
	f := setup(t)
	f.live(t)
	f.p.cfg.DailyCap = 20
	good := f.issue()
	good.ID = "good"
	good.Identifier = "TEST-2"
	f.extraIssues = []Issue{good}
	bad := lease(f.issue(), f.p.cfg.Status[0], f.entered)
	failure := rejectionError(t, "invalid name", 1)
	attempts := map[string]int{}
	f.p.command = func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] == "ls" {
			return []byte(`{"local":{"rows":[]}}`), nil
		}
		b, e := os.ReadFile(args[0])
		if e != nil {
			return nil, e
		}
		if strings.Contains(string(b), bad) {
			attempts[bad]++
			return nil, failure
		}
		attempts["good"]++
		return nil, nil
	}
	f.poll(t)
	if attempts["good"] != 1 || attempts[bad] != 1 {
		t.Fatal(attempts)
	}
	f.poll(t)
	if attempts[bad] != 1 {
		t.Fatal("backoff ignored")
	}
	f.poll(t)
	if attempts[bad] != 2 || f.p.state.TicketFailures[bad].Skip != 2 {
		t.Fatal("exponential backoff missing")
	}
	f.p.state.TicketFailures[bad] = TicketFailure{Count: 10}
	f.poll(t)
	if f.p.state.TicketFailures[bad].Skip != 64 {
		t.Fatal("uncapped backoff")
	}
}
func TestServerErrorPreservesQuotaReset(t *testing.T) {
	f := setup(t)
	f.status = 503
	reset := f.p.now().Add(2 * time.Hour)
	f.headers["X-RateLimit-Requests-Remaining"] = "0"
	f.headers["X-RateLimit-Requests-Reset"] = strconv.FormatInt(reset.UnixMilli(), 10)
	if e := f.p.poll(context.Background()); e == nil {
		t.Fatal("accepted 503")
	}
	if !f.p.state.BackoffUntil.Equal(reset) {
		t.Fatal("shortened reset")
	}
}
func TestGuestActorsDenied(t *testing.T) {
	for _, kind := range []string{"assignment", "transition", "cached"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			if kind == "cached" {
				f.poll(t)
				now := f.p.now()
				f.p.now = func() time.Time { return now.Add(15 * time.Minute) }
				f.out.Reset()
			}
			if kind == "transition" {
				f.transitionGuest = true
			} else {
				f.guest = true
			}
			f.poll(t)
			if strings.Contains(f.out.String(), "would-dispatch") {
				t.Fatal("guest allowed")
			}
		})
	}
}
func TestPrivateOrdersAndCleanup(t *testing.T) {
	for _, fails := range []bool{false, true} {
		t.Run(strconv.FormatBool(fails), func(t *testing.T) {
			f := setup(t)
			f.live(t)
			var path string
			f.p.command = func(_ context.Context, args ...string) ([]byte, error) {
				path = args[0]
				if filepath.Dir(path) != filepath.Join(f.p.cfg.StateDir, "tmp") {
					t.Fatal(path)
				}
				_, e := os.Stat(path)
				if e != nil {
					t.Fatal(e)
				}
				if e := privateOrder(path); e != nil {
					t.Fatal(e)
				}
				if fails {
					return nil, fmt.Errorf("lost response")
				}
				return nil, nil
			}
			_ = f.p.launch(context.Background(), f.issue(), f.p.cfg.Status[0], "test-1")
			if path == "" {
				t.Fatal("not launched")
			}
			if _, e := os.Stat(path); !os.IsNotExist(e) {
				t.Fatal("file retained")
			}
			stale := filepath.Join(f.p.cfg.StateDir, "tmp", "hev-intake-crash.md")
			if e := os.WriteFile(stale, []byte("private"), 0600); e != nil {
				t.Fatal(e)
			}
			if e := f.p.cleanTemp(); e != nil {
				t.Fatal(e)
			}
			if _, e := os.Stat(stale); !os.IsNotExist(e) {
				t.Fatal("crash file retained")
			}
		})
	}
	t.Run("validation redaction", func(t *testing.T) {
		f := setup(t)
		f.live(t)
		if e := os.WriteFile(f.p.cfg.Status[0].Template, []byte("---\n{{title}}: secret\n---\n## Acceptance"), 0600); e != nil {
			t.Fatal(e)
		}
		f.poll(t)
		if strings.Contains(f.out.String(), f.issue().Title) {
			t.Fatal("rendered content logged")
		}
		e := f.p.launch(context.Background(), f.issue(), f.p.cfg.Status[0], "test-1")
		if e == nil || e.Error() != "template validation failed" {
			t.Fatal(e)
		}
	})
}

func TestReconcileCLIAndStartupCleanup(t *testing.T) {
	f := setup(t)
	name := "test-1-triage-abcdef"
	if e := f.p.reservation(name, "code", "needs-reconcile"); e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(f.p.cfg.StateDir, "tmp")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	stale := filepath.Join(dir, "hev-intake-crash.md")
	if e := os.WriteFile(stale, []byte("private"), 0600); e != nil {
		t.Fatal(e)
	}
	config := filepath.Join(t.TempDir(), "intake.toml")
	text := fmt.Sprintf("state_dir = %q\n[linear]\napi_key_env = \"UNSET_INTAKE_KEY\"\nteam = \"TEST\"\nassignee = \"testbot\"\nallowed_actor_team = \"TEST\"\n[[status]]\nname = \"Review\"\naction = \"triage\"\nconcurrency = \"read\"\n", f.p.cfg.StateDir)
	if e := os.WriteFile(config, []byte(text), 0600); e != nil {
		t.Fatal(e)
	}
	if e := run([]string{"linear", "status", "--config", config}, f.out); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(f.out.String(), "needs-reconcile") {
		t.Fatal(f.out.String())
	}
	for _, args := range [][]string{
		{"linear", "reconcile", name, "--config", config},
		{"linear", "reconcile", name, "--admitted", "--not-admitted", "--config", config},
	} {
		if e := run(args, f.out); e == nil {
			t.Fatal("ambiguous reconciliation accepted")
		}
	}
	if e := run([]string{"linear", "reconcile", name, "--admitted", "--config", config}, f.out); e != nil {
		t.Fatal(e)
	}
	p, e := newPoller(f.p.cfg, "", f.out)
	if e != nil || !p.admitted[name] || len(p.state.NeedsReconcile) != 0 {
		t.Fatal("reconciliation not persisted", e)
	}
	if _, e := os.Stat(stale); !os.IsNotExist(e) {
		t.Fatal("startup did not remove crash file")
	}
}

func TestPendingGraceAndOperatorGate(t *testing.T) {
	f := setup(t)
	f.live(t)
	f.p.cfg.Status[0].Concurrency = "code"
	name := lease(f.issue(), f.p.cfg.Status[0], f.entered)
	if e := f.p.reservation(name, "code", "reserve"); e != nil {
		t.Fatal(e)
	}
	f.p.command = func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] != "ls" {
			t.Fatal("blocked lease launched")
		}
		return []byte(`{"local":{"rows":[]}}`), nil
	}
	_, running, e := f.p.inventory(context.Background())
	if e != nil || running != 1 {
		t.Fatal("grace slot missing", e, running)
	}
	now := f.p.now()
	f.p.now = func() time.Time { return now.Add(pendingGrace) }
	f.poll(t)
	if len(f.p.state.Pending) != 0 || f.p.state.NeedsReconcile[name] != "code" {
		t.Fatal("not marked for reconciliation")
	}
	if !strings.Contains(f.out.String(), "requires reconciliation") {
		t.Fatal("lease gate missing")
	}
	f.noIssues = true
	f.poll(t) // Empty candidate sets still reconcile inventory.
}
