package main

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

func runPlugin(path string, args []string, in io.Reader, out, errout io.Writer) (int, error) {
	c := exec.Command(path, args...)
	if strings.EqualFold(filepath.Ext(path), ".cmd") {
		line, err := cmdPluginLine(path, args)
		if err != nil {
			return 1, err
		}
		c = exec.Command("cmd.exe")
		c.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd.exe /d /s /c " + line}
	}
	c.Stdin, c.Stdout, c.Stderr = in, out, errout
	// The child shares our console and receives Ctrl-C from Windows directly.
	// Keep the parent alive to wait for and propagate the child's exit status.
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	err := c.Run()
	if e, ok := err.(*exec.ExitError); ok {
		code := e.ExitCode()
		if uint32(code) == 0xc000013a || code < 0 {
			code = 130
		}
		return code, nil
	}
	return 0, err
}
