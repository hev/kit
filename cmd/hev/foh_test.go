package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runHevChild runs this test binary as hev (see TestMain) with an isolated
// HOME and no inherited provider keys.
func runHevChild(t *testing.T, home string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	base := []string{"HEV_TEST_AS_HEV=1", "HOME=" + home, "USERPROFILE=" + home, "PATH=" + os.Getenv("PATH"), "TMUX_TMPDIR=" + t.TempDir()}
	if v := os.Getenv("SYSTEMROOT"); v != "" {
		base = append(base, "SYSTEMROOT="+v)
	}
	cmd.Env = append(base, env...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code = 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), errOut.String(), code
}

func TestFOHShowMascot(t *testing.T) {
	home := t.TempDir()
	out, errOut, code := runHevChild(t, home, nil, "foh", "--show-mascot")
	if code != 0 || !strings.Contains(out, "\x1b[38;2;") || strings.Count(strings.TrimRight(out, "\n"), "\n") != 19 {
		t.Fatalf("default: code=%d stderr=%q lines=%d", code, errOut, strings.Count(out, "\n"))
	}
	for _, tc := range []struct {
		name string
		env  []string
		args []string
	}{
		{"--no-color", nil, []string{"foh", "--show-mascot", "--no-color"}},
		{"NO_COLOR", []string{"NO_COLOR=1"}, []string{"foh", "--show-mascot"}},
	} {
		out, errOut, code := runHevChild(t, home, tc.env, tc.args...)
		if code != 0 || strings.ContainsRune(out, '\x1b') || !strings.Contains(out, "@") {
			t.Fatalf("%s: code=%d out=%q stderr=%q", tc.name, code, out, errOut)
		}
	}
}

func TestFOHOnce(t *testing.T) {
	const key = "sk-or-v1-cmdtest000000000000000000000000"
	var gotModel, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		sc := bufio.NewScanner(r.Body)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			if i := strings.Index(sc.Text(), `"model":"`); i >= 0 {
				rest := sc.Text()[i+9:]
				gotModel = rest[:strings.IndexByte(rest, '"')]
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	home := t.TempDir()
	out, errOut, code := runHevChild(t, home, []string{"OPENROUTER_API_KEY=" + key, "HEV_FOH_BASE_URL=" + srv.URL},
		"foh", "--once", "ping", "--model", "test/model")
	if code != 0 || strings.TrimSpace(out) != "pong" {
		t.Fatalf("code=%d out=%q stderr=%q", code, out, errOut)
	}
	if gotModel != "test/model" || gotAuth != "Bearer "+key {
		t.Fatalf("model=%q auth ok=%v", gotModel, gotAuth == "Bearer "+key)
	}
	sessions, _ := filepath.Glob(filepath.Join(home, ".hev", "foh", "sessions", "*.jsonl"))
	if len(sessions) != 1 {
		t.Fatalf("sessions = %v", sessions)
	}
	b, _ := os.ReadFile(sessions[0])
	if strings.Contains(string(b), key) || !strings.Contains(string(b), `"model":"test/model"`) {
		t.Fatalf("session = %s", b)
	}

	// No key: a clear error and a non-zero exit, no session.
	home2 := t.TempDir()
	_, errOut, code = runHevChild(t, home2, nil, "foh", "--once", "ping")
	if code == 0 || !strings.Contains(errOut, "OPENROUTER_API_KEY") {
		t.Fatalf("no key: code=%d stderr=%q", code, errOut)
	}
}

// TestFOHOnceCodexSub runs hev foh on a Codex ChatGPT login alone: no
// OpenRouter key, the default model falls back to a codex/ model, and the
// trace stamps provider codex-sub without any credential. A Claude login in
// the same HOME is never used.
func TestFOHOnceCodexSub(t *testing.T) {
	const access = "eyJhbGciOiJub25lIn0.eyJleHAiOjQxMDI0NDQ4MDB9.c2lnbmF0dXJlLWNtZA"
	const refresh = "rt_cmdREFRESHsecret0123456789"
	var gotModel, gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		var body struct {
			Model string `json:"model"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{}}\n\n")
	}))
	defer srv.Close()
	home := t.TempDir()
	codexDir := filepath.Join(home, ".codex")
	os.MkdirAll(codexDir, 0o700)
	os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{"tokens":{"access_token":"`+access+`","refresh_token":"`+refresh+`","account_id":"acct-cmd-1234"}}`), 0o600)
	os.MkdirAll(filepath.Join(home, ".claude"), 0o700)
	os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"sk-ant-oat01-NEVERUSED0000000000000"}}`), 0o600)

	out, errOut, code := runHevChild(t, home, []string{"HEV_FOH_CODEX_BASE_URL=" + srv.URL}, "foh", "--once", "ping")
	if code != 0 || strings.TrimSpace(out) != "pong" {
		t.Fatalf("code=%d out=%q stderr=%q", code, out, errOut)
	}
	if gotPath != "/responses" || gotModel != "gpt-6-astra" || gotAuth != "Bearer "+access {
		t.Fatalf("path=%q model=%q auth ok=%v", gotPath, gotModel, gotAuth == "Bearer "+access)
	}
	sessions, _ := filepath.Glob(filepath.Join(home, ".hev", "foh", "sessions", "*.jsonl"))
	if len(sessions) != 1 {
		t.Fatalf("sessions = %v", sessions)
	}
	b, _ := os.ReadFile(sessions[0])
	for _, s := range []string{access, refresh, "acct-cmd-1234", "NEVERUSED"} {
		if strings.Contains(string(b), s) {
			t.Fatalf("session leaked %q: %s", s, b)
		}
	}
	if !strings.Contains(string(b), `"model":"codex/gpt-6-astra","provider":"codex-sub"`) {
		t.Fatalf("session lacks provider stamp: %s", b)
	}

	// CODEX_HOME pointing at the Claude directory finds no login: the run
	// fails cleanly and names both ways to sign in.
	_, errOut, code = runHevChild(t, home, []string{"CODEX_HOME=" + filepath.Join(home, ".claude"), "HEV_FOH_CODEX_BASE_URL=" + srv.URL}, "foh", "--once", "ping")
	if code == 0 || !strings.Contains(errOut, "OPENROUTER_API_KEY") || !strings.Contains(errOut, "codex login") || strings.Contains(errOut, "NEVERUSED") {
		t.Fatalf("claude dir: code=%d stderr=%q", code, errOut)
	}
}

func TestFOHOnceClubKitHostedAndLegacyKeyFailClosed(t *testing.T) {
	hevKey := "hev_" + strings.Repeat("a1B2", 8)
	hkKey := "hk_" + strings.Repeat("Ab9_-1", 7) + "A"

	var gotPath, gotAuth string
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hosted pong %s\"}}]}\n\ndata: [DONE]\n\n", hevKey)
	}))
	defer srv.Close()

	// Signed in via [kit] table in config.toml: routes to <kit_api>/v1/inference and scrubs hev_ key from session trace.
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".hev")
	os.MkdirAll(cfgDir, 0o700)
	os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(fmt.Sprintf("[kit]\napi = %q\nkey = %q\n", srv.URL, hevKey)), 0o600)

	out, errOut, code := runHevChild(t, home, nil, "foh", "--once", "ping", "--model", "test/model")
	if code != 0 || !strings.Contains(out, "hosted pong") {
		t.Fatalf("hosted code=%d out=%q stderr=%q", code, out, errOut)
	}
	if gotPath != "/v1/inference/chat/completions" || gotAuth != "Bearer "+hevKey {
		t.Fatalf("path=%q auth ok=%v", gotPath, gotAuth == "Bearer "+hevKey)
	}
	sessions, _ := filepath.Glob(filepath.Join(home, ".hev", "foh", "sessions", "*.jsonl"))
	if len(sessions) != 1 {
		t.Fatalf("sessions = %v", sessions)
	}
	b, _ := os.ReadFile(sessions[0])
	if strings.Contains(string(b), hevKey) || strings.Contains(string(b), hevKey[4:]) {
		t.Fatalf("session leaked hev_ key: %s", b)
	}

	// Legacy hk_ key fails closed before any network request and never leaks the secret.
	hitsBefore := hits
	homeLegacy := t.TempDir()
	os.MkdirAll(filepath.Join(homeLegacy, ".hev"), 0o700)
	os.WriteFile(filepath.Join(homeLegacy, ".hev", "config.toml"), []byte(fmt.Sprintf("[kit]\napi = %q\nkey = %q\n", srv.URL, hkKey)), 0o600)
	_, errOut, code = runHevChild(t, homeLegacy, nil, "foh", "--once", "ping")
	if code == 0 || hits != hitsBefore {
		t.Fatalf("legacy hk_ key: code=%d hits=%d (before=%d) stderr=%q", code, hits, hitsBefore, errOut)
	}
	if strings.Contains(errOut, hkKey) || strings.Contains(errOut, hkKey[3:]) {
		t.Fatalf("legacy hk_ error leaked secret: %q", errOut)
	}
	if !strings.Contains(errOut, "hev login") || !strings.Contains(errOut, "OPENROUTER_API_KEY") {
		t.Fatalf("legacy hk_ error missing guidance: %q", errOut)
	}
}
