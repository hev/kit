//go:build linux || darwin

package nativejob

import (
	"errors"
	"os/exec"
)

type unixOwnedCommand struct {
	cmd      *exec.Cmd
	birth    Birth
	birthErr error
}

func platformStartCommand(cmd *exec.Cmd) (ownedCommand, error) {
	err := cmd.Start()
	if cmd.Process == nil {
		return nil, err
	}
	birth, birthErr := processBirth(cmd.Process.Pid)
	return &unixOwnedCommand{cmd, birth, birthErr}, err
}
func (p *unixOwnedCommand) Birth() (Birth, error) { return p.birth, p.birthErr }
func (p *unixOwnedCommand) Wait() (int, error) {
	e := p.cmd.Wait()
	if e != nil {
		var exited *exec.ExitError
		if !errors.As(e, &exited) {
			return 0, e
		}
	}
	if p.cmd.ProcessState == nil {
		return 0, ErrOwnedCommandUnknown
	}
	return p.cmd.ProcessState.ExitCode(), nil
}
func (p *unixOwnedCommand) Kill() error { return p.cmd.Process.Kill() }
