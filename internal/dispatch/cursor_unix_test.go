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

func TestCursorLaunch(t *testing.T) {
	orchHome(t)
	allowChildProbes(t)
	cfg := writeDispatchConfig(t, "[harnesses.cursor.env_refs]\nCURSOR_API_KEY = \""+testGeminiRef+"\"\n")
	fakeHarnessSecrets(t, testGeminiSecret, nil)
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	exe := filepath.Join(bin, "cursor-agent")
	body := `#!/bin/sh
[ "$CURSOR_API_KEY" = 'KIT_TEST_CURSOR_EXPECTED' ] || exit 3
[ "$#" -eq 7 ] && [ "$1" = -p ] && [ "$2" = --force ] && [ "$3" = --output-format ] &&
[ "$4" = stream-json ] && [ "$5" = --model ] && [ "$6" = example ] || exit 4
printf '%s\n' '{"type":"system","subtype":"init","session_id":"example-session"}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"text":"partial"}]}}'
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"session_id":"example-session","result":"finished"}'
`
	// Use the same injected fixture value in the fake executable.
	body = strings.Replace(body, "KIT_TEST_CURSOR_EXPECTED", testGeminiSecret, 1)
	if err := os.WriteFile(exe, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(Root(), "cursor-worker")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	meta := Meta{Start: time.Now().UTC(), ConfigPath: cfg, LaunchResolved: true, LaunchEnv: []string{"PATH"},
		Order: Order{Name: "cursor-worker", Repo: "app", Harness: "cursor", Model: "example", Timeout: 1}}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), meta); err != nil {
		t.Fatal(err)
	}
	if err := superviseRound(meta.Name); err != nil {
		t.Fatal(err)
	}
	var r Result
	if err := readJSON(filepath.Join(dir, "result.json"), &r); err != nil || r.ExitCode != 0 || r.Session != "example-session" {
		t.Fatalf("result = %+v, %v", r, err)
	}
	result, err := readCursorResult(filepath.Join(dir, "log"), 0)
	if err != nil || result.Text == nil || *result.Text != "finished" {
		t.Fatalf("Cursor terminal result = %+v, %v", result, err)
	}
	assertNoSecretOnDisk(t, dir, testGeminiSecret)
	s := applyOrch(t)
	s.Harness, s.Model = "cursor", "example"
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := orchestratorSession(ctx, s, OrchestratorDir(s.Name), OrchestratorPacket{}); err != nil {
		t.Fatal(err)
	}
	assertNoSecretOnDisk(t, OrchestratorDir(s.Name), testGeminiSecret)
}
