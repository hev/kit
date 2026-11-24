package compat

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hev/kit/internal/peers/nativeinbox"
)

func TestNativeSendAndListWithoutTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	_ = os.Unsetenv("TMUX")
	_ = os.Unsetenv("TMUX_PANE")
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("HEV_NATIVE_PEER", "")
	t.Setenv("PEERS_REMOTE_FROM", "")
	t.Setenv("PEERS_FROM", "")
	t.Setenv("PEERS_SELF", "fixture")
	t.Setenv("PEERS_HOSTS", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", os.Getenv("PATH")) // Main updates PATH.
	store := nativeinbox.AtHome(home)
	peer, err := store.Register(home, "topic", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close(peer) })
	marker := filepath.Join(home, "tmux-called")
	if runtime.GOOS != "windows" {
		bin := filepath.Join(home, ".local", "bin")
		if err := os.MkdirAll(bin, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\n: > \"$HEV_TEST_TMUX_MARKER\"\nexit 1\n"), 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HEV_TEST_TMUX_MARKER", marker)
	}
	var out, errOut bytes.Buffer
	run := func(args ...string) string {
		t.Helper()
		out.Reset()
		errOut.Reset()
		if code := Main(args, strings.NewReader(""), &out, &errOut, nil); code != 0 {
			t.Fatalf("command failed (%d): %s", code, errOut.String())
		}
		return out.String()
	}
	run("send", peer.Name, "hello")
	run("send", "fixture/"+peer.Name, "steer", "--interrupt", "--from", "another-peer")
	t.Setenv("PEERS_FROM", "")
	out.Reset()
	errOut.Reset()
	const private = "private-stdin-message"
	large := strings.NewReader(private + strings.Repeat("a", nativeinbox.MaxBodyBytes))
	if code := Main([]string{"send", peer.Name, "-"}, large, &out, &errOut, nil); code == 0 ||
		strings.Contains(errOut.String(), private) || strings.Contains(errOut.String(), home) {
		t.Fatalf("unsafe oversized send: code=%d stderr=%q", code, errOut.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("native send invoked tmux: %v", err)
	}
	messages, err := store.Drain(peer, 8)
	if err != nil || len(messages) != 2 ||
		messages[0].Mode != nativeinbox.Background || messages[0].From != "local" || messages[0].Body != "hello" ||
		messages[1].Mode != nativeinbox.Interrupt || messages[1].From != "another-peer" || messages[1].Body != "steer" {
		t.Fatalf("delivery mismatch: %+v, %v", messages, err)
	}
	for _, state := range []string{"idle", "working"} {
		if err := store.Update(peer, state == "working"); err != nil {
			t.Fatal(err)
		}
		out := run("ls", "--json")
		doc, err := LoadObj([]byte(out))
		if err != nil {
			t.Fatal(err)
		}
		matches := 0
		for _, row := range doc.List("panes") {
			if r := row.(*Obj); r.Str("address") == "fixture/"+peer.Name && r.Str("state") == state && r.Str("branch") == "topic" {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("expected a single %s native row, got %d", state, matches)
		}
	}
}

func TestNativeInventoryDeduplicatesOnlyTheOwningPane(t *testing.T) {
	home := t.TempDir()
	store := nativeinbox.AtHome(home)
	peer, err := store.Register(home, "", "%2")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close(peer) })
	const owningPID = 42
	p := &P{home: home, procs: &procTable{ppid: map[int]int{peer.PID: owningPID, owningPID: 1}}}
	owner := &Row{ID: "%2", PID: owningPID, Attached: true}
	other := &Row{ID: "%3", PID: 43, Agent: "codex"}
	rows := p.withNative([]*Row{owner, other})
	if len(rows) != 2 || rows[0].Native == nil || !rows[0].Attached || rows[1] != other {
		t.Fatalf("owning pane was not replaced: %+v", rows)
	}
	owner.PID = 44
	rows = p.withNative([]*Row{owner, other})
	if len(rows) != 3 || rows[0] != owner || rows[1] != other || rows[2].Native == nil {
		t.Fatalf("unrelated pane was replaced: %+v", rows)
	}
}

func TestNativePeersPreservesLegacyCutover(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	_ = os.Unsetenv("TMUX")
	_ = os.Unsetenv("TMUX_PANE")
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("HEV_PEERS_STATE", "")
	t.Setenv("PEERS_SELF", "fixture")
	t.Setenv("PEERS_HOSTS", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", os.Getenv("PATH"))

	legacyState := filepath.Join(home, ".local", "state")
	legacyV2 := filepath.Join(legacyState, "peers-v2")
	if err := os.MkdirAll(legacyV2, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyState, "peers"), []byte("Peers v1 sealed.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyV2, "cutover-ready.json"),
		[]byte(`{"version": 2, "legacy_consumers_retired": true}`), 0600); err != nil {
		t.Fatal(err)
	}

	store := nativeinbox.AtHome(home)
	peer, err := store.Register(home, "v0.7", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close(peer) })

	if _, err := os.Stat(filepath.Join(home, ".hev", "peers")); !os.IsNotExist(err) {
		t.Fatalf("native registration hijacked legacy peers layout: %v", err)
	}

	var out, errOut bytes.Buffer
	if code := Main([]string{"send", peer.Name, "hello"}, strings.NewReader(""), &out, &errOut, nil); code != 0 {
		t.Fatalf("send on legacy cutover host failed (%d): %s", code, errOut.String())
	}
	msgs, err := store.Drain(peer, 4)
	if err != nil || len(msgs) != 1 || msgs[0].Body != "hello" {
		t.Fatalf("messages = %+v, %v", msgs, err)
	}
}

func TestNativeTmuxSessionKeptAndHevPaneNeverTypedInto(t *testing.T) {
	t.Setenv("PEERS_NO_SELF", "1")
	home := t.TempDir()
	store := nativeinbox.AtHome(home)
	peer, err := store.Register(home, "v0.7", "%29")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close(peer) })

	const owningPID = 42
	p := &P{home: home, stateDir: filepath.Join(home, ".hev", "peers"), procs: &procTable{ppid: map[int]int{peer.PID: owningPID, owningPID: 1}}}
	owner := &Row{ID: "%29", Session: "foh-kit-9", Window: "0", Pane: "0", PID: owningPID, Attached: true, Agent: "hev", Role: "foh"}
	rows := p.withNative([]*Row{owner})
	if len(rows) != 1 || rows[0].Session != "foh-kit-9" || rows[0].ID != "%29" || rows[0].Native == nil || rows[0].Native.Name != peer.Name {
		t.Fatalf("tmux session identity lost on native row: %+v", rows[0])
	}

	// sendTo must refuse to paste/type into a hev kit pane (both native and non-native), even with force=true.
	for _, tc := range []struct {
		name string
		row  *Row
	}{
		{"native", rows[0]},
		{"unregistered_hev", &Row{ID: "%30", Session: "foh-kit-10", PID: 50, Agent: "hev", Role: "foh"}},
	} {
		if _, _, err := p.sendTo(tc.row.Session, []*Row{tc.row}, "hdr", "msg", "sender", true, nil); err == nil || !strings.Contains(err.Error(), "never types into the pane") {
			t.Fatalf("%s: expected refusal without typing into pane, got %v", tc.name, err)
		}
	}
}
