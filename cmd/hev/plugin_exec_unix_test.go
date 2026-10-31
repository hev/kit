//go:build !windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginExecHelper(t *testing.T) {
	if os.Getenv("PLUGIN_EXEC_HELPER") != "yes" {
		return
	}
	_, err := runPlugin(os.Getenv("PLUGIN_EXEC_PATH"), []string{"argument"}, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		os.Exit(99)
	}
	os.Exit(98)
}

func TestPluginReplacesProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugin")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s:' \"$$\"\n/bin/cat\nprintf '%s' \"$1\"\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(os.Args[0], "-test.run=^TestPluginExecHelper$")
	c.Env = append(os.Environ(), "PLUGIN_EXEC_HELPER=yes", "PLUGIN_EXEC_PATH="+path)
	c.Stdin = strings.NewReader("stdin:")
	var out bytes.Buffer
	c.Stdout = &out
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	pid := c.Process.Pid
	err := c.Wait()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 7 || out.String() != fmt.Sprintf("%d:stdin:argument", pid) {
		t.Fatalf("%q %v", out.String(), err)
	}
}

func TestPluginSignalExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugin")
	os.WriteFile(path, []byte("#!/bin/sh\nkill -TERM $$\n"), 0700)
	var out bytes.Buffer
	code, err := runPlugin(path, nil, strings.NewReader(""), &out, &out)
	if err != nil || code != 143 {
		t.Fatalf("%d %v", code, err)
	}
}
