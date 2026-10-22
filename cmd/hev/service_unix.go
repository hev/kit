//go:build !windows

package main

import (
	"context"

	"github.com/hev/kit/internal/daemon"
)

func windowsService(string) error               { panic("Windows service on Unix") }
func nativeDaemonPIDs() []int                   { return nil }
func nativeDaemonRun(ctx context.Context) error { return daemon.Run(ctx) }
