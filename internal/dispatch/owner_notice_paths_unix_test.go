//go:build !windows

package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ownerNoticeHome gives a test its own dispatch root and a notify config
// whose command fails the way an unreachable owner does (SAY prints its
// output, EXIT its status).
func ownerNoticeHome(t *testing.T) (cfg string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PEERS_SELF", "win-oven")
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SAY", "ssh: Could not resolve hostname macbook: Name or service not known")
	t.Setenv("EXIT", "255")
	if err := os.WriteFile(filepath.Join(bin, "owner-push"), []byte("#!/bin/sh\nprintf '%s\\n' \"$SAY\" >&2\nexit \"$EXIT\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg = filepath.Join(bin, "dispatch.toml")
	if err := os.WriteFile(cfg, []byte(`notify = ["owner-push", "{reply}", "dispatch {name}: {state} {summary}"]`), 0600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func pulled(t *testing.T) []OrchestratorNotice {
	t.Helper()
	r, err := Handle(Request{Action: "orchestrator-notices"})
	if err != nil {
		t.Fatal(err)
	}
	return r.Notices
}

// The end notice of a repeating run goes through the same push and is
// queued when the owner cannot be reached (review finding 2).
func TestRepeatEndNoticeUnreachableIsQueued(t *testing.T) {
	cfg := ownerNoticeHome(t)
	dir := filepath.Join(Root(), "rep")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	m := Meta{Start: clock, ConfigPath: cfg, LaunchResolved: true, Order: Order{Name: "rep", Host: "oven", Reply: "Adams-MacBook-Air/foh-bcc", Harness: "codex", Timeout: 1, Every: "1m", MaxLifetime: "3m", Until: "done"}}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	round := func(string) error {
		if err := WriteJSON(filepath.Join(dir, "result.json"), Result{End: clock}); err != nil {
			return err
		}
		return WriteJSON(filepath.Join(dir, "until-met.json"), map[string]any{"met": true, "reason": "done"})
	}
	if err := repeatRounds(dir, m, nil, func() time.Time { return clock }, func() { clock = clock.Add(time.Minute) }, round); err != nil {
		t.Fatal(err)
	}
	r := readResult(t, dir)
	if !strings.Contains(r.NotifyError, "queued for hev orchestrate notices pull") {
		t.Fatalf("notify error %q", r.NotifyError)
	}
	got := pulled(t)
	if len(got) != 1 || got[0].To != "Adams-MacBook-Air/foh-bcc" || !strings.Contains(got[0].Message, "repeat ended: until") {
		t.Fatalf("pull %+v", got)
	}
}

// A validation verdict notice is queued the same way (review finding 2).
func TestValidationNoticeUnreachableIsQueued(t *testing.T) {
	cfg := ownerNoticeHome(t)
	dir := filepath.Join(Root(), "val")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(dir, "result.json"), Result{End: time.Now(), Validation: "fail"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), Meta{ConfigPath: cfg, Order: Order{Name: "val", Host: "oven", Reply: "Adams-MacBook-Air/foh-bcc"}}); err != nil {
		t.Fatal(err)
	}
	if err := NotifyValidation("val"); err != nil {
		t.Fatal(err)
	}
	if r := readResult(t, dir); !strings.Contains(r.NotifyError, "queued for hev orchestrate notices pull") {
		t.Fatalf("notify error %q", r.NotifyError)
	}
	got := pulled(t)
	if len(got) != 1 || !strings.Contains(got[0].Message, "validation=fail") {
		t.Fatalf("pull %+v", got)
	}
}

// peers prints "hevd unreachable; using ssh" before every SSH fallback. When
// that fallback reaches an inbox that refuses, the notice is not queued
// (review finding 3), and neither is a send whose outcome is unknown.
func TestReachableRefusalAfterHevdFallbackIsNotQueued(t *testing.T) {
	cfg := ownerNoticeHome(t)
	m := Meta{ConfigPath: cfg, Order: Order{Name: "refused", Host: "oven", Reply: "mini/foh-x"}}
	r := Result{End: time.Now()}
	t.Setenv("SAY", "hevd unreachable; using ssh\nerror: foh-x requires a registered inbox")
	t.Setenv("EXIT", "1")
	err := Notify(context.Background(), []string{"owner-push", "{reply}", "x"}, m, r)
	if err == nil || strings.Contains(err.Error(), "queued") {
		t.Fatalf("err %v", err)
	}
	if got := pulled(t); len(got) != 0 {
		t.Fatalf("refusal queued: %+v", got)
	}
}

// An adopt (#281) re-points the reply and retries the push through Notify.
// A retry that fails again re-addresses the one queued notice to the new
// owner; a retry that succeeds retires it, so a pull neither repeats it nor
// sends it to the old owner (review finding 7).
func TestAdoptRetrySettlesQueuedNotice(t *testing.T) {
	cfg := ownerNoticeHome(t)
	args := []string{"owner-push", "{reply}", "dispatch {name}: {state} {summary}"}
	m := Meta{ConfigPath: cfg, Start: time.Now(), Order: Order{Name: "adq", Host: "oven", Reply: "Adams-MacBook-Air/foh-old"}}
	r := Result{End: time.Now()}
	if err := Notify(context.Background(), args, m, r); err == nil || !strings.Contains(err.Error(), "queued") {
		t.Fatalf("first push %v", err)
	}
	r.NotifyError = "earlier failure" // what adopt reads back; not part of the notice
	m.Reply = "mini/foh-new"
	if err := Notify(context.Background(), args, m, r); err == nil || !strings.Contains(err.Error(), "queued") {
		t.Fatalf("retry %v", err)
	}
	if got := pulled(t); len(got) != 1 || got[0].To != "mini/foh-new" {
		t.Fatalf("after failed retry %+v", got)
	}
	t.Setenv("EXIT", "0")
	if err := Notify(context.Background(), args, m, r); err != nil {
		t.Fatal(err)
	}
	if got := pulled(t); len(got) != 0 {
		t.Fatalf("pushed notice still pullable: %+v", got)
	}
	// A later failed push of a notice the owner already has does not
	// queue it again.
	t.Setenv("EXIT", "255")
	if err := Notify(context.Background(), args, m, r); err == nil || strings.Contains(err.Error(), "queued") {
		t.Fatalf("re-queued: %v", err)
	}
	if got := pulled(t); len(got) != 0 {
		t.Fatalf("re-queued %+v", got)
	}
}
