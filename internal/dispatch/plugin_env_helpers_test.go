package dispatch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// pluginOnlyAbsent reads name's recorded environment and fails if it carries
// a plugin-only name or lost its ordinary environment.
func pluginOnlyAbsent(t *testing.T, dump, name, what string) {
	t.Helper()
	env := readEnvDump(t, dump, name)
	if hasEnv(env, pluginSecret) || hasEnv(env, otherSecret) {
		t.Fatalf("%s received a plugin-only name", what)
	}
	if !hasEnv(env, "PATH") {
		t.Fatalf("%s lost its ordinary environment", what)
	}
}

func testBinaryName() string { return strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") }

// Every non-plugin child of a process holding source.env values drops them:
// the command helper, order diffs, SSH transport, PR lookups for evidence
// and finished workers, board posts and the detached validation-notification
// launcher.
func TestPluginOnlyEnvHelperCommands(t *testing.T) {
	s, dump := pluginEnvFixture(t)
	bin := t.TempDir()
	for _, n := range []string{"git", "gh", "ssh", "board"} {
		installEnvDump(t, bin, n)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx := context.Background()

	if _, e := command("", "git", "--version"); e != nil {
		t.Fatal(e)
	}
	pluginOnlyAbsent(t, dump, "git", "command helper")
	_ = orderDiff("before\n", "after\n")
	pluginOnlyAbsent(t, dump, "git", "order diff")
	_, _ = rpcTransport(ctx, Host{SSH: "worker.example"}, Request{Action: "list"})
	pluginOnlyAbsent(t, dump, "ssh", "SSH transport")
	board := filepath.Join(bin, "board")
	if runtime.GOOS == "windows" {
		board += ".exe"
	}
	_, _ = expediteBoardPost(ctx, board, "board", "post")
	pluginOnlyAbsent(t, dump, "board", "board post")
	launchValidationNotification(t.TempDir(), Meta{Order: Order{Name: "env-notify"}})
	pluginOnlyAbsent(t, dump, testBinaryName(), "validation notification launcher")

	// PR evidence for a run the orchestrator manages.
	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	if e := os.WriteFile(cfg, []byte("[hosts.here]\nlocal = true\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	dir := filepath.Join(Root(), "env-evidence")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "env-evidence", Repo: "app"}}); e != nil {
		t.Fatal(e)
	}
	if e := WriteJSON(filepath.Join(dir, "result.json"), Result{PR: "https://example.com/pull/1", End: time.Now().UTC()}); e != nil {
		t.Fatal(e)
	}
	s.Manages.Runs = RunScope{Names: []string{"env-evidence"}}
	if _, e := GatherOrchestratorEvidence(ctx, s); e != nil {
		t.Fatal(e)
	}
	pluginOnlyAbsent(t, dump, "gh", "PR evidence lookup")

	// A finished worker looks up its PR.
	installEnvDump(t, bin, "claude")
	worker := filepath.Join(Root(), "env-pr")
	if e := os.MkdirAll(filepath.Join(worker, "wt"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := WriteJSON(filepath.Join(worker, "meta.json"), Meta{Start: time.Now().UTC(), ConfigPath: cfg, LaunchResolved: true, Order: Order{Name: "env-pr", Repo: "app", Harness: "claude", Timeout: 1}}); e != nil {
		t.Fatal(e)
	}
	allowChildProbes(t)
	_ = superviseRound("env-pr")
	pluginOnlyAbsent(t, dump, "gh", "worker PR lookup")
}

// The orchestrator supervisor holds plugin-only values outside its own
// environment: even an unfiltered child inherits none, and the declaring
// plugin still receives its own.
func TestHoldPluginOnlyEnv(t *testing.T) {
	s, dump := pluginEnvFixture(t)
	t.Cleanup(func() {
		heldMu.Lock()
		heldEnv = map[string]string{}
		heldMu.Unlock()
	})
	HoldPluginOnlyEnv()
	if _, ok := os.LookupEnv(pluginSecret); ok {
		t.Fatal("supervisor kept a plugin-only name in its environment")
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	if e := exec.Command(exe).Run(); e != nil {
		t.Fatal(e)
	}
	pluginOnlyAbsent(t, dump, testBinaryName(), "unfiltered supervisor child")
	if _, e := execSourcePlugin(context.Background(), s, sourceInvocation{Args: []string{"events"}}); e != nil {
		t.Fatal(e)
	}
	env := readEnvDump(t, dump, "hev-env-source")
	if !slices.Contains(env, pluginSecret+"=source-secret-value") || hasEnv(env, otherSecret) {
		t.Fatal("plugin did not receive exactly its held source.env name")
	}
	// Another orchestrator supervisor it launches keeps the held values.
	if env := controlEnvironment(); !slices.Contains(env, pluginSecret+"=source-secret-value") || !slices.Contains(env, otherSecret+"=other-secret-value") {
		t.Fatal("orchestrator supervisor launch lost held plugin-only values")
	}
}

// The review probe, kept: the general command helper drops source.env names.
func TestReviewNonPluginCommandEnvLeak(t *testing.T) {
	_, dump := pluginEnvFixture(t)
	bin := t.TempDir()
	installEnvDump(t, bin, "git")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, e := command("", "git", "--version"); e != nil {
		t.Fatal(e)
	}
	if hasEnv(readEnvDump(t, dump, "git"), pluginSecret) {
		t.Fatal("non-plugin git exec received source.env secret")
	}
}
