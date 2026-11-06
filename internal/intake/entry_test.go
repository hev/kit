package intake

import "io"

// Package tests exercise the shared implementation; Cobra routing is tested in cmd/hev.
func runWatchTest(args []string, out io.Writer) error { return Run(args, out) }
