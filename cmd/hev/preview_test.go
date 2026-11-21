//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/preview"
)

func TestPreviewCLILSShowSyncAndServe(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgPath := filepath.Join(home, ".hev", "config.toml")
	t.Setenv("HEV_CONFIG", cfgPath)

	// Create local bare repository.
	repoBase := t.TempDir()
	bare := filepath.Join(repoBase, "docs.git")
	work := filepath.Join(repoBase, "work")
	gitEnv := append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=t",
		"GIT_AUTHOR_EMAIL=t@example.test",
		"GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@example.test",
	)
	runGit := func(dir string, args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = gitEnv
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	runGit(repoBase, "init", "--bare", "-q", "-b", "v0.7", bare)
	runGit(repoBase, "clone", "-q", bare, work)
	if err := os.MkdirAll(filepath.Join(work, "site"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "site", "body.txt"), []byte("hello-cli-preview"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(work, "add", "-A")
	runGit(work, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "initial")
	runGit(work, "push", "-q", "origin", "HEAD:v0.7")
	sha := runGit(work, "rev-parse", "HEAD")

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
		t.Fatal(err)
	}
	cfgTOML := fmt.Sprintf(`
[[preview]]
name = "docs"
bind = "127.0.0.1"
port = %d
repo = %q
ref = "origin/v0.7"
subdir = "site"
build_cmd = ["/bin/sh", "-c", "mkdir -p dist && cp body.txt dist/index.html"]
out_dir = "dist"
keep_builds = 2
timeout = "1m"
env = { PUBLIC_KIT_APP_ORIGIN = "https://app.example.ts.net" }
`, port, bare)
	if err := os.WriteFile(cfgPath, []byte(cfgTOML), 0600); err != nil {
		t.Fatal(err)
	}

	call := func(ctx context.Context, args ...string) (string, error) {
		var out bytes.Buffer
		cmd := newPreviewCommand()
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.ExecuteContext(ctx)
		return out.String(), err
	}

	// ls before sync reports not_built.
	out, err := call(t.Context(), "ls", "--json")
	if err != nil {
		t.Fatalf("preview ls failed: %v\n%s", err, out)
	}
	var rows []preview.Summary
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("decode ls json: %v\n%s", err, out)
	}
	if len(rows) != 1 || rows[0].Name != "docs" || rows[0].State != "not_built" {
		t.Fatalf("unexpected pre-sync ls: %+v", rows)
	}

	// sync builds and publishes.
	out, err = call(t.Context(), "sync", "docs", "--json")
	if err != nil {
		t.Fatalf("preview sync failed: %v\n%s", err, out)
	}
	var syncRes preview.SyncResult
	if err := json.Unmarshal([]byte(out), &syncRes); err != nil {
		t.Fatalf("decode sync json: %v\n%s", err, out)
	}
	if syncRes.Status != "synced" || syncRes.SHA != sha || syncRes.Skipped {
		t.Fatalf("unexpected sync result: %+v", syncRes)
	}

	// Second sync skips unchanged SHA.
	out, err = call(t.Context(), "sync", "docs")
	if err != nil {
		t.Fatalf("second preview sync failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "up_to_date") {
		t.Fatalf("expected up_to_date output, got: %q", out)
	}

	// show --json and text output.
	out, err = call(t.Context(), "show", "docs", "--json")
	if err != nil {
		t.Fatalf("preview show --json failed: %v\n%s", err, out)
	}
	var detail preview.Details
	if err := json.Unmarshal([]byte(out), &detail); err != nil {
		t.Fatalf("decode show json: %v\n%s", err, out)
	}
	if detail.State != "ready" || detail.SHA != sha || detail.Builds != 1 || detail.Current == nil || detail.Current.Schema != preview.SchemaVersion {
		t.Fatalf("unexpected show detail: %+v", detail)
	}

	textShow, err := call(t.Context(), "show", "docs")
	if err != nil || !strings.Contains(textShow, sha) || !strings.Contains(textShow, "ready") {
		t.Fatalf("unexpected text show: err=%v out=%q", err, textShow)
	}
	textLS, err := call(t.Context(), "ls")
	if err != nil || !strings.Contains(textLS, "docs") || !strings.Contains(textLS, "ready") {
		t.Fatalf("unexpected text ls: err=%v out=%q", err, textLS)
	}

	// serve answers requests with security headers and shuts down cleanly.
	srvCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	srvErr := make(chan error, 1)
	go func() {
		_, err := call(srvCtx, "serve", "docs")
		srvErr <- err
	}()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://" + addr + "/")
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("preview serve did not come up: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "hello-cli-preview" {
		t.Fatalf("unexpected serve response: %d %q", resp.StatusCode, string(body))
	}
	if resp.Header.Get("X-Robots-Tag") != preview.HeaderRobotsTag || resp.Header.Get("X-Content-Type-Options") != preview.HeaderContentTypeOptions {
		t.Fatalf("missing security headers: %v", resp.Header)
	}

	cancel()
	select {
	case err := <-srvErr:
		if err != nil {
			t.Fatalf("preview serve exited with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("preview serve did not stop promptly")
	}

	// Verify serve CLI refuses unsafe bind in config.
	badTOML := `
[[preview]]
name = "unsafe"
bind = "0.0.0.0"
port = 4399
root = "/tmp"
`
	if err := os.WriteFile(cfgPath, []byte(badTOML), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t.Context(), "serve", "unsafe"); err == nil {
		t.Fatal("preview serve must refuse 0.0.0.0 bind")
	}
}
