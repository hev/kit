//go:build windows

package nativehost

import (
	"context"
	"net"
)

// Windows owns this backend; the shared loop never falls back to Unix IPC.
// Keep every entry held until authenticated protected-pipe proofs exist.
func validateEndpoint(string) error                                      { return ErrHeld }
func openEndpoint(context.Context, string, Config) (net.Listener, error) { return nil, ErrHeld }
func verifyListener(net.Listener) error                                  { return ErrHeld }
func dialEndpoint(context.Context, string) (net.Conn, error)             { return nil, ErrHeld }
func readRequest(net.Conn) ([]byte, error)                               { return nil, ErrHeld }
func writeRequest(net.Conn, []byte) error                                { return ErrHeld }
