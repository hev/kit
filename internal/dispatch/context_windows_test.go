package dispatch

import "testing"

// TestWindowsContextLifecycle runs the recipe lifecycle on the Windows launch
// path: job-object supervisor start and harness standard input.
func TestWindowsContextLifecycle(t *testing.T) { runContextLifecycle(t) }
