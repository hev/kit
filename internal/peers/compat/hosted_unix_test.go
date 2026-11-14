//go:build !windows

package compat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/kitcapture"
	"github.com/hev/kit/pkg/kitapi/kitapitest"
)

func hostedConfig(t *testing.T, h *host, s *kitapitest.Server) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Join(h.home, ".hev"), 0700))
	must(t, os.WriteFile(filepath.Join(h.home, ".hev", "config.toml"), []byte(fmt.Sprintf("[kit]\napi=%q\nkey=\"key\"\nteam_id=\"team\"\n[capture]\nredact_salt=%q\n", s.API(), strings.Repeat("ab", 32))), 0600))
}
func hostedServer(t *testing.T) *kitapitest.Server {
	return kitapitest.New(t, "key", kitapitest.Identity{Host: "local", TeamID: "team", AuthorID: "member"})
}
func TestHostedFailureDoesNotFailLocalBeaconOrSend(t *testing.T) {
	c := newGoCluster(t)
	h := c.host("local", "go")
	h.agent("sender", false)
	h.agent("reader", false)
	h.ok(goImpl, "", "", "inbox", "on", "reader")
	s := hostedServer(t)
	hostedConfig(t, h, s)
	s.SetDown(true)
	start := time.Now()
	h.ok(goImpl, "", "", "beacon")
	if time.Since(start) > 5*time.Second {
		t.Fatal("hosted failure delayed beacon")
	}
	h.ok(goImpl, "sender", "", "send", "reader", "hello")
	// The actual worker path observes 503 but never changes local delivery.
	h.ok(goImpl, "", "", "_hosted-flush")
	out := h.ok(goImpl, "reader", "", "inbox", "--peek", "--json")
	if !strings.Contains(out, "hello") {
		t.Fatal(out)
	}
	s.SetDown(false)
	h.ok(goImpl, "", "", "_hosted-flush")
	page, e := s.Client("key").PeerMessages(context.Background(), nil)
	if e != nil || len(page.Messages) != 1 {
		t.Fatal(page, e)
	}
	presence, e := s.Client("key").PeerPresence(context.Background())
	if e != nil || len(presence.Machines) != 1 {
		t.Fatal(presence, e)
	}
}
func TestHostedDefaultPeersOutputUnchanged(t *testing.T) {
	c := newGoCluster(t)
	h := c.host("local", "go")
	h.agent("sender", false)
	h.agent("reader", false)
	// Freeze the fixture's display-only activity field, so byte comparison
	// does not cross a natural seconds boundary while enabling the config.
	realTmux, e := exec.LookPath("tmux")
	must(t, e)
	shim := fmt.Sprintf(`#!/usr/bin/python3
import os,sys
binary=%q
args=[arg.replace("#{window_activity}","0") for arg in sys.argv[1:]]
os.execv(binary,[binary]+args)
`, realTmux)
	must(t, os.WriteFile(filepath.Join(h.bin, "tmux"), []byte(shim), 0755))
	before := h.ok(goImpl, "", "", "ls")
	s := hostedServer(t)
	hostedConfig(t, h, s)
	after := h.ok(goImpl, "", "", "ls")
	if before != after {
		t.Fatalf("default output changed:\n%s\n%s", before, after)
	}
	if s.Requests("GET /v1/whoami") != 0 || s.Requests("GET /v1/peers/presence") != 0 {
		t.Fatal("default ls sent hosted traffic")
	}
	h.ok(goImpl, "", "", "beacon")
	h.ok(goImpl, "", "", "_hosted-flush")
	team := h.ok(goImpl, "", "", "--team")
	if !strings.Contains(team, `"host": "local"`) {
		t.Fatal(team)
	}
	log := h.ok(goImpl, "", "", "log", "--machine", "local", "--limit", "1")
	if !strings.Contains(log, `"messages"`) {
		t.Fatal(log)
	}
}
func TestHostedChatLeavesPythonStateAndWireUnchanged(t *testing.T) {
	c := newGoCluster(t)
	h := c.host("local", "go")
	h.agent("sender", false)
	h.agent("reader", false)
	h.ok(goImpl, "", "", "inbox", "on", "reader")
	s := hostedServer(t)
	hostedConfig(t, h, s)
	out := h.ok(goImpl, "sender", "", "chat", "new", "reader", "--title", "review", "-m", "chat body")
	if !strings.Contains(out, "chat #") {
		t.Fatal(out)
	}
	// Sender chat history is the exact reference field set, with no hosted ID.
	raw, e := os.ReadFile(filepath.Join(h.home, ".local/state/peers-v2/sent.jsonl"))
	must(t, e)
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		o, e := LoadObj([]byte(line))
		must(t, e)
		if strings.HasPrefix(o.Str("to"), "#") && (len(o.Keys()) != 7 || o.Has("id")) {
			t.Fatal("chat history wire changed", line)
		}
	}
	// Recipient inbox must not acquire the sender's hosted ID. Its existing
	// per-recipient delivery ID is still allowed and remains independent.
	inbox := h.ok(goImpl, "reader", "", "inbox", "--peek", "--json")
	var obj map[string]any
	must(t, json.Unmarshal([]byte(inbox), &obj))
	snapshot := map[string][]byte{}
	root := filepath.Join(h.home, ".local/state/peers-v2")
	must(t, filepath.Walk(root, func(path string, info os.FileInfo, e error) error {
		if e == nil && !info.IsDir() {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			snapshot[path] = b
		}
		return e
	}))
	h.ok(goImpl, "", "", "_hosted-flush")
	h.ok(goImpl, "", "", "_hosted-flush")
	for path, b := range snapshot {
		after, e := os.ReadFile(path)
		must(t, e)
		if string(b) != string(after) {
			t.Fatal("hosted flush rewrote Python state", path)
		}
	}
	page, e := s.Client("key").PeerMessages(context.Background(), nil)
	must(t, e)
	if len(page.Messages) != 1 || page.Messages[0].Kind != "chat" || len(page.Messages[0].ID) != 32 || len(page.Messages[0].Recipients) != 1 {
		t.Fatal(page)
	}
	if strings.Contains(string(raw), page.Messages[0].ID) || strings.Contains(inbox, page.Messages[0].ID) {
		t.Fatal("hosted ID leaked into compatible state")
	}
	// The separate team-bound outbox is fully drained and no failed entries.
	o, e := kitcapture.OpenOutbox(filepath.Join(h.home, ".hev/kit/peers"), "team")
	must(t, e)
	names, e := o.Pending()
	must(t, e)
	if len(names) != 0 {
		t.Fatal(names)
	}
}
func TestHostedBeaconLoginGate(t *testing.T) {
	c := newGoCluster(t)
	h := c.host("local", "go")
	h.agent("sender", false)
	s := hostedServer(t)
	h.ok(goImpl, "", "", "beacon")
	h.ok(goImpl, "", "", "_hosted-flush")
	if s.Requests("POST /v1/peers/presence") != 0 {
		t.Fatal("signed-out beacon")
	}
	hostedConfig(t, h, s)
	h.ok(goImpl, "", "", "beacon")
	h.ok(goImpl, "", "", "_hosted-flush")
	if s.Requests("POST /v1/peers/presence") != 1 {
		t.Fatal("signed-in beacon missing")
	}
}
