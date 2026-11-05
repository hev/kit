package dispatch

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeLiveLaunches records a supervisor owned by the test process, so
// launched runs stay live until end removes the record.
func fakeLiveLaunches(t *testing.T) func(string) {
	t.Helper()
	old := launch
	t.Cleanup(func() { launch = old })
	launch = func(dir, _ string) error { return writePID(dir) }
	return func(name string) {
		if e := os.Remove(filepath.Join(Root(), name, "supervisor.json")); e != nil {
			t.Fatal(e)
		}
		if e := WriteJSON(filepath.Join(Root(), name, "result.json"), Result{ExitCode: 0, End: timeNowUTC()}); e != nil {
			t.Fatal(e)
		}
	}
}
