package main

import "os/exec"

func runSecretsChild(child *exec.Cmd) error { return child.Run() }
