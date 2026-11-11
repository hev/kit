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
