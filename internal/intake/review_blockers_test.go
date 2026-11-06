package intake

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRealKitRejections(t *testing.T) {
	for _, file := range []string{"kit-bad-order.stderr", "kit-host-full.stderr"} {
		t.Run(file, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("testdata", file))
			if err != nil {
				t.Fatal(err)
			}
			for _, code := range []int{1, 2} {
				f := setup(t)
				f.live(t)
				failure := rejectionError(t, string(b), code)
				f.p.command = func(context.Context, ...string) ([]byte, error) { return nil, failure }
				name := lease(f.issue(), f.p.cfg.Status[0], f.entered)
				if err := f.p.launch(context.Background(), f.issue(), f.p.cfg.Status[0], name); err == nil {
					t.Fatal("expected rejection")
				}
				if (len(f.p.state.Pending) == 0) != (code == 1) {
					t.Fatal("wrong refund", code)
				}
			}
		})
	}
}

func TestHostFullExact(t *testing.T) {
	for _, s := range []string{"prefix host full (1/1)", "host full (1/1) suffix", "Error: host full (1/1) uncertain\nUsage:\n", "Error: transport failed\nUsage:\nhost full (1/1)"} {
		if deterministic(s) {
			t.Fatalf("refunded %q", s)
		}
	}
}

func TestHostFullDoesNotBackoff(t *testing.T) {
	f := setup(t)
	f.live(t)
	failure := rejectionError(t, "worker: 0 free\nError: host full (1/1)\nUsage:\n hev dispatch ORDER.md\nhost full (1/1)", 1)
	attempts := 0
	f.p.command = func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] == "ls" {
			return []byte(`{"local":{}}`), nil
		}
		attempts++
		return nil, failure
	}
	f.poll(t)
	f.poll(t)
	if attempts != 2 || len(f.p.state.TicketFailures) != 0 || f.p.day().Dispatches != 0 || len(f.p.state.Pending) != 0 {
		t.Fatal("capacity penalized", attempts, f.p.state)
	}
}

func TestUncertainStopsPollAndHoldsCodeSlot(t *testing.T) {
	f := setup(t)
	f.live(t)
	f.p.cfg.Status[0].Concurrency = "code"
	other := f.issue()
	other.ID = "second"
	other.Identifier = "TEST-2"
	f.extraIssues = []Issue{other}
	attempts := 0
	f.p.command = func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] == "ls" {
			return []byte(`{"local":{}}`), nil
		}
		attempts++
		return nil, fmt.Errorf("lost response")
	}
	f.poll(t)
	if attempts != 1 || f.histories != 1 || len(f.p.state.Pending) != 1 {
		t.Fatal("poll continued", attempts, f.histories)
	}
	_, running, err := f.p.inventory(context.Background())
	if err != nil || running != 1 {
		t.Fatal("lost slot", running, err)
	}
	f.poll(t)
	if attempts != 1 || !strings.Contains(f.out.String(), "code-job limit") {
		t.Fatal("uncertain code slot reused")
	}
}

func TestCancellationBeforeReserve(t *testing.T) {
	f := setup(t)
	f.live(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.p.launch(ctx, f.issue(), f.p.cfg.Status[0], "test-1"); err != context.Canceled {
		t.Fatal(err)
	}
	if len(f.p.state.Pending) != 0 || f.p.day().Dispatches != 0 {
		t.Fatal("reserved cancelled work")
	}
}

func TestSuccessClearsBackoff(t *testing.T) {
	f := setup(t)
	f.live(t)
	name := lease(f.issue(), f.p.cfg.Status[0], f.entered)
	f.p.state.TicketFailures[name] = TicketFailure{Count: 4}
	f.poll(t)
	if _, ok := f.p.state.TicketFailures[name]; ok {
		t.Fatal("success retained failure")
	}
	if !f.p.admitted[name] {
		t.Fatal("not admitted")
	}
}

func TestEmptyCandidatesReconcileAndPrune(t *testing.T) {
	f := setup(t)
	f.noIssues = true
	name := "test-2-triage-abcdef"
	if err := f.p.reservation(name, "code", "reserve"); err != nil {
		t.Fatal(err)
	}
	f.p.state.TicketFailures["stale"] = TicketFailure{Count: 1}
	f.p.state.TicketFailures[name] = TicketFailure{Count: 1}
	f.p.state.NeedsReconcile["stale"] = "read"
	f.p.state.NeedsReconcile[name] = "code"
	t.Setenv("INTAKE_INVENTORY", fmt.Sprintf(`{"local":{"rows":[{"name":%q,"state":"done"}]}}`, name))
	f.poll(t)
	if !f.p.admitted[name] || len(f.p.state.Pending) != 0 {
		t.Fatal("empty candidates skipped inventory")
	}
	if len(f.p.state.TicketFailures) != 1 || len(f.p.state.NeedsReconcile) != 1 {
		t.Fatal("not pruned", f.p.state)
	}
}

func TestFakeLinearRejectsUnknownUserField(t *testing.T) {
	f := setup(t)
	var data any
	for _, query := range []string{
		`query Membership { user(id: "actor") { isGuest } }`,
		`query Membership { user(id: "actor") { unexpectedField } }`,
		`query Membership { user(id: "actor") { alias: anotherUnknown } }`,
		`query Candidates { users { nodes { bogus } } }`,
		`query History { issue { history { nodes { actor { bogus } } } } }`,
	} {
		if err := f.p.gql(context.Background(), query, nil, &data); err == nil {
			t.Fatalf("fake accepted unknown User field: %s", query)
		}
	}

}

func TestPruneRetainsUncheckedCandidateGates(t *testing.T) {
	f := setup(t)
	name := "test-1-triage-abcdef"
	f.p.state.TicketFailures[name] = TicketFailure{Count: 2}
	f.p.state.NeedsReconcile[name] = "code"
	f.p.pruneLeases([]Issue{f.issue()}, map[string]bool{})
	if len(f.p.state.TicketFailures) != 1 || len(f.p.state.NeedsReconcile) != 1 {
		t.Fatal("unchecked candidate lost gate")
	}
	f.poll(t)
	if len(f.p.state.TicketFailures) != 0 || len(f.p.state.NeedsReconcile) != 1 {
		t.Fatal("obsolete lease retained after history check")
	}
	current := lease(f.issue(), f.p.cfg.Status[0], f.entered)
	f.p.state.TicketFailures[current] = TicketFailure{Count: 2}
	f.p.state.NeedsReconcile[current] = "code"
	f.poll(t)
	if len(f.p.state.TicketFailures) != 1 || len(f.p.state.NeedsReconcile) != 2 {
		t.Fatal("current lease pruned")
	}
}

func TestPollCancellationDoesNotMarkUnlaunched(t *testing.T) {
	f := setup(t)
	f.live(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.p.command = func(context.Context, ...string) ([]byte, error) { cancel(); return []byte(`{"local":{}}`), nil }
	if err := f.p.poll(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(f.p.state.Pending) != 0 || len(f.p.state.TicketFailures) != 0 || f.p.day().Dispatches != 0 {
		t.Fatal("cancelled ticket marked failed/uncertain")
	}
}

func TestLocalPreparationFailureContinuesPoll(t *testing.T) {
	f := setup(t)
	f.live(t)
	// Only the first ticket renders an invalid frontmatter scalar.
	if err := os.WriteFile(f.p.cfg.Status[0].Template, []byte("---\nname: {{name}}\nrepo: {{identifier}}\nbase: {{title}}\n---\n## Acceptance\n- Check\n"), 0600); err != nil {
		t.Fatal(err)
	}
	good := f.issue()
	good.ID = "good"
	good.Identifier = "TEST-2"
	good.Title = "main"
	f.extraIssues = []Issue{good}
	f.poll(t)
	if f.p.day().Dispatches != 1 || !strings.Contains(f.out.String(), `"decision":"failed"`) || !strings.Contains(f.out.String(), `"decision":"dispatched"`) {
		t.Fatal(f.out.String())
	}
}

func TestReconcileGateSurvivesMissingCandidate(t *testing.T) {
	f := setup(t)
	f.live(t)
	name := lease(f.issue(), f.p.cfg.Status[0], f.entered)
	if err := f.p.reservation(name, "code", "needs-reconcile"); err != nil {
		t.Fatal(err)
	}
	launches := 0
	f.p.command = func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] == "ls" {
			return []byte(`{"local":{}}`), nil
		}
		launches++
		return nil, nil
	}
	f.noIssues = true
	f.poll(t)
	f.noIssues = false
	f.poll(t)
	if launches != 0 || f.p.state.NeedsReconcile[name] != "code" {
		t.Fatal("operator gate lost", launches, f.p.state)
	}
	config := filepath.Join(t.TempDir(), "intake.toml")
	text := fmt.Sprintf("state_dir = %q\n[linear]\napi_key_env = \"UNSET_INTAKE_KEY\"\nteam = \"TEST\"\nassignee = \"testbot\"\nallowed_actor_team = \"TEST\"\n[[status]]\nname = \"Review\"\naction = \"triage\"\nconcurrency = \"read\"\n", f.p.cfg.StateDir)
	if err := os.WriteFile(config, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	if err := run([]string{"linear", "status", "--config", config}, f.out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.out.String(), name) || !strings.Contains(f.out.String(), "needs-reconcile") {
		t.Fatal(f.out.String())
	}
	if err := run([]string{"linear", "reconcile", name, "--not-admitted", "--config", config}, f.out); err != nil {
		t.Fatal(err)
	}
	p, err := newPoller(f.p.cfg, "", f.out)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.state.NeedsReconcile) != 0 {
		t.Fatal("reconcile did not clear durable gate")
	}
}

func TestUnexpectedRejectionOutputIsUncertain(t *testing.T) {
	for _, message := range []string{
		"unexpected extra line\nError: invalid name\nUsage:\ninvalid name",
		"worker: 0 free\nunexpected extra line\nError: host full (1/1)\nUsage:\nhost full (1/1)",
		"Error: invalid name\nUsage:\ninvalid timeout",
	} {
		f := setup(t)
		f.live(t)
		failure := rejectionError(t, message, 1)
		f.p.command = func(context.Context, ...string) ([]byte, error) { return nil, failure }
		name := lease(f.issue(), f.p.cfg.Status[0], f.entered)
		if err := f.p.launch(context.Background(), f.issue(), f.p.cfg.Status[0], name); err == nil {
			t.Fatal("expected failure")
		}
		if len(f.p.state.Pending) != 1 || f.p.day().Dispatches != 1 {
			t.Fatalf("uncertain output refunded: %q", message)
		}
	}
}

func TestLaunchRepairsExistingTempDirectoryPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix modes only; Windows privacy comes from the state_dir ACL")
	}
	f := setup(t)
	f.live(t)
	dir := filepath.Join(f.p.cfg.StateDir, "tmp")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f.p.command = func(context.Context, ...string) ([]byte, error) {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0700 {
			t.Fatalf("temp directory mode = %o", info.Mode().Perm())
		}
		return nil, nil
	}
	if err := f.p.launch(context.Background(), f.issue(), f.p.cfg.Status[0], "test-1"); err != nil {
		t.Fatal(err)
	}
}
