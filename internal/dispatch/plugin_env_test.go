package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	pluginSecret = "KIT_TEST_SOURCE_SECRET"
	otherSecret  = "KIT_TEST_OTHER_SECRET"
)

// installEnvDump copies this test binary onto PATH as name; it records its
// environment in the dump directory and exits (see TestMain).
func installEnvDump(t *testing.T, bin, name string) {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(exe)
	if e != nil {
		t.Fatal(e)
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if e = os.WriteFile(filepath.Join(bin, name), b, 0700); e != nil {
		t.Fatal(e)
	}
}

func readEnvDump(t *testing.T, dump, name string) []string {
	t.Helper()
	p := filepath.Join(dump, name+".env")
	var b []byte
	var e error
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if b, e = os.ReadFile(p); e == nil {
			_ = os.Remove(p)
			return strings.Split(string(b), "\n")
		}
	}
	t.Fatalf("%s never recorded its environment: %v", name, e)
	return nil
}

func hasEnv(env []string, name string) bool {
	return slices.ContainsFunc(env, func(v string) bool {
		k, _, _ := strings.Cut(v, "=")
		return strings.EqualFold(k, name)
	})
}

// pluginEnvFixture applies a spec whose source.env names pluginSecret and
// another whose source.env names otherSecret, both set in this process.
func pluginEnvFixture(t *testing.T) (OrchestratorSpec, string) {
	t.Helper()
	orchHome(t)
	text := strings.Replace(orchestratorExample, "source: none", "source:\n  plugin: env-source\n  env: ["+pluginSecret+"]", 1)
	s, e := ApplyOrchestrator(text, false)
	if e != nil {
		t.Fatal(e)
	}
	other := strings.Replace(strings.Replace(orchestratorExample, "name: release-train", "name: other", 1), "source: none", "source:\n  plugin: other-source\n  env: ["+otherSecret+"]", 1)
	if _, e = ApplyOrchestrator(other, false); e != nil {
		t.Fatal(e)
	}
	bin, dump := t.TempDir(), t.TempDir()
	for _, n := range []string{"hev-env-source", "claude"} {
		installEnvDump(t, bin, n)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(pluginSecret, "source-secret-value")
	t.Setenv(otherSecret, "other-secret-value")
	t.Setenv("DISPATCH_ENV_DUMP", dump)
	return s, dump
}

// testPluginOnlyEnv covers the plugin exec and every shared child path: the
// round harness, the worker harness and notification senders.
func testPluginOnlyEnv(t *testing.T) (OrchestratorSpec, string) {
	s, dump := pluginEnvFixture(t)
	allowChildProbes(t)
	if _, e := execSourcePlugin(context.Background(), s, sourceInvocation{Args: []string{"events"}}); e != nil {
		t.Fatal(e)
	}
	env := readEnvDump(t, dump, "hev-env-source")
	if !slices.Contains(env, pluginSecret+"=source-secret-value") {
		t.Fatal("plugin did not receive its source.env name")
	}
	if hasEnv(env, otherSecret) {
		t.Fatal("plugin received another spec's source.env name")
	}

	s.Harness = "claude"
	// The session's exit races its harness record; only the environment matters.
	_ = orchestratorSession(context.Background(), s, t.TempDir(), OrchestratorPacket{})
	env = readEnvDump(t, dump, "claude")
	if hasEnv(env, pluginSecret) || hasEnv(env, otherSecret) {
		t.Fatal("round harness received a plugin-only name")
	}
	if !hasEnv(env, "PATH") {
		t.Fatal("round harness lost its ordinary environment")
	}

	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	if e := os.WriteFile(cfg, nil, 0600); e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(Root(), "env-worker")
	if e := os.MkdirAll(filepath.Join(dir, "wt"), 0700); e != nil {
		t.Fatal(e)
	}
	// A host env entry naming the secret cannot forward it to a worker.
	m := Meta{Start: time.Now().UTC(), ConfigPath: cfg, LaunchResolved: true, LaunchEnv: []string{pluginSecret, "DISPATCH_ENV_DUMP"}, Order: Order{Name: "env-worker", Repo: "app", Harness: "claude", Timeout: 1}}
	if e := WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
		t.Fatal(e)
	}
	if launched, e := launchEnvironment(dir); e != nil || hasEnv(launched, pluginSecret) {
		t.Fatalf("launch environment forwards a plugin-only name: %v", e)
	}
	_ = superviseRound("env-worker")
	env = readEnvDump(t, dump, "claude")
	if hasEnv(env, pluginSecret) || hasEnv(env, otherSecret) {
		t.Fatal("worker harness received a plugin-only name")
	}
	if hasEnv(notificationEnvironment(), pluginSecret) {
		t.Fatal("notification sender received a plugin-only name")
	}
	return s, dump
}

func TestSourceEnvSpecValidation(t *testing.T) {
	for _, env := range []string{"[PATH]", "[\"1BAD\"]", "[A, a]", "[\"A=B\"]", "[HEV_ORCHESTRATOR]"} {
		text := strings.Replace(orchestratorExample, "source: none", "source:\n  plugin: env-source\n  env: "+env, 1)
		if _, e := ParseOrchestrator(text); e == nil {
			t.Fatalf("accepted source.env %s", env)
		}
	}
	if _, e := ParseOrchestrator(strings.Replace(orchestratorExample, "source: none", "source:\n  env: [TOKEN]", 1)); e == nil {
		t.Fatal("accepted source.env without a plugin")
	}
	s, e := ParseOrchestrator(strings.Replace(orchestratorExample, "source: none", "source:\n  plugin: env-source\n  env: [SOURCE_TOKEN]", 1))
	if e != nil || !slices.Equal(s.Source.Env, []string{"SOURCE_TOKEN"}) {
		t.Fatal(s.Source.Env, e)
	}
}
