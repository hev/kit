//go:build !windows

package dispatch

import (
	"os"
	"os/exec"
	"path/filepath"
)

func startOrchestratorSupervisor(dir, name string, args []string) error {
	c := exec.Command(args[0], args[1:]...)
	c.Env = orchestratorEnvironment("") // Code gathering uses the child spec; sessions get its identity.
	setupChild(c)
	f, e := os.OpenFile(filepath.Join(dir, "supervisor.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	c.Stdout = f
	c.Stderr = f
	if e = c.Start(); e != nil {
		return e
	}
	go c.Wait()
	return nil
}
