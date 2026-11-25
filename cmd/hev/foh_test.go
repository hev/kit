package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hev/kit/internal/foh"
	"github.com/hev/kit/internal/secrets"
)

// runHevChild runs this test binary as hev (see TestMain) with an isolated
// HOME and no inherited provider keys.
func runHevChild(t *testing.T, home string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
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

func TestFOHStaleLegacyKitKeyWithCodexSub(t *testing.T) {
	hkKey := "hk_" + strings.Repeat("Ab9_-1", 7) + "A"
	const access = "eyJhbGciOiJub25lIn0.eyJleHAiOjQxMDI0NDQ4MDB9.c2lnbmF0dXJlLWxr"
	const refresh = "rt_legacyCodexREFRESH0123456789"

	var kitHits, codexHits int
	kitSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kitHits++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer kitSrv.Close()

	codexSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		codexHits++
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{}}\n\n")
	}))
	defer codexSrv.Close()

	setupCodex := func(home string) {
		codexDir := filepath.Join(home, ".codex")
		os.MkdirAll(codexDir, 0o700)
		os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{"tokens":{"access_token":"`+access+`","refresh_token":"`+refresh+`","account_id":"acct-legacy-1234"}}`), 0o600)
	}
	setupStaleKit := func(home string) {
		cfgDir := filepath.Join(home, ".hev")
		os.MkdirAll(cfgDir, 0o700)
		os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(fmt.Sprintf("[kit]\napi = %q\nkey = %q\n", kitSrv.URL, hkKey)), 0o600)
	}

	// Permitted side: stale implicit [kit].key = hk_... + valid Codex login runs cleanly on codex-sub
	// with a concise secret-free warning on stderr and zero hits to the Club Kit server.
	home := t.TempDir()
	setupCodex(home)
	setupStaleKit(home)
	out, errOut, code := runHevChild(t, home, []string{"HEV_FOH_CODEX_BASE_URL=" + codexSrv.URL}, "foh", "--once", "ping")
	if code != 0 || strings.TrimSpace(out) != "pong" {
		t.Fatalf("stale [kit].key + codex: code=%d out=%q stderr=%q", code, out, errOut)
	}
	if kitHits != 0 || codexHits != 1 {
		t.Fatalf("hits: kit=%d (want 0) codex=%d (want 1)", kitHits, codexHits)
	}
	if !strings.Contains(errOut, "hk_") || !strings.Contains(errOut, "hev login") {
		t.Fatalf("expected warning naming hk_ and hev login on stderr, got %q", errOut)
	}
	for _, leak := range []string{hkKey, hkKey[3:], access, refresh, "acct-legacy-1234"} {
		if strings.Contains(errOut, leak) || strings.Contains(out, leak) {
			t.Fatalf("output leaked %q: out=%q stderr=%q", leak, out, errOut)
		}
	}
	sessions, _ := filepath.Glob(filepath.Join(home, ".hev", "foh", "sessions", "*.jsonl"))
	if len(sessions) != 1 {
		t.Fatalf("sessions = %v", sessions)
	}
	b, _ := os.ReadFile(sessions[0])
	if !strings.Contains(string(b), `"provider":"codex-sub"`) {
		t.Fatalf("session missing codex-sub provider: %s", b)
	}
	for _, leak := range []string{hkKey, hkKey[3:], access, refresh, "acct-legacy-1234"} {
		if strings.Contains(string(b), leak) {
			t.Fatalf("session leaked %q: %s", leak, b)
		}
	}

	// Refused side (a): stale implicit [kit].key = hk_... alone (no Codex login) fails with ErrLegacyKitKey guidance and zero hits.
	homeAlone := t.TempDir()
	setupStaleKit(homeAlone)
	_, errOut, code = runHevChild(t, homeAlone, []string{"HEV_FOH_CODEX_BASE_URL=" + codexSrv.URL}, "foh", "--once", "ping")
	if code == 0 || kitHits != 0 || codexHits != 1 {
		t.Fatalf("stale [kit].key alone: code=%d kitHits=%d codexHits=%d stderr=%q", code, kitHits, codexHits, errOut)
	}
	for _, want := range []string{"hk_", "hev login", "hev_", "OPENROUTER_API_KEY"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("stale [kit].key alone missing %q in stderr: %q", want, errOut)
		}
	}
	if strings.Contains(errOut, hkKey) || strings.Contains(errOut, hkKey[3:]) {
		t.Fatalf("stale [kit].key alone leaked secret: %q", errOut)
	}

	// Refused side (b): explicit HEV_API_KEY=hk_... + valid Codex login MUST still fail closed with zero hits on either server.
	for _, envName := range []string{"HEV_API_KEY", "OPENROUTER_API_KEY"} {
		homeExplicit := t.TempDir()
		setupCodex(homeExplicit)
		_, errOut, code = runHevChild(t, homeExplicit, []string{envName + "=" + hkKey, "HEV_FOH_CODEX_BASE_URL=" + codexSrv.URL}, "foh", "--once", "ping")
		if code == 0 || kitHits != 0 || codexHits != 1 {
			t.Fatalf("explicit %s=hk_ + codex: code=%d kitHits=%d codexHits=%d stderr=%q", envName, code, kitHits, codexHits, errOut)
		}
		if !strings.Contains(errOut, "hev login") || !strings.Contains(errOut, "OPENROUTER_API_KEY") {
			t.Fatalf("explicit %s=hk_ missing guidance: %q", envName, errOut)
		}
		if strings.Contains(errOut, hkKey) || strings.Contains(errOut, hkKey[3:]) {
			t.Fatalf("explicit %s=hk_ leaked secret: %q", envName, errOut)
		}
	}
}

func TestFOHScopedCredentialRefCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake credential resolver uses a shell script")
	}
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(filepath.Join(home, ".hev"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	// Generate the sentinel only while resolving the reference over stdin.
	const provider = `#!/bin/sh
printf '%s\n' "$*" >> "$OP_ARGV"
[ "$#" = 1 ] && [ "$1" = inject ] || exit 2
awk '{
  if (sub(/^\{\{ op:\/\/vault\/alpha\/credential \}\}/, "")) c="A"
  else if (sub(/^\{\{ op:\/\/vault\/beta\/credential \}\}/, "")) c="B"
  else if (match($0, /extra-[C-H]/)) {
    c=substr($0,RSTART+6,1)
    sub(/^\{\{ op:\/\/vault\/extra-[C-H]\/credential \}\}/, "")
  }
  else exit 2
  printf "hev_"; for(i=0;i<32;i++) printf "%s",c; printf "%s",$0
}'
`
	if err := os.WriteFile(filepath.Join(bin, "op"), []byte(provider), 0700); err != nil {
		t.Fatal(err)
	}
	keyA := "hev_" + strings.Repeat("A", 32)
	keyB := "hev_" + strings.Repeat("B", 32)
	var turnSteps atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		workspace := ""
		switch key {
		case keyA:
			workspace = "team-a"
		case keyB:
			workspace = "team-b"
		default:
			for letter := byte('C'); letter <= 'H'; letter++ {
				if key == "hev_"+strings.Repeat(string(letter), 32) {
					workspace = "team-" + strings.ToLower(string(letter))
				}
			}
		}
		if workspace == "" {
			t.Error("hosted request did not use a configured credential")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/whoami":
			fmt.Fprintf(w, `{"author":{"id":"user"},"team":{"id":%q}}`, workspace)
		case "/v1/inference/models":
			fmt.Fprintf(w, `{"data":[{"id":"test/model","name":%q}]}`, key)
		case "/v1/inference/chat/completions":
			var request foh.ChatRequest
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				t.Error("malformed synthetic inference request")
				return
			}
			if request.Model == "test/multi-step" {
				step := turnSteps.Add(1)
				if workspace != "team-a" {
					t.Error("multi-step turn changed credentials")
				}
				if step <= 7 {
					fmt.Fprintf(w, "data: "+`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-%d","type":"function","function":{"name":"glob","arguments":"{\"pattern\":\"*.none\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\ndata: [DONE]\n\n", step)
					return
				}
			}
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\ndata: [DONE]\n\n", workspace+" "+key)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg := `[kit]
api = %q
[[foh.key_scopes]]
name = "alpha"
account = "user"
workspace = "team-a"
api_key_ref = "op://vault/alpha/credential"
[[foh.key_scopes]]
name = "beta"
account = "user"
workspace = "team-b"
api_key_ref = "op://vault/beta/credential"
`
	if err := os.WriteFile(filepath.Join(home, ".hev", "config.toml"), []byte(fmt.Sprintf(cfg, server.URL)), 0600); err != nil {
		t.Fatal(err)
	}
	argv := filepath.Join(home, "hev-argv")
	opArgv := filepath.Join(home, "op-argv")
	env := []string{
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HEV_SECRETS_CACHE=off",
		"OP_SERVICE_ACCOUNT_TOKEN=synthetic-op-service-token",
		"HEV_TEST_ARGV_LOG=" + argv,
		"OP_ARGV=" + opArgv,
		"HEV_API_KEY=unused-process-credential",
	}
	_, stderr, code := runHevChild(t, home, env, "foh", "--once", "ping")
	if code == 0 || !strings.Contains(stderr, "select a configured account") {
		t.Fatal("CLI did not reject the missing selection")
	}
	for _, scope := range []struct{ name, workspace string }{{"alpha", "team-a"}, {"beta", "team-b"}} {
		out, _, code := runHevChild(t, home, env, "foh", "--once", "ping", "--model", "scope/"+scope.name+"/test/model")
		if code != 0 || strings.TrimSpace(out) != scope.workspace+" [REDACTED]" {
			t.Fatal("CLI did not route to the requested workspace or leaked output")
		}
	}
	calls, err := os.ReadFile(opArgv)
	if err != nil || string(calls) != "inject\ninject\n" {
		t.Fatal("credential resolver did not use exactly one argument-free reference injection per scope")
	}
	// Each workload starts with its own empty ledger and the default policy.
	newHome := func(config string) string {
		t.Helper()
		h := t.TempDir()
		if err := os.Mkdir(filepath.Join(h, ".hev"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(h, ".hev", "config.toml"), []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
		return h
	}
	burstHome := newHome(fmt.Sprintf(cfg, server.URL))
	contendedHome := newHome(fmt.Sprintf(cfg, server.URL))
	turnHome := newHome(fmt.Sprintf(cfg, server.URL))
	catalogConfig := fmt.Sprintf(cfg, server.URL)
	for letter := 'C'; letter <= 'H'; letter++ {
		catalogConfig += fmt.Sprintf(`[[foh.key_scopes]]
name = "extra-%s"
account = "user"
workspace = "team-%s"
api_key_ref = "op://vault/extra-%c/credential"
`, strings.ToLower(string(letter)), strings.ToLower(string(letter)), letter)
	}
	catalogHome := newHome(catalogConfig)
	t.Setenv("HOME", catalogHome)
	t.Setenv("USERPROFILE", catalogHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(catalogHome, ".config"))
	t.Setenv("HEV_SECRETS_CACHE", "off")
	t.Setenv("HEV_SECRETS_POLICY", "")
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(catalogHome, ".hev", "secrets", "op-reads.log"))
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "synthetic-op-service-token")
	t.Setenv("OP_ARGV", filepath.Join(catalogHome, "op-argv"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	config, err := foh.LoadConfig(filepath.Join(catalogHome, ".hev", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := config.ScopedInference()
	if err != nil {
		t.Fatal(err)
	}

	child := func(h, scope, model, workspace string) {
		childEnv := append(append([]string(nil), env...),
			"OP_ARGV="+filepath.Join(h, "op-argv"), "HEV_TEST_ARGV_LOG="+filepath.Join(h, "hev-argv"))
		out, stderr, code := runHevChild(t, h, childEnv, "foh", "--once", "ping", "--model", "scope/"+scope+"/"+model)
		if code != 0 || !strings.HasSuffix(strings.TrimSpace(out), workspace+" [REDACTED]") ||
			strings.Contains(stderr, keyA) || strings.Contains(stderr, keyB) {
			t.Errorf("%s did not complete with the selected workspace and redacted output (exit %d)", model, code)
		}
	}
	burst := func(h string) {
		var calls sync.WaitGroup
		for i := range 8 {
			calls.Go(func() {
				if i%2 == 0 {
					child(h, "alpha", "test/model", "team-a")
				} else {
					child(h, "beta", "test/model", "team-b")
				}
			})
		}
		calls.Wait()
	}
	var workloads sync.WaitGroup
	workloads.Go(func() { burst(burstHome) })
	workloads.Go(func() {
		// Model three reads that a continuing turn charged before the cold
		// processes reached the ledger. The last cold call needs window three.
		for range 3 {
			child(contendedHome, "alpha", "test/model", "team-a")
		}
		burst(contendedHome)
	})
	workloads.Go(func() { child(turnHome, "alpha", "test/multi-step", "team-a") })
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	models, _, catalogErr := catalog.Models(ctx, false)
	workloads.Wait()
	if catalogErr != nil || len(models) != 8 {
		t.Fatalf("the eight-scope catalog did not finish under the default budget: %d models, %v", len(models), catalogErr)
	}
	for _, scope := range config.KeyScopes {
		found := false
		for _, model := range models {
			if model.ID == "scope/"+scope.Name+"/test/model" && model.Name == "[REDACTED] ("+scope.Name+")" {
				found = true
			}
		}
		if !found {
			t.Errorf("catalog omitted the redacted model for %s", scope.Name)
		}
	}
	if turnSteps.Load() != 8 {
		t.Errorf("multi-step turn made %d inference requests, want 8", turnSteps.Load())
	}
	for _, workload := range []struct {
		home  string
		reads int
	}{{burstHome, 8}, {contendedHome, 11}, {turnHome, 8}, {catalogHome, 8}} {
		calls, err := os.ReadFile(filepath.Join(workload.home, "op-argv"))
		if err != nil || string(calls) != strings.Repeat("inject\n", workload.reads) {
			t.Errorf("expected %d argument-free credential reads", workload.reads)
		}
		data, err := os.ReadFile(filepath.Join(workload.home, ".hev", "secrets", "budget.json"))
		var ledger struct{ Spends []secrets.Spend }
		if err != nil || json.Unmarshal(data, &ledger) != nil || len(ledger.Spends) != workload.reads {
			t.Errorf("expected %d charged reads in one isolated ledger", workload.reads)
			continue
		}
		for _, spend := range ledger.Spends {
			var used int
			for _, other := range ledger.Spends {
				age := spend.At.Sub(other.At)
				if age >= 0 && age < time.Minute && spend.Caller == other.Caller {
					used += other.Requests
				}
				if spend.Caller != other.Caller {
					t.Error("workload did not share one caller budget")
				}
			}
			if used > 5 {
				t.Error("workload exceeded five charged reads in a rolling minute")
			}
		}
	}
	for _, h := range []string{home, burstHome, contendedHome, turnHome} {
		data, err := os.ReadFile(filepath.Join(h, "hev-argv"))
		if err != nil || len(data) == 0 {
			t.Error("missing CLI process argument evidence")
		}
	}
	for _, h := range []string{home, burstHome, contendedHome, turnHome, catalogHome} {
		err = filepath.WalkDir(h, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || !entry.Type().IsRegular() {
				return err
			}
			data, err := os.ReadFile(path)
			for letter := 'A'; letter <= 'H'; letter++ {
				if bytes.Contains(data, []byte("hev_"+strings.Repeat(string(letter), 32))) {
					t.Error("CLI trace, process arguments, or another file contained a credential")
				}
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
