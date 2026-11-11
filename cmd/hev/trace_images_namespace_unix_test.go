//go:build !windows

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestLayerNamespaceOutsideArchiveCharsetReachesLayer is the repro for a
// regression in image capture: with a Layer key, `hev traces ls --namespace
// team:traces` must query Layer as it did before images were captured, not
// fail locally because the namespace cannot be a directory name.
func TestLayerNamespaceOutsideArchiveCharsetReachesLayer(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the hev binary")
	}
	tr := newTrap(t)
	bin := buildHev(t)
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, ".hev"))
	env := append(noKeyEnv(home, tr), "HEVLAYER_API_KEY=fake")
	cmd := exec.Command(bin, "traces", "ls", "--namespace", "team:traces")
	cmd.Env = env
	out, _ := cmd.CombinedOutput() // the trap answers 418, so the command fails; where it fails is the point
	if strings.Contains(string(out), "must be letters, digits") {
		t.Fatalf("failed locally on the namespace:\n%s", out)
	}
	tr.mu.Lock()
	requests := append([]string(nil), tr.requests...)
	tr.mu.Unlock()
	want := "POST " + strings.TrimPrefix(tr.server.URL, "http://") + "/v2/namespaces/team:traces-sessions/query"
	for _, r := range requests {
		if r == want {
			return
		}
	}
	t.Fatalf("no %q in %v; output:\n%s", want, requests, out)
}
