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

// The run side: a run whose completion notice failed with "no pane" is
// re-pointed, and the retry reaches the new owner through a fake peers.
func TestAdoptRunRetriesFailedNotification(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	bin := t.TempDir()
	calls := filepath.Join(bin, "calls")
	script := "#!/bin/sh\n[ \"$1\" = send ] || exit 3\nif [ \"$2\" != \"" + ownerB + "\" ]; then echo \"no pane on desk matches '$2'\" >&2; exit 1; fi\nprintf '%s %s\\n' \"$2\" \"$3\" >> \"" + calls + "\"\n"
	if err := os.WriteFile(filepath.Join(bin, "peers"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cfg := filepath.Join(bin, "dispatch.toml")
	if err := os.WriteFile(cfg, []byte(`notify = ["peers", "send", "{reply}", "dispatch {name}: {state}"]`), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(Root(), "orphan")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	m := Meta{Order: Order{Name: "orphan", Reply: ownerA}, ConfigPath: cfg}
	r := Result{End: time.Now()}
	// The original send failed the way a cleared session fails.
	r.NotifyError = Notify(context.Background(), []string{"peers", "send", "{reply}", "x"}, m, r).Error()
	if !strings.Contains(r.NotifyError, "no pane") {
		t.Fatal(r.NotifyError)
	}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(dir, "result.json"), r); err != nil {
		t.Fatal(err)
	}
	resp, err := Handle(Request{Action: "adopt", Name: "orphan", To: ownerB})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Adopt == nil || !resp.Adopt.Retried || resp.Adopt.RetryError != "" || resp.Adopt.From != ownerA {
		t.Fatalf("%+v", resp.Adopt)
	}
	var after Result
	var meta Meta
	_ = readJSON(filepath.Join(dir, "result.json"), &after)
	_ = readJSON(filepath.Join(dir, "meta.json"), &meta)
	if after.NotifyError != "" || meta.Reply != ownerB {
		t.Fatalf("%+v %+v", after, meta.Order)
	}
	b, _ := os.ReadFile(calls)
	if string(b) != ownerB+" dispatch orphan: done\n" {
		t.Fatalf("%q", b)
	}
	if _, err := Handle(Request{Action: "adopt", Name: "missing", To: ownerB}); err == nil {
		t.Fatal("adopted a missing run")
	}
}
