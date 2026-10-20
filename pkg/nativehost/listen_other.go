//go:build !linux && !darwin && !windows

package nativehost

import (
	"context"
	"net"
)

func validateEndpoint(string) error                                      { return ErrHeld }
func openEndpoint(context.Context, string, Config) (net.Listener, error) { return nil, ErrHeld }
func verifyListener(net.Listener) error                                  { return ErrHeld }
func dialEndpoint(context.Context, string) (net.Conn, error)             { return nil, ErrHeld }
func readRequest(net.Conn) ([]byte, error)                               { return nil, ErrHeld }
func writeRequest(net.Conn, []byte) error                                { return ErrHeld }
