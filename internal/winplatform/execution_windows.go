//go:build windows

// Package winplatform scopes the controlled native execution candidate to the
// kernel/architecture with actual owned filesystem, IPC and Job evidence.
package winplatform

import (
	"golang.org/x/sys/windows"
	"runtime"
)

func ExecutionBuild() bool {
	v := windows.RtlGetVersion()
	return v != nil && runtime.GOARCH == "amd64" && v.MajorVersion == 10 && v.MinorVersion == 0 && v.BuildNumber == 26300
}
