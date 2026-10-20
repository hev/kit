//go:build linux || darwin

package nativehost

import (
	"io"
	"os/exec"
)

// startReceiver constructs only the fixed pinned receiver invocation. It never
// accepts replacement argv or adopts a PID. The returned Wait owns this Cmd.
func startReceiver(profile Profile, descriptorRef string) (func() error, error) {
	cmd := exec.Command(profile.SupervisorRef, "--config", profile.ConfigRef, "--supervise", descriptorRef)
	cmd.Dir = profile.Runtime.Directory
	cmd.Env = append([]string(nil), profile.Runtime.Environment...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Wait, nil
}
