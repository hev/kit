package intake

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func Test400RateLimited(t *testing.T) {
	f := setup(t)
	f.status = 400
	f.errorBody = `{"errors":[{"message":"You have exceeded the rate limit","extensions":{"code":"RATELIMITED","statusCode":429}}]}`
	f.headers["Retry-After"] = "600"
	if e := f.p.poll(context.Background()); e == nil {
		t.Fatal("accepted rate limit")
	}
	f.poll(t)
	if f.calls != 1 || !f.p.state.BackoffUntil.Equal(f.p.now().Add(10*time.Minute)) {
		t.Fatal("not suspended")
	}
}
func TestOutsideTransitionAndMembershipRevoked(t *testing.T) {
	t.Run("outside transition", func(t *testing.T) {
		f := setup(t)
		f.transitionOutside = true
		f.poll(t)
		if strings.Contains(f.out.String(), "would-dispatch") {
			t.Fatal(f.out.String())
		}
	})
	t.Run("revoked membership", func(t *testing.T) {
		f := setup(t)
		f.poll(t)
		f.member = false
		now := f.p.now()
		f.p.now = func() time.Time { return now.Add(15 * time.Minute) }
		f.out.Reset()
		f.poll(t)
		if strings.Contains(f.out.String(), "would-dispatch") || f.histories != 1 {
			t.Fatal(f.out.String(), f.histories)
		}
	})
}
func TestURLSlugNeverStored(t *testing.T) {
	f := setup(t)
	f.poll(t)
	b, e := os.ReadFile(filepath.Join(f.p.cfg.StateDir, "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b)+f.out.String(), "private-title-slug") {
		t.Fatal("slug persisted")
	}
	if !strings.Contains(f.out.String(), "https://example.invalid/issue/TEST-1") {
		t.Fatal(f.out.String())
	}
}
func TestWindowsStyleSecretEnv(t *testing.T) {
	for _, s := range []string{"LINEAR_API_KEY=secret", "linear_api_key=secret", "Linear_Api_Key=secret"} {
		if !secretEntry(s, "LINEAR_API_KEY") {
			t.Fatal(s)
		}
	}
	if secretEntry("LINEAR_API_KEY_OTHER=ok", "LINEAR_API_KEY") {
		t.Fatal("wrong variable")
	}
}
func TestRefusedStartsNotRecorded(t *testing.T) {
	f := setup(t)
	for j := 0; j < 3; j++ {
		if e := f.p.start(); e != nil {
			t.Fatal(e)
		}
	}
	if e := f.p.start(); e == nil {
		t.Fatal("accepted fourth")
	}
	if len(f.p.state.Starts) != 3 {
		t.Fatal("recorded refused start")
	}
}
func TestOSLockRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "RUNNING")
	os.WriteFile(path, []byte("stale"), 0600)
	release, e := lockState(path)
	if e != nil {
		t.Fatal(e)
	}
	if r, e := lockState(path); e == nil {
		r()
		t.Fatal("parallel lock")
	}
	release()
	r, e := lockState(path)
	if e != nil {
		t.Fatal(e)
	}
	r()
}
func TestStrictKitOrders(t *testing.T) {
	good := "---\nname: test-1\nhost: worker\nrepo: synthetic\npreview: python3 -m http.server {port}\n---\n## Acceptance\n- Check\n"
	if e := validateOrder(good, "test-1"); e != nil {
		t.Fatal(e)
	}
	for _, field := range []string{"unknown: value", "repo: duplicate", "linear: A\nissue: B", "timeout: zero", "harness: other", "validation: other", "model:", "base: ../bad", "model: [nested]", "model: 'unclosed"} {
		text := strings.Replace(good, "preview:", field+"\npreview:", 1)
		if e := validateOrder(text, "test-1"); e == nil {
			t.Fatal(field)
		}
	}
}
func TestInventoryNamesLedgerAndUnrelatedJobs(t *testing.T) {
	t.Run("names", func(t *testing.T) {
		f := setup(t)
		name := lease(f.issue(), f.p.cfg.Status[0], f.entered)
		t.Setenv("INTAKE_INVENTORY", fmt.Sprintf(`{"local":{"names":[%q],"rows":[]}}`, name))
		f.poll(t)
		if strings.Contains(f.out.String(), "would-dispatch") {
			t.Fatal(f.out.String())
		}
	})
	t.Run("ledger after clean", func(t *testing.T) {
		f := setup(t)
		f.live(t)
		f.poll(t)
		p, e := newPoller(f.p.cfg, "fake-only", f.out)
		if e != nil {
			t.Fatal(e)
		}
		p.now = f.p.now
		p.endpoint = f.p.endpoint
		f.p = p
		f.out.Reset()
		f.poll(t)
		if p.day().Dispatches != 1 || strings.Contains(f.out.String(), `"decision":"dispatched"`) {
			t.Fatal(f.out.String())
		}
	})
	t.Run("unrelated", func(t *testing.T) {
		f := setup(t)
		f.live(t)
		f.p.cfg.Status[0].Concurrency = "code"
		t.Setenv("INTAKE_INVENTORY", `{"local":{"rows":[{"name":"other-unrelated","state":"running"}]}}`)
		f.poll(t)
		if f.p.day().Dispatches != 1 {
			t.Fatal(f.out.String())
		}
	})
}
func TestThresholdStopsHistoryPagination(t *testing.T) {
	f := setup(t)
	f.historyPage = true
	f.historyHeaders = true
	f.poll(t)
	if f.histories != 1 || f.calls != 2 {
		t.Fatal("continued after quota", f.calls)
	}
}
func TestServerFailureExponentialBackoff(t *testing.T) {
	f := setup(t)
	f.status = 503
	for j := 0; j < 3; j++ {
		now := f.p.now()
		if e := f.p.poll(context.Background()); e == nil {
			t.Fatal("accepted 503")
		}
		want := now.Add(5 * time.Minute * time.Duration(1<<j))
		if !f.p.state.BackoffUntil.Equal(want) {
			t.Fatal(f.p.state.BackoffUntil, want)
		}
		f.poll(t)
		f.p.now = func() time.Time { return want.Add(time.Second) }
	}
	if f.calls != 3 {
		t.Fatal(f.calls)
	}
}
func TestPruneState(t *testing.T) {
	f := setup(t)
	f.poll(t)
	f.p.state.Days["2020-01-01"] = &Daily{}
	f.noIssues = true
	f.poll(t)
	if len(f.p.state.Cache) != 0 || len(f.p.state.Days) != 1 {
		t.Fatal("state not pruned")
	}
}
func TestDeterministicRejectionReleasesCap(t *testing.T) {
	f := setup(t)
	f.live(t)
	command := f.p.command
	f.p.command = func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] == "ls" {
			return command(ctx, args...)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestRejectHelper$")
		_, err := cmd.Output()
		return nil, err
	}
	if e := f.p.poll(context.Background()); e != nil {
		t.Fatal(e)
	}
	if f.p.day().Dispatches != 0 || len(f.p.state.Pending) != 0 {
		t.Fatal("rejection retained reservation")
	}
}

func TestReservationJournalRecovery(t *testing.T) {
	f := setup(t)
	if e := os.MkdirAll(f.p.cfg.StateDir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := f.p.reservation("test-1-triage-abcdef", "code", "reserve"); e != nil {
		t.Fatal(e)
	}
	p, e := newPoller(f.p.cfg, "fake-only", f.out)
	if e != nil {
		t.Fatal(e)
	}
	p.now = f.p.now
	if p.day().Dispatches != 1 || p.state.Pending["test-1-triage-abcdef"] != "code" {
		t.Fatal("lost crash reservation")
	}
}
func TestReservedNamesConsumeSlot(t *testing.T) {
	f := setup(t)
	f.live(t)
	f.p.cfg.Status[0].Concurrency = "code"
	t.Setenv("INTAKE_INVENTORY", `{"local":{"names":["test-2-triage-abcdef"],"rows":[]}}`)
	f.poll(t)
	if f.p.day().Dispatches != 0 {
		t.Fatal("reservation ignored")
	}
}

func TestOnceExemptFromRestartGuard(t *testing.T) {
	f := setup(t)
	for j := 0; j < 3; j++ {
		if e := f.p.start(); e != nil {
			t.Fatal(e)
		}
	}
	pause := filepath.Join(f.p.cfg.StateDir, "PAUSED")
	os.WriteFile(pause, nil, 0600)
	config := filepath.Join(t.TempDir(), "config.toml")
	text := fmt.Sprintf("state_dir = %q\npause_file = %q\n[linear]\napi_key_env = \"FAKE_LINEAR_KEY\"\nteam = \"TEST\"\nassignee = \"testbot\"\nallowed_actor_team = \"TEST\"\n[[status]]\nname = \"Review\"\naction = \"triage\"\nconcurrency = \"read\"\n", f.p.cfg.StateDir, pause)
	os.WriteFile(config, []byte(text), 0600)
	if e := runWatchTest([]string{"linear", "once", "--config", config}, f.out); e != nil {
		t.Fatal(e)
	}
	p, e := newPoller(f.p.cfg, "", f.out)
	if e != nil || len(p.state.Starts) != 3 {
		t.Fatal("once counted as run start", e)
	}
}
func TestMinimumInterval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("interval = \"1m\"\n"), 0600)
	if _, e := loadConfig(path); e == nil || !strings.Contains(e.Error(), "at least 5m") {
		t.Fatal(e)
	}
}

func TestSystemTransitionDenied(t *testing.T) {
	f := setup(t)
	f.transitionOutside = true
	f.transitionSystem = true
	f.poll(t)
	if strings.Contains(f.out.String(), "would-dispatch") {
		t.Fatal(f.out.String())
	}
}

func TestRejectHelper(t *testing.T) {
	if os.Getenv("INTAKE_HELPER") == "1" {
		fmt.Fprintln(os.Stderr, "host full (1/1)")
		os.Exit(1)
	}
}
