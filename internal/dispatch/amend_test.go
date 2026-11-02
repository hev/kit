package dispatch

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func amendFixture(t *testing.T, live bool) (string, Meta) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(Root(), "example")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	o, err := Parse(good)
	if err != nil {
		t.Fatal(err)
	}
	m := Meta{Order: o, Branch: "d/example", Start: time.Now().UTC(), LaunchResolved: true}
	if err = WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "order.md"), []byte(good), 0600); err != nil {
		t.Fatal(err)
	}
	// A real supervisor process record exercises liveness on both Unix and Windows.
	if live {
		if err = writePID(dir); err != nil {
			t.Fatal(err)
		}
	}
	launchBefore, stopBefore := amendLaunch, amendStop
	t.Cleanup(func() { amendLaunch = launchBefore; amendStop = stopBefore })
	amendLaunch = func(dir, name string) error { return nil }
	amendStop = func(dir, name string) error {
		_ = os.Remove(filepath.Join(dir, "supervisor.json"))
		return WriteJSON(filepath.Join(dir, "result.json"), Result{Stopped: true, End: time.Now().UTC(), PR: "https://example.com/pull/1", Session: "first-session"})
	}
	return dir, m
}

func TestAmendLiveBodyAndShowHistory(t *testing.T) {
	dir, _ := amendFixture(t, true)
	revised := strings.Replace(good, "- done", "- revised acceptance", 1)
	r, err := Handle(Request{Action: "amend", Name: "example", Order: revised})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Message, "attempt 1") {
		t.Fatal(r.Message)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "order.md"))
	if string(b) != revised {
		t.Fatal(string(b))
	}
	b, _ = os.ReadFile(filepath.Join(dir, "inbox.md"))
	if !strings.Contains(string(b), "+- revised acceptance") || !strings.Contains(string(b), "-- done") {
		t.Fatal(string(b))
	}
	show, err := Handle(Request{Action: "show", Name: "example"})
	if err != nil {
		t.Fatal(err)
	}
	if len(show.History.Revisions) != 2 || len(show.History.Attempts) != 1 || show.History.Revisions[0].Order != good {
		t.Fatalf("%+v", show.History)
	}
	wire, _ := json.Marshal(show)
	var decoded Response
	if err = json.Unmarshal(wire, &decoded); err != nil || len(decoded.History.Revisions) != 2 {
		t.Fatalf("%s %v", wire, err)
	}
	if name, _ := Attribution(filepath.Join(dir, "wt")); name != "example" {
		t.Fatal(name)
	}
}

func TestAmendInPlaceFields(t *testing.T) {
	dir, _ := amendFixture(t, true)
	revised := strings.Replace(good, "repo: kit", "repo: kit\ntimeout: 10\nreply: team\nvalidation: browser", 1)
	called := false
	amendLaunch = func(string, string) error { called = true; return nil }
	if _, err := Handle(Request{Action: "amend", Name: "example", Order: revised}); err != nil {
		t.Fatal(err)
	}
	var m Meta
	_ = readJSON(filepath.Join(dir, "meta.json"), &m)
	if m.Timeout != 10 || m.Reply != "team" || m.Validation != "browser" || called {
		t.Fatalf("%+v launch=%v", m, called)
	}
}

func TestAmendRestartFields(t *testing.T) {
	for _, field := range []string{"harness: claude", "model: other", "base: release"} {
		t.Run(field, func(t *testing.T) {
			dir, m := amendFixture(t, true)
			revised := strings.Replace(good, "repo: kit", "repo: kit\n"+field, 1)
			launches := 0
			amendLaunch = func(string, string) error { launches++; return nil }
			if _, err := Handle(Request{Action: "amend", Name: "example", Order: revised}); err != nil {
				t.Fatal(err)
			}
			h := history(dir, m)
			if launches != 1 || len(h.Attempts) != 2 || len(h.Revisions) != 2 {
				t.Fatalf("%+v launches=%d", h, launches)
			}
			if h.Attempts[0].Result == nil || h.Attempts[0].Result.Session != "first-session" || h.Attempts[1].Meta.Branch != m.Branch {
				t.Fatalf("%+v", h.Attempts)
			}
			if name, _ := Attribution(filepath.Join(dir, "wt")); name != m.Name {
				t.Fatal(name)
			}
		})
	}
}

func TestAmendEndedUsesRevisedOrder(t *testing.T) {
	dir, m := amendFixture(t, false)
	revised := strings.Replace(good, "- done", "- revised", 1)
	_ = WriteJSON(filepath.Join(dir, "result.json"), Result{End: time.Now().UTC(), PR: "https://example.com/pull/1"})
	launches := 0
	amendLaunch = func(string, string) error { launches++; return nil }
	if _, err := Handle(Request{Action: "amend", Name: m.Name, Order: revised}); err != nil {
		t.Fatal(err)
	}
	h := history(dir, m)
	if launches != 1 || len(h.Attempts) != 2 || h.Attempts[0].Result.PR != "https://example.com/pull/1" {
		t.Fatalf("%+v", h)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "order.md"))
	if string(b) != revised || strings.Contains(string(b), "## Rework") {
		t.Fatal(string(b))
	}
}

func TestAmendNameChangeRefused(t *testing.T) {
	dir, _ := amendFixture(t, true)
	before, _ := os.ReadFile(filepath.Join(dir, "meta.json"))
	if _, err := Handle(Request{Action: "amend", Name: "example", Order: strings.Replace(good, "name: example", "name: renamed", 1)}); err == nil {
		t.Fatal("renamed")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "meta.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("record changed")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "order.md"))
	if string(b) != good {
		t.Fatal("order changed")
	}
}

func TestAmendNewHostAdmissionRefusalPreservesRun(t *testing.T) {
	dir, m := amendFixture(t, true)
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("cap=2\n[admission]\nrepos=[]\n[hosts.other]\nlocal=true\ncap=2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.ConfigPath = cfg
	_ = WriteJSON(filepath.Join(dir, "meta.json"), m)
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	previousRPC := routeRPC
	t.Cleanup(func() { routeRPC = previousRPC })
	targetHome := t.TempDir()
	routeRPC = func(ctx context.Context, host Host, q Request) (Response, error) {
		home, profile := os.Getenv("HOME"), os.Getenv("USERPROFILE")
		_ = os.Setenv("HOME", targetHome)
		_ = os.Setenv("USERPROFILE", targetHome)
		defer os.Setenv("HOME", home)
		defer os.Setenv("USERPROFILE", profile)
		return RPC(ctx, host, q)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "meta.json"))
	stop := false
	amendStop = func(string, string) error { stop = true; return nil }
	_, err := Handle(Request{Action: "amend", Name: m.Name, Order: strings.Replace(good, "host: buildbox", "host: other", 1)})
	if err == nil || !strings.Contains(err.Error(), "not admitted") {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "meta.json"))
	if stop || !bytes.Equal(before, after) || !supervisorLive(dir) {
		t.Fatal("refusal mutated original")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "order.md"))
	if string(b) != good {
		t.Fatal("order changed")
	}
}

func TestAmendTransferPreservesBranchDirtyFilesAndHistory(t *testing.T) {
	dir, m := amendFixture(t, false)
	wt := filepath.Join(dir, "wt")
	for _, args := range [][]string{{"init", "-b", m.Branch}, {"-c", "user.name=Example", "-c", "user.email=example@example.com", "commit", "--allow-empty", "-m", "initial"}} {
		if _, err := command(wt, append([]string{"git"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(wt, "pending.txt"), []byte("uncommitted work"), 0600)
	h := history(dir, m)
	data, err := packAttempt(dir, m.Branch, h)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err = unpackAttempt(out, data); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(out, "wt", "pending.txt"))
	if string(b) != "uncommitted work" {
		t.Fatal(string(b))
	}
	if _, err = os.Stat(filepath.Join(out, "wt", ".git")); !os.IsNotExist(err) {
		t.Fatal("transferred git pointer")
	}
	var got History
	_ = readJSON(filepath.Join(out, "history.json"), &got)
	if !reflect.DeepEqual(got, h) {
		t.Fatalf("%+v != %+v", got, h)
	}
	if _, err = command(out, "git", "bundle", "verify", "branch.bundle"); err == nil {
		t.Fatal("bundle verification should need repository")
	}
	if _, err = command(wt, "git", "bundle", "verify", filepath.Join(out, "branch.bundle")); err != nil {
		t.Fatal(err)
	}
}

func TestAmendTransferRejectsUnsafePaths(t *testing.T) {
	for _, name := range []string{"../escape", "sub/../../escape", "/absolute", "wt/.git", "wt/.git/config", "C:/escape", "wt\\file"} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: 1})
			_, _ = tw.Write([]byte("x"))
			_ = tw.Close()
			if err := unpackAttempt(t.TempDir(), buf.Bytes()); err == nil {
				t.Fatal("unsafe path accepted")
			}
		})
	}
}

func TestAmendHostMoveKeepsBranchAndInbox(t *testing.T) {
	dir, m := amendFixture(t, true)
	sourceHome := os.Getenv("HOME")
	targetHome := t.TempDir()
	workspace := t.TempDir()
	repo := filepath.Join(workspace, "kit")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"-c", "user.name=Example", "-c", "user.email=example@example.com", "commit", "--allow-empty", "-m", "target"}} {
		if _, err := command(repo, append([]string{"git"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	wt := filepath.Join(dir, "wt")
	for _, args := range [][]string{{"init", "-b", m.Branch}, {"-c", "user.name=Example", "-c", "user.email=example@example.com", "commit", "--allow-empty", "-m", "source"}} {
		if _, err := command(wt, append([]string{"git"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(wt, "pending.txt"), []byte("pending changes"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "inbox.md"), []byte("held message"), 0600)
	targetCfg := filepath.Join(t.TempDir(), "target.toml")
	// TOML literal paths are portable for this test's generated directories.
	_ = os.WriteFile(targetCfg, []byte("cap=2\nworkspace='"+workspace+"'\n"), 0600)
	sourceCfg := filepath.Join(t.TempDir(), "source.toml")
	_ = os.WriteFile(sourceCfg, []byte("[hosts.other]\nlocal=true\ncap=2\nconfig_path='"+targetCfg+"'\n"), 0600)
	m.ConfigPath = sourceCfg
	_ = WriteJSON(filepath.Join(dir, "meta.json"), m)
	previousRPC := routeRPC
	t.Cleanup(func() { routeRPC = previousRPC })
	routeRPC = func(ctx context.Context, host Host, q Request) (Response, error) {
		home, profile, config := os.Getenv("HOME"), os.Getenv("USERPROFILE"), os.Getenv("HEV_DISPATCH_CONFIG")
		_ = os.Setenv("HOME", targetHome)
		_ = os.Setenv("USERPROFILE", targetHome)
		_ = os.Setenv("HEV_DISPATCH_CONFIG", targetCfg)
		defer os.Setenv("HOME", home)
		defer os.Setenv("USERPROFILE", profile)
		defer os.Setenv("HEV_DISPATCH_CONFIG", config)
		return RPC(ctx, host, q)
	}
	launches := 0
	amendLaunch = func(dir, name string) error { launches++; return nil }
	revised := strings.Replace(good, "host: buildbox", "host: other", 1)
	if _, err := Handle(Request{Action: "amend", Name: m.Name, Order: revised}); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("HOME") != sourceHome {
		t.Fatal("test routing leaked environment")
	}
	var owner Meta
	_ = readJSON(filepath.Join(dir, "meta.json"), &owner)
	if owner.Forward == nil || launches != 1 {
		t.Fatalf("%+v launches=%d", owner, launches)
	}
	targetDir := filepath.Join(targetHome, "dispatch", m.Name)
	var next Meta
	_ = readJSON(filepath.Join(targetDir, "meta.json"), &next)
	if next.Branch != m.Branch || next.Host != "other" || !next.Imported {
		t.Fatalf("%+v", next)
	}
	h := history(targetDir, next)
	if len(h.Attempts) != 2 || len(h.Revisions) != 2 || h.Attempts[0].Result.Session != "first-session" {
		t.Fatalf("%+v", h)
	}
	b, _ := os.ReadFile(filepath.Join(targetDir, "wt", "pending.txt"))
	if string(b) != "pending changes" {
		t.Fatal(string(b))
	}
	b, _ = os.ReadFile(filepath.Join(targetDir, "inbox.md"))
	if string(b) != "held message" {
		t.Fatal(string(b))
	}
	head, err := command(wt, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	nextHead, err := command(filepath.Join(targetDir, "wt"), "git", "rev-parse", "HEAD")
	if err != nil || nextHead != head {
		t.Fatalf("%s != %s %v", nextHead, head, err)
	}
	// The original address routes peer messages to the imported attempt.
	if _, err := Handle(Request{Action: "peer-send", Name: m.Name, Note: "after move"}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(targetDir, "inbox.md"))
	if !strings.Contains(string(b), "after move") {
		t.Fatal(string(b))
	}
	for _, attempt := range h.Attempts {
		if attempt.Meta.Name != m.Name || attempt.Meta.Branch != m.Branch {
			t.Fatalf("identity lost: %+v", attempt)
		}
	}
}

func TestAmendSupervisorReloadsSettingsAndDeadline(t *testing.T) {
	dir, m := amendFixture(t, true)
	current := m
	current.Timeout = 1
	current.Reply = "new-reply"
	current.Validation = "browser"
	if err := WriteJSON(filepath.Join(dir, "meta.json"), current); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(time.Hour)
	defer deadline.Stop()
	reloadAttemptOrder(dir, &m, time.Now().Add(-2*time.Minute), deadline)
	if m.Reply != "new-reply" || m.Validation != "browser" || m.Timeout != 1 {
		t.Fatalf("%+v", m.Order)
	}
	select {
	case <-deadline.C:
	case <-time.After(time.Second):
		t.Fatal("reduced timeout was not applied")
	}
	current.Start = current.Start.Add(time.Second)
	current.Reply = "next-attempt"
	if err := WriteJSON(filepath.Join(dir, "meta.json"), current); err != nil {
		t.Fatal(err)
	}
	reloadAttemptOrder(dir, &m, time.Now(), nil)
	if m.Reply != "new-reply" {
		t.Fatal("old supervisor loaded next attempt's order")
	}
}

func TestAmendNewHostCapRefusalPreservesRun(t *testing.T) {
	dir, m := amendFixture(t, true)
	targetHome := t.TempDir()
	busy := filepath.Join(targetHome, "dispatch", "busy")
	if err := os.MkdirAll(busy, 0700); err != nil {
		t.Fatal(err)
	}
	// Both platforms use a real process record for this occupied target slot.
	if err := writeProcessRecord(filepath.Join(busy, "supervisor.json"), os.Getpid()); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("cap=1\n[hosts.other]\nlocal=true\ncap=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.ConfigPath = cfg
	_ = WriteJSON(filepath.Join(dir, "meta.json"), m)
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	before, _ := os.ReadFile(filepath.Join(dir, "meta.json"))
	previousRPC := routeRPC
	t.Cleanup(func() { routeRPC = previousRPC })
	routeRPC = func(ctx context.Context, host Host, q Request) (Response, error) {
		home, profile := os.Getenv("HOME"), os.Getenv("USERPROFILE")
		_ = os.Setenv("HOME", targetHome)
		_ = os.Setenv("USERPROFILE", targetHome)
		defer os.Setenv("HOME", home)
		defer os.Setenv("USERPROFILE", profile)
		return RPC(ctx, host, q)
	}
	stopped := false
	amendStop = func(string, string) error { stopped = true; return nil }
	_, err := Handle(Request{Action: "amend", Name: m.Name, Order: strings.Replace(good, "host: buildbox", "host: other", 1)})
	if err == nil || !strings.Contains(err.Error(), "host full") {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "meta.json"))
	if stopped || !bytes.Equal(before, after) {
		t.Fatal("capacity refusal changed source")
	}
}
