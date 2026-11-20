//go:build !windows

package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

// Under go test only handlers planted in a temp directory run, so a test
// that forgets to isolate PATH cannot reach a developer's installed
// handlers (hev-hook-slack posts to a live channel).
func TestTestBinaryIgnoresInstalledHandlers(t *testing.T) {
	installed, err := os.MkdirTemp(".", "installed-bin-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(installed) })
	planted := t.TempDir()
	for _, dir := range []string{installed, planted} {
		if err := os.WriteFile(filepath.Join(dir, "hev-hook-"+filepath.Base(dir)), []byte("#!/bin/sh\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	got := userExecutables(installed + string(os.PathListSeparator) + planted)
	if len(got) != 1 || filepath.Dir(got[0].exe) != planted {
		t.Fatalf("handlers = %+v, want only the one in %s", got, planted)
	}
}
