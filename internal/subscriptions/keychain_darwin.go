package subscriptions

import (
	"context"
	"os/exec"
	"time"
)

// systemKeychain reads the Claude Code credential item from the login
// keychain. The secret arrives on stdout; nothing secret is an argument.
func systemKeychain(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output()
}

// systemAgyKeychain checks whether an Antigravity OAuth item exists in the
// macOS login keychain. Omitting -w checks item metadata only and never reads
// or prints secret bytes.
func systemAgyKeychain(ctx context.Context) (bool, error) {
	for _, svc := range []string{"antigravity-oauth-token", "jetski-standalone-oauth-token"} {
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		cmd := exec.CommandContext(cctx, "/usr/bin/security", "find-generic-password", "-s", svc)
		cmd.WaitDelay = time.Second
		err := cmd.Run()
		cancel()
		if err == nil {
			return true, nil
		}
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
	}
	return false, nil
}
