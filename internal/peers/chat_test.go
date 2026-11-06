//go:build !windows

package peers

import (
	"context"
	"github.com/hev/kit/internal/dispatch"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChatBothDirections(t *testing.T) {
	e, call := fixture(t)
	original := e.Command
	e.Command = func(ctx context.Context, name string, in io.Reader, args ...string) (string, error) {
		if name == "tmux" && args[0] == "list-panes" {
			return "desk\t%7\t700\tclaude\t/work\tdesk\ton\nother\t%8\t701\tclaude\t/else\tother\ton\n", nil
		}
		return original(ctx, name, in, args...)
	}
	s, err := call("", "chat", "new", "local/other", "--title", "review", "-m", "first message")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(s), "\n")
	id := lines[len(lines)-1]
	if _, err = decodeChat(id); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_PANE", "%8")
	s, err = call("", "chat", "ls")
	if err != nil || !strings.Contains(s, id) {
		t.Fatal(s, err)
	}
	if _, err = call("", "chat", "say", id, "reply from other"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_PANE", "%7")
	s, err = call("", "chat", "show", id)
	if err != nil || !strings.Contains(s, "reply from other") || !strings.Contains(s, "first message") {
		t.Fatal(s, err)
	}
	// Repeated discovery does not grow the retained index.
	before, err := os.ReadFile(e.path("desk.chats"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = call("", "chat", "ls"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(e.path("desk.chats"))
	if len(after) != len(before) {
		t.Fatal("discovery appended duplicate routes")
	}
}

func TestChatDispatchInboxRoundTrip(t *testing.T) {
	_, call := fixture(t)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(dispatch.Root(), "expediter")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	meta := dispatch.Meta{Order: dispatch.Order{Name: "expediter", Harness: "codex", Host: "local"}}
	if err := dispatch.WriteJSON(filepath.Join(dir, "meta.json"), meta); err != nil {
		t.Fatal(err)
	}
	// Headless recipient has a stable run identity even without a tmux pane.
	s, err := call("", "chat", "new", "local/expediter", "--title", "headless", "-m", "check work")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimSpace(s), "\n")
	id := parts[len(parts)-1]
	t.Setenv("TMUX_PANE", "")
	t.Chdir(filepath.Join(dir, "wt"))
	if s, err = call("", "chat", "ls"); err != nil || !strings.Contains(s, id) {
		t.Fatal(s, err)
	}
	if _, err = call("", "chat", "say", id, "headless reply"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_PANE", "%7")
	s, err = call("", "chat", "show", id)
	if err != nil || !strings.Contains(s, "headless reply") || !strings.Contains(s, "local/d-expediter") {
		t.Fatal(s, err)
	}
}
