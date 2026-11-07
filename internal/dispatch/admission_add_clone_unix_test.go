//go:build !windows

package dispatch

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/gittest"
)

// A real clone over HTTPS uses only the host's stored credential and trust,
// never runs host hooks, never recurses submodules and stays bounded.
func TestSafeCloneUsesHostCredentialsAndHardening(t *testing.T) {
	srv := gittest.Serve(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	srv.HostConfig(t, home)
	// Host-wide settings that a hostile clone could otherwise ride on.
	hooks := filepath.Join(home, "hooks")
	marker := filepath.Join(home, "hook-ran")
	if err := os.MkdirAll(hooks, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	extra := "[core]\n\thooksPath = " + hooks + "\n[submodule]\n\trecurse = true\n[protocol \"file\"]\n\tallow = always\n"
	f, err := os.OpenFile(filepath.Join(home, ".gitconfig"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(extra)
	f.Close()
	sub := filepath.Join(t.TempDir(), "sub")
	if _, err = command("", "git", "init", "-q", sub); err != nil {
		t.Fatal(err)
	}
	srv.Repo(t, "ExampleOrg/widget.git", map[string]string{
		"README.md":   "widget\n",
		".gitmodules": "[submodule \"evil\"]\n\tpath = evil\n\turl = file://" + sub + "\n",
	})
	url := srv.URL + "/ExampleOrg/widget.git"
	dest := filepath.Join(t.TempDir(), "widget")
	if err = safeClone(context.Background(), url, dest); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(marker); err == nil {
		t.Fatal("host hook ran during clone")
	}
	if b, _ := os.ReadFile(filepath.Join(dest, ".git", "config")); strings.Contains(string(b), "hooksPath") || strings.Contains(string(b), "submodule") {
		t.Fatalf("clone persisted transient settings: %s", b)
	}
	if entries, _ := os.ReadDir(filepath.Join(dest, "evil")); len(entries) != 0 {
		t.Fatal("submodule was cloned")
	}
	if entries, _ := os.ReadDir(filepath.Join(dest, ".git", "hooks")); len(entries) != 0 {
		t.Fatalf("template hooks installed: %v", entries)
	}

	for _, tc := range []struct {
		name string
		url  string
		prep func()
		want string
	}{
		{"redirect", srv.URL + "/redirect/ExampleOrg/widget.git", nil, "redirect"},
		{"file transport", "file://" + sub, nil, "transport 'file' not allowed"},
		{"no credential", url, func() {
			os.Remove(filepath.Join(home, ".git-credentials"))
		}, "terminal prompts disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.prep != nil {
				tc.prep()
			}
			done := make(chan error, 1)
			go func() { done <- safeClone(context.Background(), tc.url, filepath.Join(t.TempDir(), "x")) }()
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("got %v", err)
				}
			case <-time.After(time.Minute):
				t.Fatal("clone waited for input")
			}
		})
	}
}

func TestSafeCloneBounds(t *testing.T) {
	srv := gittest.Serve(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv.HostConfig(t, home)
	blob := make([]byte, 1<<20)
	rand.Read(blob)
	srv.Repo(t, "ExampleOrg/big.git", map[string]string{"blob": string(blob)})
	url := srv.URL + "/ExampleOrg/big.git"
	previousSize, previousTime := addRepoMaxBytes, addRepoTimeout
	t.Cleanup(func() { addRepoMaxBytes, addRepoTimeout = previousSize, previousTime })

	addRepoMaxBytes = 64 << 10
	if err := safeClone(context.Background(), url, filepath.Join(t.TempDir(), "a")); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("size bound: %v", err)
	}
	addRepoMaxBytes, addRepoTimeout = 1<<30, time.Millisecond
	if err := safeClone(context.Background(), url, filepath.Join(t.TempDir(), "b")); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("time bound: %v", err)
	}
}
