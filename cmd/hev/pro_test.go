package main

import (
	"bytes"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
)

func TestProURLIsTagged(t *testing.T) {
	u, err := url.Parse(proURL("shared-key"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "hevlayer.com" || u.Fragment != "start-trial" || q.Get("utm_source") != "kit" || q.Get("utm_content") != "shared-key" {
		t.Fatalf("proURL = %s", u)
	}
}

func TestLicenseRequiredMessage(t *testing.T) {
	err := &layer.HTTPError{Status: 402, Body: []byte(`{"error":"license_required","feature":"agents"}`), Message: "raw"}
	msg := licenseRequiredMessage(err)
	if !strings.Contains(msg, "agents needs a hev layer pro license") || !strings.Contains(msg, "hev pro") {
		t.Fatalf("message %q", msg)
	}
	if licenseRequiredMessage(&layer.HTTPError{Status: 500, Message: "raw"}) != "" {
		t.Fatal("only a 402 license_required is rewritten")
	}
}

// Off a terminal, which is every test, hints never show; the agent and
// opt-out variables are checked before the terminal is.
func TestHintsStayOutOfAgentsAndPipes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, env := range append([]string{"HEV_NO_HINTS"}, agentEnv...) {
		t.Setenv(env, "")
	}
	if hintsAllowed() {
		t.Fatal("hints allowed off a terminal")
	}
	t.Setenv("CLAUDECODE", "1")
	if hintDue("shared-key", time.Now()) {
		t.Fatal("hint due inside Claude Code")
	}
}

func TestShowHintRecordsWhen(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var out bytes.Buffer
	now := time.Now()
	showHint(&out, "shared-key", "two machines", now)
	if !strings.Contains(out.String(), "two machines") || !strings.Contains(out.String(), "HEV_NO_HINTS") {
		t.Fatalf("hint output %q", out.String())
	}
	shown := readHints(filepath.Join(home, ".hev", "hints.json"))
	if shown["shared-key"].Unix() != now.Unix() {
		t.Fatalf("recorded %v, want %v", shown["shared-key"], now)
	}
}
