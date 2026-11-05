package dispatch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWindowsHostDispatchPause(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	if runtime.GOOS != "windows" {
		bin := t.TempDir()
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := Handle(Request{Action: "pause"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, action := range []string{"start", "rework", "retry"} {
		_, err := Handle(Request{Action: action, Name: "example", Order: "invalid"})
		if err == nil || !strings.Contains(err.Error(), "host dispatch paused") {
			t.Fatalf("%s: %v", action, err)
		}
	}
	if _, err := checkAmendAdmission(Request{}, Meta{}, nil, false); err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("amend: %v", err)
	}
	if _, err := os.Stat(filepath.Join(Root(), "example")); !os.IsNotExist(err) {
		t.Fatalf("pause created run: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := Handle(Request{Action: "resume"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkHostPause(); err != nil {
		t.Fatal(err)
	}
}

func TestHostPauseStateFailsClosed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	if err := os.MkdirAll(filepath.Join(Root(), ".paused", "child"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := checkHostPause(); err == nil {
		t.Fatal("directory pause marker ignored")
	}
	if err := controlHostPause("resume"); err == nil {
		t.Fatal("nonempty marker silently removed")
	}
}
