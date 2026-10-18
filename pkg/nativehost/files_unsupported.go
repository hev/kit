//go:build !linux && !darwin && !windows

package nativehost

import "os"

func platformPrivate(string, bool) error        { return ErrHeld }
func platformPinned(string, bool) error         { return ErrHeld }
func platformRuntime(Runtime) error             { return ErrHeld }
func openPrivate(string) (*os.File, error)      { return nil, ErrHeld }
func openPinned(string, bool) (*os.File, error) { return nil, ErrHeld }
