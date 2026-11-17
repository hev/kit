//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hev/kit/internal/layerenv"
)

// trap records every outbound HTTP request and every docker, op or gh
// invocation. The no-key path must make none of them.
type trap struct {
	mu       sync.Mutex
	requests []string
	server   *httptest.Server
	bin, log string
}

func newTrap(t *testing.T) *trap {
	t.Helper()
	tr := &trap{bin: t.TempDir()}
	tr.log = filepath.Join(tr.bin, "invocations.log")
	tr.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tr.mu.Lock()
		tr.requests = append(tr.requests, r.Method+" "+r.Host+r.URL.Path)
		tr.mu.Unlock()
		http.Error(w, "no outbound requests in file mode", http.StatusTeapot)
	}))
	t.Cleanup(tr.server.Close)
	for _, name := range []string{"docker", "op", "gh"} {
		script := fmt.Sprintf("#!/bin/sh\necho %s \"$@\" >> %q\nexit 1\n", name, tr.log)
		if err := os.WriteFile(filepath.Join(tr.bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return tr
}

func (tr *trap) check(t *testing.T) {
	t.Helper()
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.requests) > 0 {
		t.Fatalf("outbound requests in file mode: %v", tr.requests)
	}
	b, _ := os.ReadFile(tr.log)
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		// Reading the GitHub user from gh's local config is how kit names
		// the capturing operator in every mode; it is not a Layer request.
		if line != "" && !strings.HasPrefix(line, "gh config get") {
			t.Fatalf("external commands run in file mode:\n%s", b)
		}
	}
}

// claudeTranscript is one small Claude Code session.
func claudeTranscript(session, cwd string, at time.Time, prompt, reply string) string {
	ts := func(d time.Duration) string { return at.Add(d).UTC().Format(time.RFC3339) }
	lines := []map[string]any{
		{"type": "user", "uuid": "u1", "sessionId": session, "cwd": cwd, "timestamp": ts(0), "message": map[string]any{"role": "user", "content": prompt}},
		{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "sessionId": session, "cwd": cwd, "timestamp": ts(time.Second), "message": map[string]any{
			"role": "assistant", "model": "claude-sonnet-4-6", "content": []any{map[string]any{"type": "text", "text": reply}},
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 20}}},
	}
	var b strings.Builder
	for _, l := range lines {
		raw, _ := json.Marshal(l)
		b.Write(raw)
		b.WriteByte('\n')
	}
	return b.String()
}

func buildHev(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "hev")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

// noKeyEnv is a clean environment: a fresh HOME, no Layer key anywhere,
// every HTTP request routed to the trap, and docker/op/gh replaced by
// recorders.
func noKeyEnv(home string, tr *trap) []string {
	env := []string{
		"HOME=" + home, "USERPROFILE=" + home,
		"PATH=" + tr.bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HEV_CONFIG=" + filepath.Join(home, ".hev", "config.toml"),
		"HEVLAYER_ENDPOINT=" + tr.server.URL,
		"HTTP_PROXY=" + tr.server.URL, "HTTPS_PROXY=" + tr.server.URL, "NO_PROXY=", "no_proxy=",
		"HEV_NO_HINTS=1", "NO_COLOR=1", "TMPDIR=" + os.TempDir(),
	}
	skip := map[string]bool{}
	for _, name := range layerenv.Names() {
		skip[name] = true
	}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if skip[name] || strings.HasPrefix(name, "HEV") || strings.HasPrefix(name, "OP_") || name == "HOME" || name == "PATH" || strings.Contains(strings.ToLower(name), "proxy") {
			continue
		}
		if name == "GOCACHE" || name == "GOPATH" || name == "LANG" || name == "SHELL" || name == "USER" {
			env = append(env, kv)
		}
	}
	return env
}

func runHev(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hev %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// TestNoKeyCaptureListShowSearch is the file-mode acceptance: with no Layer
// key and no Docker, hevd captures into the file archive and hev traces
// list, show and search return what it captured. Any outbound request or
// docker invocation fails the test.
func TestNoKeyCaptureListShowSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the hev binary")
	}
	tr := newTrap(t)
	bin := buildHev(t)
	home := t.TempDir()
	env := noKeyEnv(home, tr)
	mustMkdir(t, filepath.Join(home, ".hev"))
	// The config names an endpoint (the trap) but no key: endpoint alone is
	// not Layer.
	mustWrite(t, filepath.Join(home, ".hev", "config.toml"), []byte(fmt.Sprintf("[layer]\nendpoint = %q\napi_key = \"\"\n[capture]\nscan_interval = \"1h\"\ninstructions = false\n", tr.server.URL)))
	session := "5e1f0c3a-9b7d-4c2e-8f10-2a3b4c5d6e7f"
	project := filepath.Join(home, ".claude", "projects", "-work-kit")
	mustMkdir(t, project)
	secret := "sk-ant-api03-" + strings.Repeat("A", 93) + "AA"
	mustWrite(t, filepath.Join(project, session+".jsonl"), []byte(claudeTranscript(session, "/work/kit", time.Now().Add(-time.Hour),
		"why did the preflight fail with HEVLAYER_API_KEY 401? my key is "+secret,
		"The preflight failed because the gateway rejected the stale key; exporting a fresh one fixed it.")))

	daemon := exec.Command(bin, "daemon")
	daemon.Env = env
	var daemonOut strings.Builder
	daemon.Stdout, daemon.Stderr = &daemonOut, &daemonOut
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	statusPath := filepath.Join(home, ".hev", "daemon-status.json")
	deadline := time.Now().Add(90 * time.Second)
	for {
		if _, err := os.Stat(statusPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			daemon.Process.Kill()
			daemon.Wait()
			log, _ := os.ReadFile(filepath.Join(home, ".hev", "hev.log"))
			t.Fatalf("hevd never finished a scan\n%s\n%s", daemonOut.String(), log)
		}
		time.Sleep(200 * time.Millisecond)
	}
	daemon.Process.Signal(syscall.SIGTERM)
	daemon.Wait()
	var status struct {
		UnitsIndexed     int    `json:"units_indexed"`
		LastError        string `json:"last_error"`
		MigrationPending bool   `json:"migration_pending"`
	}
	raw, _ := os.ReadFile(statusPath)
	if err := json.Unmarshal(raw, &status); err != nil || status.UnitsIndexed != 1 || status.LastError != "" || status.MigrationPending {
		log, _ := os.ReadFile(filepath.Join(home, ".hev", "hev.log"))
		t.Fatalf("status = %s (%v)\n%s", raw, err, log)
	}
	archive := filepath.Join(home, ".hev", "archive", "hev-traces")
	if _, err := os.Stat(filepath.Join(archive, "sessions", session[:2], session+".json")); err != nil {
		t.Fatalf("session not in the file archive: %v", err)
	}
	// Redaction ran before the write: the secret is nowhere on disk.
	filepath.Walk(archive, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, _ := os.ReadFile(path); strings.Contains(string(b), secret) {
				t.Errorf("unredacted secret in %s", path)
			}
		}
		return nil
	})

	list := runHev(t, bin, env, "traces", "list", "--since", "7d")
	if !strings.Contains(list, session[:8]) || !strings.Contains(list, "Sonnet 4.6") {
		t.Fatalf("list:\n%s", list)
	}
	show := runHev(t, bin, env, "traces", "show", session[:8])
	if !strings.Contains(show, "gateway rejected the stale key") {
		t.Fatalf("show:\n%s", show)
	}
	search := runHev(t, bin, env, "traces", "search", "--json", "--since", "7d", "--harness", "claude_code", "stale gateway key")
	jsonStart := strings.Index(search, "[")
	var hits []queryHit
	if jsonStart < 0 || json.Unmarshal([]byte(search[jsonStart:]), &hits) != nil || len(hits) == 0 || hits[0].SessionID != session {
		t.Fatalf("search:\n%s", search)
	}
	if !strings.Contains(search, "lexical") {
		t.Fatalf("search does not say it is lexical:\n%s", search)
	}
	also := runHev(t, bin, env, "traces", "search", "nothing matches this", "--also", "preflight 401")
	if !strings.Contains(also, "preflight") {
		t.Fatalf("--also:\n%s", also)
	}
	miss := runHev(t, bin, env, "traces", "search", "--harness", "codex", "preflight")
	if !strings.Contains(miss, "nothing found") {
		t.Fatalf("harness filter ignored:\n%s", miss)
	}

	// hev serve with no key serves the file archive.
	serve := exec.Command(bin, "serve", "--port", "0")
	serve.Env = env
	stdout, err := serve.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := serve.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { serve.Process.Kill(); serve.Wait() }()
	first := make([]byte, 256)
	n, _ := stdout.Read(first)
	url := strings.TrimSpace(strings.TrimPrefix(string(first[:n]), "hev read side: "))
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(url + "/api/sessions?window=7d")
	if err != nil {
		t.Fatal(err)
	}
	var sessions struct {
		Sessions []map[string]any `json:"sessions"`
	}
	json.NewDecoder(resp.Body).Decode(&sessions)
	resp.Body.Close()
	if len(sessions.Sessions) != 1 || sessions.Sessions[0]["id"] != session {
		t.Fatalf("hev serve sessions: %+v", sessions)
	}
	tr.check(t)
}

// TestWithKeyTracesUseLayerAndWriteNoFiles: with a key, list and search go
// to Layer as before and the archive directory is never created.
func TestWithKeyTracesUseLayerAndWriteNoFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the hev binary")
	}
	tr := newTrap(t)
	var mu sync.Mutex
	var paths []string
	layerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", 401)
			return
		}
		row := map[string]any{"id": "layer-session", "session_id": "layer-session", "harness": "codex", "start": time.Now().UnixMilli(), "text": "answer from layer", "$dist": 0.5}
		json.NewEncoder(w).Encode(map[string]any{"rows": []any{row}, "results": []any{map[string]any{"rows": []any{row}}}})
	}))
	defer layerSrv.Close()
	bin := buildHev(t)
	home := t.TempDir()
	env := noKeyEnv(home, tr)
	env = append(env, "HEVLAYER_API_KEY=test-key")
	for i, kv := range env {
		if strings.HasPrefix(kv, "HEVLAYER_ENDPOINT=") {
			env[i] = "HEVLAYER_ENDPOINT=" + layerSrv.URL
		}
		if strings.HasPrefix(kv, "HTTP_PROXY=") || strings.HasPrefix(kv, "HTTPS_PROXY=") {
			env[i] = strings.SplitN(kv, "=", 2)[0] + "="
		}
	}
	list := runHev(t, bin, env, "traces", "list", "--since", "7d")
	if !strings.Contains(list, "layer-se") {
		t.Fatalf("list:\n%s", list)
	}
	search := runHev(t, bin, env, "traces", "search", "answer")
	if !strings.Contains(search, "answer from layer") || strings.Contains(search, "lexical") {
		t.Fatalf("search:\n%s", search)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) < 2 {
		t.Fatalf("Layer was not asked: %v", paths)
	}
	if _, err := os.Stat(filepath.Join(home, ".hev", "archive")); !os.IsNotExist(err) {
		t.Fatalf("archive directory exists in Layer mode: %v", err)
	}
}
