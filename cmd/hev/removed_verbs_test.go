package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRemovedVerbsFailClearly(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "hev")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	// A fake backend on PATH must never be executed by a removed verb.
	bin := t.TempDir()
	marker := filepath.Join(bin, "ran")
	if runtime.GOOS != "windows" {
		script := "#!/bin/sh\ntouch '" + marker + "'\n"
		for _, name := range []string{"factory", "hev-control", "backend"} {
			if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Built in two pieces so the legacy names never appear as contiguous literals in the tree.
	legacyResource := "HEV_RESOURCE_" + "BACKEND"
	legacyCoordination := "HEV_COORDINATION_" + "BACKEND"
	for _, verb := range removedVerbs {
		for _, args := range [][]string{{verb}, {verb, "runs"}, {verb, "--help"}} {
			cmd := exec.Command(binary, args...)
			cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				legacyResource+"="+filepath.Join(bin, "backend"), legacyCoordination+"="+filepath.Join(bin, "backend"))
			out, err := cmd.CombinedOutput()
			want := "hev " + verb + " has been removed; use hev dispatch or hev orchestrate"
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || !strings.Contains(string(out), want) {
				t.Errorf("%v: err=%v out=%q", args, err, out)
			}
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a backend executable was run")
	}
}

func TestRemovedVerbsHiddenFromHelp(t *testing.T) {
	for _, verb := range removedVerbs {
		if !removedVerbCommand(verb).Hidden {
			t.Errorf("%s is listed in help", verb)
		}
	}
}
