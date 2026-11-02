//go:build !windows

package dispatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPeerSendLiveAndEndedNeverReworks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	tmux := filepath.Join(bin, "tmux")
	os.WriteFile(tmux, []byte("#!/bin/sh\necho d-example\n"), 0700)
	dir := filepath.Join(Root(), "example")
	os.MkdirAll(filepath.Join(dir, "wt"), 0700)
	WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "example"}, Branch: "d/example"})
	original := "original order"
	os.WriteFile(filepath.Join(dir, "order.md"), []byte(original), 0600)
	r, err := Handle(Request{Action: "peer-send", Name: "example", Note: "live message"})
	if err != nil || r.Message != "delivered to run inbox" {
		t.Fatal(r, err)
	}
	os.WriteFile(tmux, []byte("#!/bin/sh\nexit 0\n"), 0700)
	r, err = Handle(Request{Action: "peer-send", Name: "example", Note: "ended message"})
	if err != nil || !strings.Contains(r.Message, "held") {
		t.Fatal(r, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "order.md"))
	if string(b) != original {
		t.Fatal("peer message started rework")
	}
	b, _ = os.ReadFile(filepath.Join(dir, "inbox.md"))
	if !strings.Contains(string(b), "live message") || !strings.Contains(string(b), "ended message") {
		t.Fatal(string(b))
	}
}

// fakeNotifyPeers models sender resolution after the harness has exited: only
// the supervisor pane remains, and an explicit unknown sender is rejected.
func fakeNotifyPeers(t *testing.T) (string, []string) {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	script := `#!/bin/sh
if [ -n "$PEERS_FROM" ] && [ "$PEERS_FROM" != "oven/d-example" ]; then
 echo "no pane matches $PEERS_FROM" >&2
 exit 1
fi
[ "$TMUX_PANE" = "%supervisor" ] || exit 2
[ "$1" = send ] && [ "$2" = desk ] || exit 3
printf '%s\n' "$3" >> "$NOTIFY_LOG"
`
	if err := os.WriteFile(filepath.Join(bin, "peers"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PEERS_FROM", "")
	t.Setenv("TMUX_PANE", "%supervisor")
	t.Setenv("NOTIFY_LOG", log)
	return log, []string{"peers", "send", "{reply}", "dispatch {name}: {state} {summary}"}
}

func TestPeerNotificationFinishedRun(t *testing.T) {
	log, args := fakeNotifyPeers(t)
	t.Setenv("PEERS_FROM", "unknown/inherited")
	m := Meta{Order: Order{Name: "example", Host: "configured-host-alias", Reply: "desk"}}
	if err := Notify(context.Background(), args, m, Result{End: time.Now()}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil || string(b) != "dispatch example: done exit=0\n" {
		t.Fatalf("%q: %v", b, err)
	}
}

func TestPeerNotificationWithoutReply(t *testing.T) {
	log, args := fakeNotifyPeers(t)
	for _, target := range []string{"{reply}", "prefix-{reply}"} {
		args[2] = target
		if err := Notify(context.Background(), args, Meta{}, Result{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("notify was called: %v", err)
	}
	// Commands that do not use reply still run without an order reply.
	args[2] = "desk"
	if err := Notify(context.Background(), args, Meta{}, Result{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(log); err != nil {
		t.Fatal(err)
	}
}

func TestPeerNotificationRepeatFinalOnly(t *testing.T) {
	log, args := fakeNotifyPeers(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(Root(), "example")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for name, script := range map[string]string{"codex": "#!/bin/sh\nexit 0\n", "gh": "#!/bin/sh\nexit 1\n", "git": "#!/bin/sh\necho test-head\n"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := filepath.Join(bin, "dispatch.toml")
	if err := os.WriteFile(cfg, []byte(`notify = ["peers", "send", "{reply}", "dispatch {name}: {state} {summary}"]`), 0600); err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	m := Meta{Start: clock, ConfigPath: cfg, LaunchResolved: true, Order: Order{Name: "example", Host: "alias", Reply: "desk", Harness: "codex", Timeout: 1, Every: "1m", MaxLifetime: "3m", Until: "done"}}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	rounds := 0
	round := func(name string) error {
		if err := superviseRound(name); err != nil {
			return err
		}
		var result Result
		if err := readJSON(filepath.Join(dir, "result.json"), &result); err != nil || result.ExitCode != 0 || result.Error != "" {
			t.Fatalf("round result=%+v err=%v", result, err)
		}
		if _, err := os.Stat(log); !os.IsNotExist(err) {
			t.Fatalf("early notification: %v", err)
		}
		rounds++
		if rounds == 2 {
			return WriteJSON(filepath.Join(dir, "until-met.json"), map[string]any{"met": true, "reason": "done"})
		}
		return WriteJSON(filepath.Join(dir, "result.json"), Result{End: clock})
	}
	err := repeatRoundsNotify(dir, m, make(chan os.Signal), func() time.Time { return clock }, func() { clock = clock.Add(time.Minute) }, round, func(m Meta, r Result) error { return Notify(context.Background(), args, m, r) })
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil || rounds != 2 || strings.Count(string(b), "dispatch example:") != 1 || !strings.Contains(string(b), "repeat ended: until") {
		t.Fatalf("rounds=%d calls=%q err=%v", rounds, b, err)
	}
}

func TestValidationNotificationDetachedSender(t *testing.T) {
	for _, live := range []bool{true, false} {
		t.Run(fmt.Sprint("live=", live), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			bin := t.TempDir()
			log := filepath.Join(bin, "calls")
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("TMUX_PANE", "")
			t.Setenv("PEERS_FROM", "wrong/unknown")
			t.Setenv("PEERS_SELF", "oven")
			t.Setenv("HEV_PEERS_BACKEND", "external-backend")
			t.Setenv("NOTIFY_LOG", log)
			tmux := "#!/bin/sh\nexit 1\n"
			if live {
				tmux = "#!/bin/sh\necho d-example\n"
			}
			if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte(tmux), 0700); err != nil {
				t.Fatal(err)
			}
			script := `#!/bin/sh
[ -z "$TMUX_PANE" ] && [ -z "$PEERS_FROM" ] || exit 1
if [ "$1" = peers ]; then
 [ -z "$HEV_PEERS_BACKEND" ] || exit 2
 shift
 expected=oven/d-example
else
 expected=d-example
fi
[ "$1" = send ] && [ "$2" = desk ] || exit 3
[ "$4" = --from ] && [ "$5" = "$expected" ] || { echo "unknown sender: $5" >&2; exit 4; }
printf '%s\n' "$3" >> "$NOTIFY_LOG"
`
			for _, name := range []string{"peers", "hev"} {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
			}
			old := notificationExecutable
			notificationExecutable = func() (string, error) { return filepath.Join(bin, "hev"), nil }
			defer func() { notificationExecutable = old }()
			dir := filepath.Join(Root(), "example")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			cfg := filepath.Join(bin, "dispatch.toml")
			if err := os.WriteFile(cfg, []byte(`notify = ["peers", "send", "{reply}", "dispatch {name}: {state} {summary}"]`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := WriteJSON(filepath.Join(dir, "result.json"), Result{End: time.Now(), Validation: "pass"}); err != nil {
				t.Fatal(err)
			}
			m := Meta{ConfigPath: cfg, Order: Order{Name: "example", Host: "configured-alias", Reply: "desk"}}
			if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
				t.Fatal(err)
			}
			if err := NotifyValidation("example"); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(log)
			if err != nil || !strings.Contains(string(b), "validation=pass") {
				t.Fatalf("notice=%q err=%v", b, err)
			}
			// The native path uses the qualified run address even while live.
			if err := notifyValidationResult(context.Background(), []string{filepath.Join(bin, "hev"), "peers", "send", "{reply}", "dispatch {name}: {state} {summary}"}, m, Result{End: time.Now(), Validation: "pass"}); err != nil {
				t.Fatal(err)
			}
			m.Reply = ""
			if err := notifyValidationResult(context.Background(), []string{"peers", "send", "{reply}", "unused"}, m, Result{}); err != nil {
				t.Fatal(err)
			}
			b, err = os.ReadFile(log)
			if err != nil || strings.Count(string(b), "dispatch example:") != 2 {
				t.Fatalf("unexpected calls %q: %v", b, err)
			}
		})
	}
}
