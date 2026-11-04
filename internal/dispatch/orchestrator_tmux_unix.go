//go:build !windows

package dispatch

import (
	"fmt"
	"os"
)

// The supervisor occupies the same pane during rounds and rest. While resting
// it blocks on filesystem controls and a deadline, with no shell polling loop.
func launchOrchestratorTmux(exe, name string) (bool, error) {
	s, err := LoadOrchestrator(name)
	if err != nil {
		return true, err
	}
	args := []string{"new-session", "-d", "-s", "o-" + name, "-c", OrchestratorDir(name)}
	// A pre-existing tmux server may carry another HOME or identity.
	for _, key := range []string{"HOME", "USERPROFILE", "PATH", "HEV_DISPATCH_CONFIG"} {
		if v, ok := os.LookupEnv(key); ok {
			args = append(args, "-e", key+"="+v)
		}
	}
	command := "unset HEV_ORCHESTRATOR; exec " + Quote(exe) + " orchestrate supervise " + Quote(name)
	args = append(args, command, ";", "set-option", "-t", "o-"+name, "@peers-role", "orchestrator", ";", "select-pane", "-t", "=o-"+name+":", "-T", s.Goal)
	c := tmuxCommand(args...)
	c.Env = orchestratorEnvironment("")
	if out, e := c.CombinedOutput(); e != nil {
		return true, fmt.Errorf("orchestrator tmux: %w: %s", e, out)
	}
	return true, nil
}
