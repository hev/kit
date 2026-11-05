//go:build !windows

package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSourcePluginTimeoutKillsGrandchild(t *testing.T) {
	testSourceTimeoutKillsTree(t, 4*time.Second)
}

func TestSourcePluginExecEnvArgsAndCappedStderr(t *testing.T) {
	s, _ := installHangSource(t)
	s.Source.Args = []string{"--config", "fake.toml"}
	out, e := execSourcePlugin(context.Background(), s, sourceInvocation{Args: sourceArgs(s, "echo", "--team", "core"), Env: []string{"HEV_SOURCE_CURSOR=" + `a b "c" $(x)`}})
	if e != nil {
		t.Fatal(e)
	}
	if got := strings.TrimSpace(string(out)); got != `cursor=a b "c" $(x) args=echo --config fake.toml --team core` {
		t.Fatal(got)
	}
	log, _ := os.ReadFile(filepath.Join(OrchestratorDir(s.Name), "supervisor.log"))
	if len(log) > sourceStderrCap+8192 || !strings.Contains(string(log), "[stderr truncated at") || !strings.Contains(string(log), "source hang-source echo: stderr filler line 0\n") {
		t.Fatal("stderr not capped", len(log))
	}
}
