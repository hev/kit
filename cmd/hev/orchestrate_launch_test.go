package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

// A scheduled launch's supervisor restores the caller's context, then holds
// plugin-only values. The hold must see the restored context's specs: here
// only the caller's home declares the source.env name, so holding first would
// leave its value in the supervisor's environment. The held value stays in
// this test process, where no supervisor reads it.
func TestScheduledLaunchRestoresContextBeforeHold(t *testing.T) {
	const secret = "KIT_TEST_LAUNCH_SECRET"
	profile, caller := t.TempDir(), t.TempDir()
	for _, k := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(k, caller)
	}
	spec := strings.Replace(sourceSpecForCLI, "source: {plugin: fake,", "source: {plugin: fake, env: ["+secret+"],", 1)
	if _, e := dispatch.ApplyOrchestrator(spec, false); e != nil {
		t.Fatal(e)
	}
	for _, k := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(k, profile)
	}
	t.Setenv(secret, "synthetic-secret")
	old := superviseOrchestrator
	t.Cleanup(func() { superviseOrchestrator = old })
	var home string
	held := true
	superviseOrchestrator = func(_ context.Context, name string, _ dispatch.OrchestratorDriver) error {
		home = os.Getenv("HOME")
		_, held = os.LookupEnv(secret)
		return nil
	}
	cmd := newOrchestrateCommand()
	cmd.SetArgs([]string{"supervise", "intake", "--launch-env", "HOME=" + caller, "--launch-env", "USERPROFILE=" + caller})
	if e := cmd.Execute(); e != nil {
		t.Fatal(e)
	}
	if home != caller {
		t.Fatalf("supervisor ran with HOME %q, want the carried %q", home, caller)
	}
	if held {
		t.Fatal("supervisor kept a plugin-only value: it held before restoring the caller's context")
	}
}
