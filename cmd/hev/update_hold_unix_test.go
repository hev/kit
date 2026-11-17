//go:build !windows

package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/update"
	"github.com/hev/kit/internal/version"
)

// hev update hold / release, the hold in hev dispatch hosts, a held host
// refusing hev dispatch hosts update without --ignore-hold, holds set
// remotely, and a manual update's warning.
func TestUpdateHoldCommands(t *testing.T) {
	link, svc := updateSandbox(t)
	sender := localDispatchSender(t)
	if err := update.SetChannel(os.Getenv("HEV_CONFIG"), "edge"); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		c := newUpdateCommand()
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetErr(&out)
		c.SetArgs(args)
		err := c.Execute()
		return out.String(), err
	}
	out, err := run("hold", "--for", "4h", "--reason", "0.7 rollout")
	if err != nil || !strings.Contains(out, "held until") || !strings.Contains(out, "by cli (0.7 rollout)") {
		t.Fatalf("hold: %v %s", err, out)
	}
	if h, _ := hostUpdateStatus(); h == nil || h.Reason != "0.7 rollout" {
		t.Fatalf("status hold = %+v", h)
	}
	var s bytes.Buffer
	writeUpdateStatus(&s)
	if !strings.Contains(s.String(), "Update: held until") {
		t.Fatalf("hev s: %q", s.String())
	}
	out, err = runDispatch(t, "--config", sender, "--host", "here", "hosts")
	if err != nil || !strings.Contains(out, "UPDATE") || !strings.Contains(out, "(0.7 rollout)") {
		t.Fatalf("hosts:\n%s %v", out, err)
	}
	// The held host refuses an update, then moves with --ignore-hold.
	out, err = runDispatch(t, "--config", sender, "--host", "here", "hosts", "update")
	if err == nil || !strings.Contains(out, "--ignore-hold") || runsAs(t, link) != cmdEdgeA {
		t.Fatalf("held host updated: %v\n%s", err, out)
	}
	out, err = runDispatch(t, "--config", sender, "--host", "here", "hosts", "update", "--ignore-hold", "--json")
	if err != nil || runsAs(t, link) != cmdEdgeB || svc.calls != 1 || !strings.Contains(out, "warning: updating although the daemon") {
		t.Fatalf("--ignore-hold: %v\n%s", err, out)
	}
	// Release and set the hold remotely.
	out, err = runDispatch(t, "--config", sender, "--host", "here", "hosts", "update", "--release")
	if err != nil || !strings.Contains(out, "update hold released") {
		t.Fatalf("remote release: %v\n%s", err, out)
	}
	if h, _ := hostUpdateStatus(); h != nil {
		t.Fatal("hold survived release")
	}
	out, err = runDispatch(t, "--config", sender, "--host", "here", "hosts", "update", "--hold", "--reason", "next host first")
	if err != nil || !strings.Contains(out, "held until released by dispatch:") {
		t.Fatalf("remote hold: %v\n%s", err, out)
	}
	if _, err := runDispatch(t, "--config", sender, "--host", "here", "hosts", "update", "--hold", "--channel", "edge"); err == nil {
		t.Fatal("--hold with an update accepted")
	}
	// An older host has no holds and says so.
	out, err = runDispatch(t, "--config", sender, "--host", "old", "hosts", "update", "--hold")
	if err == nil || !strings.Contains(out, "has no update holds") {
		t.Fatalf("old host hold: %v\n%s", err, out)
	}
	// A manual hev update ignores the hold and warns.
	version.Version = cmdEdgeB
	out, err = run("--to", cmdEdgeA)
	if err != nil || runsAs(t, link) != cmdEdgeA || !strings.Contains(out, "warning: updating although the daemon's automatic update would defer: update held until released") {
		t.Fatalf("manual update while held: %v\n%s", err, out)
	}
	out, err = run("release")
	if err != nil || !strings.Contains(out, "update hold released") {
		t.Fatalf("release: %v %s", err, out)
	}
	if out, _ := run("release"); !strings.Contains(out, "no update hold was set") {
		t.Fatalf("second release: %s", out)
	}
}

// The daemon's own scheduled check, wired as on a host: a dispatch run's tmux
// session defers it, then a hold does, and past max_defer it updates anyway.
func TestDaemonAutoUpdateDefersForRunningRunThenMaxDeferral(t *testing.T) {
	link, _ := updateSandbox(t)
	localDispatchSender(t)
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	// tmux lists one live dispatch run session.
	os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\necho d-kit-hooks-steering-20261116\necho foh-hev-7\n"), 0o700)
	os.MkdirAll(filepath.Join(os.Getenv("HOME"), "dispatch", "kit-hooks-steering-20261116"), 0o700)
	os.MkdirAll(filepath.Dir(os.Getenv("HEV_CONFIG")), 0o700)
	os.WriteFile(os.Getenv("HEV_CONFIG"), []byte("[update]\nchannel = \"edge\"\nauto = true\nmax_defer = \"2h\"\n"), 0o600)
	now := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	saved := adjustUpdater
	adjustUpdater = func(u *update.Updater) { saved(u); u.Now = func() time.Time { return now } }
	var log bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&log, nil))
	ctx := context.Background()

	if daemonAutoUpdate(ctx, logger) || runsAs(t, link) != cmdEdgeA {
		t.Fatal("updated while a dispatch run was running")
	}
	if !strings.Contains(log.String(), "update: scheduled check deferred") || !strings.Contains(log.String(), "running here: kit-hooks-steering-20261116") {
		t.Fatalf("log: %s", log.String())
	}
	var s bytes.Buffer
	writeUpdateStatus(&s)
	if !strings.Contains(s.String(), "Update: automatic update deferred since 2026-11-17T08:00:00Z: running here: kit-hooks-steering-20261116") {
		t.Fatalf("hev s: %q", s.String())
	}
	t.Logf("hev s: %s", strings.TrimSpace(s.String()))

	// The run ends; a hold still defers.
	os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\necho 'no server running' >&2\nexit 1\n"), 0o700)
	u, _ := newHostUpdater("cli", nil)
	u.SetHold(0, "0.7 rollout")
	now = now.Add(10 * time.Minute)
	log.Reset()
	if daemonAutoUpdate(ctx, logger) || runsAs(t, link) != cmdEdgeA || !strings.Contains(log.String(), "0.7 rollout") {
		t.Fatalf("updated while held: %s", log.String())
	}

	// Past max_defer it updates anyway and says why.
	now = now.Add(2 * time.Hour)
	log.Reset()
	if !daemonAutoUpdate(ctx, logger) || runsAs(t, link) != cmdEdgeB {
		t.Fatalf("did not update past max_defer: %s", log.String())
	}
	if !strings.Contains(log.String(), "max deferral reached; updating anyway") || !strings.Contains(log.String(), "max_defer 2h0m0s") {
		t.Fatalf("log does not say why: %s", log.String())
	}
	t.Logf("log: %s", strings.TrimSpace(log.String()))
}
