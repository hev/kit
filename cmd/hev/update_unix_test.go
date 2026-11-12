//go:build !windows

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hev/kit/internal/update"
	"github.com/hev/kit/internal/version"
)

const (
	cmdEdgeA = "0.7.0-edge.aaaaaaa"
	cmdEdgeB = "0.7.0-edge.bbbbbbb"
)

// fakeBuilds serves edge builds A (sequence 1) and B (2) the way the edge
// workflow publishes them, and points updateSource at it.
func fakeBuilds(t *testing.T) {
	t.Helper()
	files := map[string][]byte{}
	var releases []map[string]any
	for i, v := range []string{cmdEdgeA, cmdEdgeB} {
		bin := []byte("#!/bin/sh\necho " + v + "\n")
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		tw.WriteHeader(&tar.Header{Name: "hev", Mode: 0o755, Size: int64(len(bin)), Typeflag: tar.TypeReg})
		tw.Write(bin)
		tw.Close()
		gz.Close()
		name := update.AssetName(v, runtime.GOOS, runtime.GOARCH)
		sum := sha256.Sum256(buf.Bytes())
		m, _ := json.Marshal(update.Manifest{Schema: 1, Version: v, Channel: "edge", Sequence: i + 1,
			Assets: []update.Asset{{Name: name, OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: hex.EncodeToString(sum[:])}}})
		tag := update.Tag(v)
		files[tag+"/"+name], files[tag+"/manifest.json"] = buf.Bytes(), m
		releases = append([]map[string]any{{"tag_name": tag, "prerelease": true, "published_at": "2026-11-0" + string(rune('1'+i)) + "T00:00:00Z", "assets": []map[string]string{{"name": "manifest.json"}}}}, releases...)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/releases", func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(releases) })
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[strings.TrimPrefix(r.URL.Path, "/download/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	saved := updateSource
	updateSource = update.Source{API: srv.URL + "/api", Download: srv.URL + "/download"}
	t.Cleanup(func() { updateSource = saved })
}

type recordingService struct{ calls int }

func (s *recordingService) Name() string                  { return "hevd (fake)" }
func (s *recordingService) Present() bool                 { return true }
func (s *recordingService) Restart(context.Context) error { s.calls++; return nil }

// updateSandbox runs this host as edge build A from a temporary install with
// a fake service manager, never the test binary or this machine's services.
func updateSandbox(t *testing.T) (link string, svc *recordingService) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, ".hev", "config.toml"))
	link = filepath.Join(t.TempDir(), "hev")
	os.WriteFile(link, []byte("#!/bin/sh\necho "+cmdEdgeA+"\n"), 0o755)
	svc = &recordingService{}
	savedAdjust, savedVersion := adjustUpdater, version.Version
	adjustUpdater = func(u *update.Updater) {
		u.Invoked, u.Exe, u.Extra, u.Services = link, link, nil, []update.Service{svc}
	}
	version.Version = cmdEdgeA
	t.Cleanup(func() { adjustUpdater, version.Version = savedAdjust, savedVersion })
	fakeBuilds(t)
	return link, svc
}

func runsAs(t *testing.T, link string) string {
	t.Helper()
	out, err := exec.Command(link).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestHevUpdateCommand(t *testing.T) {
	link, svc := updateSandbox(t)
	run := func(args ...string) (string, error) {
		c := newUpdateCommand()
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetErr(&out)
		c.SetArgs(args)
		err := c.Execute()
		return out.String(), err
	}
	// Default channel is release: with no release published, nothing moves.
	if _, err := run("--check"); err == nil {
		t.Fatal("release channel found a build in a source with none")
	}
	out, err := run("--check", "--channel", "edge")
	if err != nil || !strings.Contains(out, cmdEdgeB+" is available on edge") || runsAs(t, link) != cmdEdgeA {
		t.Fatalf("check: %v %s", err, out)
	}
	out, err = run("--channel", "edge")
	if err != nil || !strings.Contains(out, "updated "+cmdEdgeA+" -> "+cmdEdgeB) || runsAs(t, link) != cmdEdgeB || svc.calls != 1 {
		t.Fatalf("update: %v %s", err, out)
	}
	if cfg, _ := os.ReadFile(os.Getenv("HEV_CONFIG")); !strings.Contains(string(cfg), `channel = "edge"`) {
		t.Fatalf("channel not recorded: %s", cfg)
	}
	version.Version = cmdEdgeB
	out, err = run("--rollback")
	if err != nil || runsAs(t, link) != cmdEdgeA {
		t.Fatalf("rollback: %v %s", err, out)
	}
	version.Version = cmdEdgeA
	out, err = run("--to", cmdEdgeB, "--json")
	var rep update.Report
	if err != nil || json.Unmarshal([]byte(out), &rep) != nil || rep.After != cmdEdgeB {
		t.Fatalf("--to: %v %s", err, out)
	}
	if _, err := run("--to", "https://example.com/hev"); err == nil {
		t.Fatal("--to took a URL")
	}
}

func localDispatchSender(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0o700)
	// An older host: ssh answers as hev 0.6.2's dispatch rpc does.
	os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\ncat >/dev/null\necho '{\"cap\":0,\"version\":\"0.6.2\",\"error\":\"invalid name\"}'\n"), 0o700)
	target := filepath.Join(t.TempDir(), "target.toml")
	os.WriteFile(target, []byte("cap=3\n"), 0o600)
	t.Setenv("HEV_DISPATCH_CONFIG", target)
	sender := filepath.Join(t.TempDir(), "sender.toml")
	os.WriteFile(sender, []byte("[hosts.here]\nlocal=true\ncap=3\n[hosts.old]\nssh=\"old.invalid\"\ncap=1\n"), 0o600)
	return sender
}

func runDispatch(t *testing.T, args ...string) (string, error) {
	t.Helper()
	c := newDispatchCommand()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), err
}

func TestDispatchHostsShowsChannelAndBehind(t *testing.T) {
	updateSandbox(t)
	sender := localDispatchSender(t)
	if err := update.SetChannel(os.Getenv("HEV_CONFIG"), "edge"); err != nil {
		t.Fatal(err)
	}
	out, err := runDispatch(t, "--config", sender, "--host", "here", "hosts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "CHANNEL") || !strings.Contains(out, "edge") || !strings.Contains(out, "yes, 1 commits ("+cmdEdgeB+")") {
		t.Fatalf("hosts:\n%s", out)
	}
	version.Version = cmdEdgeB
	if out, _ := runDispatch(t, "--config", sender, "--host", "here", "hosts"); !strings.Contains(out, "edge     no") && !strings.Contains(out, "edge  no") {
		t.Fatalf("hosts after moving:\n%s", out)
	}
}

func TestDispatchHostsUpdate(t *testing.T) {
	link, svc := updateSandbox(t)
	sender := localDispatchSender(t)
	if _, err := runDispatch(t, "--config", sender, "hosts", "update"); err == nil {
		t.Fatal("hosts update without --host or --all")
	}
	out, err := runDispatch(t, "--config", sender, "--host", "here", "hosts", "update", "--channel", "edge", "--check")
	if err != nil || runsAs(t, link) != cmdEdgeA || !strings.Contains(out, "is available on edge") {
		t.Fatalf("check: %v\n%s", err, out)
	}
	// --all: the local host moves; the older host says what to do.
	out, err = runDispatch(t, "--config", sender, "hosts", "update", "--all", "--channel", "edge")
	if err == nil || !strings.Contains(err.Error(), "1 of 2 hosts did not update") {
		t.Fatalf("err = %v\n%s", err, out)
	}
	lines := strings.Split(out, "\n")
	var here, old string
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "here "):
			here = l
		case strings.HasPrefix(l, "old "):
			old = l
		}
	}
	if f := strings.Fields(here); len(f) < 3 || f[1] != cmdEdgeA || f[2] != cmdEdgeB {
		t.Fatalf("here row: %q\n%s", here, out)
	}
	if runsAs(t, link) != cmdEdgeB || svc.calls != 1 {
		t.Fatal("local host did not move")
	}
	if !strings.Contains(old, "old runs hev 0.6.2, which cannot update itself on request") || !strings.Contains(old, "by hand once") {
		t.Fatalf("old row: %q", old)
	}
}
