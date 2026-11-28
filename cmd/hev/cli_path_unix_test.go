//go:build !windows

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/version"
)

func TestTopLevelCommandsIgnorePATHExecutables(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "executed")
	for _, name := range []string{"hev-foo", "hev-version"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nprintf 'executed\\n' > executed\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", dir)
	t.Setenv("HEV_TEST_AS_HEV", "1")
	t.Setenv("HEV_TEST_ARGV_LOG", "")

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("hev %v created the execution marker or it could not be checked: %v", args, statErr)
		}
		return string(out), err
	}

	out, err := run("foo")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || !strings.Contains(out, `unknown command "foo" for "hev"`) {
		t.Fatalf("hev foo = %q, %v; want Cobra's unknown command error and exit 1", out, err)
	}

	out, err = run("--help")
	if err != nil || strings.Contains(out, "Plugins:") || strings.Contains(out, "foo") || !strings.Contains(out, "Other commands:") {
		t.Fatalf("hev --help = %q, %v; want built-in help without PATH entries", out, err)
	}

	out, err = run("version")
	if err != nil || out != version.String()+"\n" {
		t.Fatalf("hev version = %q, %v; want built-in version", out, err)
	}
}
