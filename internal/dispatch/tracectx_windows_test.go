package dispatch

import "testing"

// The Windows CI selector runs Windows-prefixed tests only; this covers the
// detached-process launch path's worker environment.
func TestWindowsWorkerTraceEnv(t *testing.T) { testWorkerTraceEnv(t) }
