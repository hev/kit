//go:build !windows

package dispatch

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	lfsMarker := filepath.Join(home, "lfs-ran")
	extra := "[core]\n\thooksPath = " + hooks + "\n[submodule]\n\trecurse = true\n[protocol \"file\"]\n\tallow = always\n" +
		// A URL-scoped redirect setting must not re-enable redirects.
		"[http \"" + srv.URL + "/\"]\n\tfollowRedirects = true\n" +
		// A host LFS filter must not run (and fetch from a repo's .lfsconfig) during the clone.
		"[filter \"lfs\"]\n\tsmudge = touch '" + lfsMarker + "'; cat\n\trequired = true\n"
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
		"README.md":      "widget\n",
		".gitmodules":    "[submodule \"evil\"]\n\tpath = evil\n\turl = file://" + sub + "\n",
		".gitattributes": "*.bin filter=lfs\n",
		".lfsconfig":     "[lfs]\n\turl = https://elsewhere.example.test/lfs\n",
		"data.bin":       "version https://git-lfs.github.com/spec/v1\n",
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
	if _, err = os.Stat(lfsMarker); err == nil {
		t.Fatal("LFS filter ran during clone")
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

// A credential helper that blocks, with a grandchild holding git's output
// open, cannot hold the clone past its timeout, and nothing git started
// survives it.
func TestSafeCloneKillsBlockedHelpers(t *testing.T) {
	srv := gittest.Serve(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	srv.HostConfig(t, home)
	os.Remove(filepath.Join(home, ".git-credentials"))
	pids := filepath.Join(home, "pids")
	helper := filepath.Join(home, "helper.sh")
	script := "#!/bin/sh\nsleep 300 &\necho $$ $! >> '" + pids + "'\nwait\n"
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(home, ".gitconfig"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(f, "[credential]\n\thelper = %s\n", helper)
	f.Close()
	srv.Repo(t, "ExampleOrg/widget.git", map[string]string{"README.md": "widget\n"})
	previousTime, previousDelay := addRepoTimeout, cloneWaitDelay
	t.Cleanup(func() { addRepoTimeout, cloneWaitDelay = previousTime, previousDelay })
	addRepoTimeout, cloneWaitDelay = 2*time.Second, time.Second

	start := time.Now()
	done := make(chan error, 1)
	go func() {
		done <- safeClone(context.Background(), srv.URL+"/ExampleOrg/widget.git", filepath.Join(t.TempDir(), "x"))
	}()
	select {
	case err = <-done:
		if err == nil || !strings.Contains(err.Error(), "exceeded") {
			t.Fatalf("got %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("clone hung past its timeout")
	}
	if elapsed := time.Since(start); elapsed > addRepoTimeout+cloneWaitDelay+3*time.Second {
		t.Fatalf("clone returned after %s", elapsed)
	}
	b, _ := os.ReadFile(pids)
	fields := strings.Fields(string(b))
	if len(fields) < 2 {
		t.Fatalf("helper did not run: %q", b)
	}
	for _, field := range fields {
		pid, _ := strconv.Atoi(field)
		deadline := time.Now().Add(5 * time.Second)
		for processAlive(pid) {
			if time.Now().After(deadline) {
				t.Fatalf("helper process %d survived the clone", pid)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}
