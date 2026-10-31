//go:build !windows

package main

import (
	"io"
	"os"
	"os/exec"
	"syscall"
)

func runPlugin(path string, args []string, in io.Reader, out, errout io.Writer) (int, error) {
	if in == os.Stdin && out == os.Stdout && errout == os.Stderr {
		return 1, syscall.Exec(path, append([]string{path}, args...), os.Environ())
	}
	c := exec.Command(path, args...)
	c.Stdin, c.Stdout, c.Stderr = in, out, errout
	err := c.Run()
	if e, ok := err.(*exec.ExitError); ok {
		if s, ok := e.Sys().(syscall.WaitStatus); ok && s.Signaled() {
			return 128 + int(s.Signal()), nil
		}
		return e.ExitCode(), nil
	}
	return 0, err
}
