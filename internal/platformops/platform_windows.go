//go:build windows

package platformops

import (
	"errors"
	"os"
)

var errUnsupported = errors.New("UnsupportedCapability: Windows trace writes and daemon lifecycle require native locking, ACL and durability acceptance")

func RequireTraceWrites() error             { return errUnsupported }
func LockFile(*os.File, bool) error         { return errUnsupported }
func UnlockFile(*os.File)                   {}
func LockPath(string, bool) (func(), error) { return nil, errUnsupported }
func IsLockBusy(error) bool                 { return false }
func SignalPID(int, bool) error             { return errUnsupported }
func ProcessGone(error) bool                { return false }
