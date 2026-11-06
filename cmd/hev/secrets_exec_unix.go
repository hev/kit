//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func runSecretsChild(child *exec.Cmd) error {
	path, err := exec.LookPath(child.Path)
	if err != nil {
		return err
	}
	return syscall.Exec(path, child.Args, child.Env)
}
