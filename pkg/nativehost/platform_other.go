//go:build !linux && !darwin && !windows

package nativehost

import (
	"net"
	"os"
)

func owner(os.FileInfo) bool { return false }
func peer(net.Conn) error    { return ErrHeld }
func supported() bool        { return false }

func startReceiver(Profile, string) (func() error, error) { return nil, ErrHeld }
