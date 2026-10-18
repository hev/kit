//go:build windows

package platformops

import (
	"os"
)

var errUnsupported = ErrTraceWritesUnsupported

func RequireTraceWrites() error             { return errUnsupported }
func LockFile(*os.File, bool) error         { return errUnsupported }
func UnlockFile(*os.File)                   {}
func LockPath(string, bool) (func(), error) { return nil, errUnsupported }
func IsLockBusy(error) bool                 { return false }
func SignalPID(int, bool) error             { return errUnsupported }
func ProcessGone(error) bool                { return false }
