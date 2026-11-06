package businessflow

import (
	"errors"
	"os"
)

var errPlatform = errors.New("UnsupportedCapability: businessflow Windows authority/atomic snapshot not implemented")

func mutationSupported() error                  { return errPlatform }
func lockFile(*os.File) error                   { return errPlatform }
func unlockFile(*os.File) error                 { return errPlatform }
func openSnapshot(string) (*os.File, error)     { return nil, errPlatform }
func openExistingLock(string) (*os.File, error) { return nil, errPlatform }
