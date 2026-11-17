//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/update"
)

func windowsService(string) error                   { panic("Windows service on Unix") }
func nativeDaemonPIDs() []int                       { return nil }
func windowsUpdateServices(string) []update.Service { return nil }

// nativeDaemonRun runs the daemon. When it stops to move onto another build
// it re-executes from the path it was started as, which now names that
// build: same process, so the service manager sees no restart.
func nativeDaemonRun(ctx context.Context) error {
	err := daemon.Run(ctx)
	if !errors.Is(err, daemon.ErrRestart) {
		return err
	}
	path := update.Invoked()
	if path == "" {
		return err
	}
	if e := syscall.Exec(path, os.Args, os.Environ()); e != nil {
		return fmt.Errorf("%w: re-execute %s: %v", err, path, e)
	}
	return nil
}
