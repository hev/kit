//go:build windows

package subscriptions

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"time"
)

// systemKeychain: Claude Code keeps its login in a file off macOS.
func systemKeychain(context.Context) ([]byte, error) {
	return nil, errors.New("no keychain on this platform")
}

// systemAgyKeychain checks Windows Credential Manager target names through
// cmdkey /list without reading or returning any credential blob.
func systemAgyKeychain(ctx context.Context) (bool, error) {
	for _, prefix := range []string{"antigravity-oauth-token", "jetski-standalone-oauth-token"} {
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		cmd := exec.CommandContext(cctx, "cmdkey.exe", "/list:"+prefix+"*")
		var out bytes.Buffer
		cmd.Stdout = &limitedWriter{w: &out, max: 64 << 10}
		cmd.Stderr = io.Discard
		cmd.WaitDelay = time.Second
		err := cmd.Run()
		cancel()
		if err == nil && hasWindowsCredentialTarget(out.String(), prefix) {
			return true, nil
		}
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
	}
	return false, nil
}
