//go:build !linux && !darwin && !windows

package nativehost

import "os"

func platformDaemonLog(string) (*os.File, error) { return nil, ErrHeld }
