package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/daemon"
)

type inviteFixture struct {
	home, log                            string
	server                               *httptest.Server
	mu                                   sync.Mutex
	events                               []string
	redeemStatus, keyStatus, leaveStatus int
	code, hostname                       string
	machines                             int
	rejoining                            bool
	key                                  string
	skillsInstalled                      bool
}

func inviteSandbox(t *testing.T) *inviteFixture {
	t.Helper()
	onOS(t, "darwin")
	home, log := sandbox(t, "1") // Docker unavailable: join must never call it.
	f := &inviteFixture{home: home, log: log, machines: 1, key: "invite-test-key"}
	t.Setenv("FAKE_UNLOADED", "1")
	bin := os.Getenv("PATH")
	for _, name := range []string{"open", "xdg-open"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho browser >> \""+log+"\"\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Record exactly which config the daemon sees at shutdown and startup.
	script := `#!/bin/sh
 echo "launchctl $*" >> "$INVITE_CALLS"
 case "$1" in
 print) if [ -n "$FAKE_UNLOADED" ] || [ -f "$INVITE_CALLS.unloaded" ]; then exit 113; fi ;;
 bootout) /bin/cp "$HEV_CONFIG" "$INVITE_CALLS.before"; /usr/bin/touch "$INVITE_CALLS.unloaded" ;;
 bootstrap) [ -f "$HOME/.hev/index-state.json" ] && echo stale-index-state >> "$INVITE_CALLS"; /bin/cp "$HEV_CONFIG" "$INVITE_CALLS.after"; /bin/rm -f "$INVITE_CALLS.unloaded" ;;
 esac
 exit 0
 `
	t.Setenv("INVITE_CALLS", log)
	if err := os.WriteFile(filepath.Join(bin, "launchctl"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.events = append(f.events, r.Method+" "+r.URL.Path)
		if r.Method != "POST" {
			t.Errorf("method = %s", r.Method)
		}
		switch r.URL.Path {
		case "/join/redeem":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if len(body) != 2 {
				t.Errorf("redeem body = %v", body)
			}
			f.code, f.hostname = body["code"], body["hostname"]
			if r.Header.Get("Authorization") != "" {
				t.Error("redeem sent stored key")
			}
			if f.redeemStatus != 0 {
				w.WriteHeader(f.redeemStatus)
				fmt.Fprint(w, `{"error":"do not echo server secrets"}`)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"endpoint": f.server.URL, "namespace": "kit-graham-traces", "key": f.key, "key_id": "key-1", "machine": f.machines, "machines_left": 3 - f.machines, "until": "2026-12-31", "for": "Graham Siener"})
		case "/v2/namespaces/kit-graham-traces/query":
			if r.Header.Get("Authorization") != "Bearer "+f.key {
				t.Error("gateway missing minted key")
			}
			var query map[string]any
			if err := json.NewDecoder(r.Body).Decode(&query); err != nil || query["top_k"] != float64(1) {
				t.Errorf("key query = %v, %v", query, err)
			}
			// Key validation must happen before config replacement or daemon stop.
			if raw, _ := os.ReadFile(daemon.DefaultConfigPath()); !f.rejoining && strings.Contains(string(raw), "invite-test-key") {
				t.Error("config switched before key validated")
			}
			if strings.Contains(allCalls(t, log), "bootout") {
				t.Error("daemon stopped before gateway validation")
			}
			if f.keyStatus != 0 {
				w.WriteHeader(f.keyStatus)
			}
			fmt.Fprint(w, `{"rows":[]}`)
		case "/join/leave", "/join/session":
			if r.Header.Get("Authorization") != "Bearer "+f.key {
				t.Errorf("auth = %q", r.Header.Get("Authorization"))
			}
			if r.URL.Path == "/join/leave" {
				if f.leaveStatus != 0 {
					w.WriteHeader(f.leaveStatus)
					return
				}
				// Remote revocation comes before stopping capture or clearing config.
				if raw, _ := os.ReadFile(daemon.DefaultConfigPath()); !strings.Contains(string(raw), "invite-test-key") {
					t.Error("target cleared before revoke")
				}
				fmt.Fprint(w, `{}`)
			} else {
				fmt.Fprint(w, `{"url":"https://app.hevkit.com/session?token=one-time"}`)
			}
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	old := inviteBaseURL
	inviteBaseURL = f.server.URL
	t.Cleanup(func() { inviteBaseURL = old; f.server.Close() })
	originalSkills := inviteSkills
	inviteSkills = func(home string) ([]string, error) {
		if home != f.home {
			t.Fatal("skills received an unisolated HOME")
		}
		started, err := os.ReadFile(f.log + ".after")
		if err != nil || !strings.Contains(string(started), "redact = true") {
			t.Fatal("skills installed before hosted capture bootstrap")
		}
		f.skillsInstalled = true
		for _, harness := range []string{".claude", ".codex"} {
			path := filepath.Join(home, harness, "skills", "hev-query", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(path, []byte("mock embedded hev-query skill"), 0644); err != nil {
				return nil, err
			}
		}
		return []string{"claude", "codex"}, nil
	}
	t.Cleanup(func() { inviteSkills = originalSkills })

	return f
}

func (f *inviteFixture) requests() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.events, "\n")
}

func readInviteDoc(t *testing.T) map[string]any {
	t.Helper()
	doc, err := daemon.ReadInviteConfig()
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestJoinFreshMatchesRFCWhenPiped(t *testing.T) {
	f := inviteSandbox(t)
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	rootCmd.SetArgs([]string{"redeem", "HEV-GRAHAM-7Q2K"})
	rootCmd.SetOut(writer)
	t.Cleanup(func() { rootCmd.SetArgs(nil); rootCmd.SetOut(nil) })
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	writer.Close()
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	hostname, _ := os.Hostname()
	want := fmt.Sprintf("✓ invite accepted · Graham Siener · 3 machines · until 2026-12-31\n✓ key minted for %s · namespace kit-graham-traces\n✓ archive   hev layer pro (hosted)\n✓ hevd      capturing under launchd, redaction on\n✓ skills    installed for Claude Code and Codex\n  search    hev query \"why did the preflight fail\"\n  next      hev join HEV-GRAHAM-7Q2K on your other machine (2 left)\n  leave     hev leave   (revokes your keys, deletes your archive)\n", hostname)
	if string(raw) != want {
		t.Fatalf("output:\n%s\nwant:\n%s", raw, want)
	}
	if !f.skillsInstalled {
		t.Fatal("skills installer not invoked")
	}
	if f.code != "HEV-GRAHAM-7Q2K" || f.hostname != hostname {
		t.Fatalf("redeem request = %s %s", f.code, f.hostname)
	}
	if want := "POST /join/redeem\nPOST /v2/namespaces/kit-graham-traces/query"; f.requests() != want {
		t.Fatalf("requests = %s", f.requests())
	}
	cfg, err := daemon.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LayerEndpoint != f.server.URL || cfg.LayerAPIKey != "invite-test-key" || cfg.LayerNamespace != "kit-graham-traces" || cfg.LayerStore != "" || cfg.Invite == nil || cfg.Invite.KeyID != "key-1" {
		t.Fatalf("config = %+v", cfg)
	}
	doc := readInviteDoc(t)
	if doc["capture"].(map[string]any)["redact"] != true {
		t.Fatal("redaction not forced on")
	}
	b, _ := os.ReadFile(daemon.DefaultConfigPath())
	if strings.Contains(string(b), "HEV-GRAHAM") {
		t.Fatal("invite code persisted")
	}
	info, _ := os.Stat(daemon.DefaultConfigPath())
	if info.Mode().Perm() != 0600 {
		t.Fatalf("config mode = %v", info.Mode())
	}
	started, _ := os.ReadFile(f.log + ".after")
	if !strings.Contains(string(started), "invite-test-key") || !strings.Contains(string(started), "redact = true") {
		t.Fatalf("bootstrap read wrong config: %s", started)
	}
	for _, h := range []string{".claude", ".codex"} {
		if _, err := os.Stat(filepath.Join(f.home, h, "skills", "hev-query", "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(allCalls(t, f.log), "stale-index-state") {
		t.Fatal("daemon started before state reset")
	}
	if strings.Contains(allCalls(t, f.log), "docker") {
		t.Fatal("join called Docker")
	}
	var status bytes.Buffer
	printInvite(&status)
	if status.String() != "Invite: Graham Siener · 1/3 machines used at last join · until 2026-12-31\n" {
		t.Fatalf("status = %s", &status)
	}
}

func TestJoinPreservesLocalStackAndStopsWriterBeforeSwitch(t *testing.T) {
	f := inviteSandbox(t)
	t.Setenv("FAKE_UNLOADED", "")
	previous := "[layer]\nendpoint = \"http://127.0.0.1:8080\"\napi_key = \"local\"\nnamespace = \"my-local-traces\"\nstore = \"pgvector\"\n[local]\nproject = \"old-project\"\nport = 8080\n[capture]\nredact = false\nredact_salt = \"" + strings.Repeat("ab", 32) + "\"\nscan_interval = \"13m\"\n[future]\nkeep = \"me\"\n"
	path := writeConfig(t, f.home, previous)
	state := indexState(t, f.home)
	compose := filepath.Join(f.home, ".hev", "local", "docker-compose.yml")
	os.MkdirAll(filepath.Dir(compose), 0755)
	os.WriteFile(compose, []byte("archive-volume"), 0600)
	var out bytes.Buffer
	if err := runJoin(context.Background(), &out, "HEV-GRAHAM-7Q2K"); err != nil {
		t.Fatal(err)
	}
	stopped, _ := os.ReadFile(f.log + ".before")
	if string(stopped) != previous {
		t.Fatalf("daemon stopped after switch: %s", stopped)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("index state retained for new target")
	}
	if raw, _ := os.ReadFile(compose); string(raw) != "archive-volume" {
		t.Fatal("Docker archive changed")
	}
	doc := readInviteDoc(t)
	if doc["local"].(map[string]any)["project"] != "old-project" || doc["future"].(map[string]any)["keep"] != "me" || doc["invite_local"].(map[string]any)["namespace"] != "my-local-traces" {
		t.Fatalf("lost config: %v", doc)
	}
	capture := doc["capture"].(map[string]any)
	if capture["redact"] != true || capture["scan_interval"] != "13m" || capture["redact_salt"] != strings.Repeat("ab", 32) {
		t.Fatalf("capture = %v", capture)
	}
	if !strings.Contains(out.String(), "local archive preserved in its Docker volume; local capture stopped") {
		t.Fatalf("output: %s", &out)
	}
	if strings.Contains(allCalls(t, f.log), "stale-index-state") {
		t.Fatal("daemon started before state reset")
	}
	if strings.Contains(allCalls(t, f.log), "docker") {
		t.Fatal("Docker called")
	}
	// Leaving restores the stopped local target, preserving both its namespace
	// and store choice; it never starts local capture or deletes its volume.
	os.WriteFile(f.log, nil, 0600)
	if err := runLeave(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	doc = readInviteDoc(t)
	if doc["invite"] != nil || doc["invite_local"] != nil || doc["layer"].(map[string]any)["namespace"] != "my-local-traces" {
		t.Fatalf("leave config = %v", doc)
	}
	if strings.Contains(allCalls(t, f.log), "bootstrap") {
		t.Fatal("leave restarted capture")
	}
	if raw, _ := os.ReadFile(path); strings.Contains(string(raw), "invite-test-key") {
		t.Fatal("hosted key retained")
	}
}

func TestJoinReadableRedemptionErrorsChangeNothing(t *testing.T) {
	for status, message := range map[int]string{404: "unknown invite code", 409: "machine limit", 410: "expired or revoked", 429: "rate limit"} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := inviteSandbox(t)
			f.redeemStatus = status
			path := writeConfig(t, f.home, hostedConfig)
			state := indexState(t, f.home)
			var out bytes.Buffer
			err := runJoin(context.Background(), &out, "bad")
			if err == nil || !strings.Contains(err.Error(), message) || strings.Contains(err.Error(), "server secrets") {
				t.Fatalf("error = %v", err)
			}
			if out.Len() != 0 || allCalls(t, f.log) != "" {
				t.Fatalf("failure had side effects: %s %s", &out, allCalls(t, f.log))
			}
			if raw, _ := os.ReadFile(path); string(raw) != hostedConfig {
				t.Fatal("config changed")
			}
			if raw, _ := os.ReadFile(state); string(raw) != stateBytes {
				t.Fatal("state changed")
			}
		})
	}
}

func TestJoinRejectedGatewayKeyKeepsTargetAndDaemon(t *testing.T) {
	for _, status := range []int{401, 403, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := inviteSandbox(t)
			f.keyStatus = status
			path := writeConfig(t, f.home, hostedConfig)
			var out bytes.Buffer
			err := runJoin(context.Background(), &out, "code")
			if err == nil || !strings.Contains(err.Error(), "gateway key check failed") {
				t.Fatalf("err=%v", err)
			}
			if raw, _ := os.ReadFile(path); string(raw) != hostedConfig {
				t.Fatal("target changed")
			}
			if allCalls(t, f.log) != "" {
				t.Fatalf("daemon touched: %s", allCalls(t, f.log))
			}
		})
	}
}

func TestLeaveRevokesThenUnloadsAndClearsHostedTarget(t *testing.T) {
	f := inviteSandbox(t)
	doc := map[string]any{"future": map[string]any{"keep": "me"}}
	if err := daemon.WriteInviteConfig(doc, f.server.URL, "kit-graham-traces", "invite-test-key", daemon.InviteConfig{For: "Graham Siener", Machine: 2, MachinesLeft: 1, Until: "2026-12-31", KeyID: "key-1"}); err != nil {
		t.Fatal(err)
	}
	state := indexState(t, f.home)
	t.Setenv("FAKE_UNLOADED", "")
	plist := (launchdJob{Label: launchdLabel()}).plistPath(f.home)
	os.MkdirAll(filepath.Dir(plist), 0755)
	os.WriteFile(plist, []byte("old job"), 0600)
	// File credential wins over an unrelated shell credential.
	t.Setenv("LAYER_API_KEY", "wrong-shell-key")
	var out bytes.Buffer
	if err := runLeave(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if f.requests() != "POST /join/leave" {
		t.Fatal(f.requests())
	}
	if !strings.Contains(allCalls(t, f.log), "bootout") || strings.Contains(allCalls(t, f.log), "bootstrap") {
		t.Fatal(allCalls(t, f.log))
	}
	if _, err := os.Stat(plist); !os.IsNotExist(err) {
		t.Fatal("plist retained")
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("state retained")
	}
	doc = readInviteDoc(t)
	if doc["layer"] != nil || doc["invite"] != nil || doc["future"] == nil {
		t.Fatalf("config = %v", doc)
	}
	if !strings.Contains(out.String(), "all invite keys revoked, hosted archive deleted") {
		t.Fatal(out.String())
	}
}

func TestLeaveFailureRetainsCredentialForRetry(t *testing.T) {
	f := inviteSandbox(t)
	f.leaveStatus = 500
	if err := daemon.WriteInviteConfig(map[string]any{}, f.server.URL, "kit-graham-traces", "invite-test-key", daemon.InviteConfig{KeyID: "key-1"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(daemon.DefaultConfigPath())
	var out bytes.Buffer
	if err := runLeave(context.Background(), &out); err == nil {
		t.Fatal("leave reported success")
	}
	after, _ := os.ReadFile(daemon.DefaultConfigPath())
	if !bytes.Equal(before, after) || allCalls(t, f.log) != "" {
		t.Fatal("failed leave changed local setup")
	}
}

func TestOpenUsesInviteKeyAndBrowserAndPipedFallback(t *testing.T) {
	f := inviteSandbox(t)
	if err := daemon.WriteInviteConfig(map[string]any{}, f.server.URL, "kit-graham-traces", "invite-test-key", daemon.InviteConfig{KeyID: "key-1"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(daemon.DefaultConfigPath())
	t.Setenv("LAYER_API_KEY", "unrelated-shell-key")
	original := inviteOpenURL
	var opened string
	inviteOpenURL = func(out io.Writer, u string) error { opened = u; fmt.Fprintln(out, "mock browser opened"); return nil }
	t.Cleanup(func() { inviteOpenURL = original })
	var out bytes.Buffer
	if err := runOpen(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if opened != "https://app.hevkit.com/session?token=one-time" || f.requests() != "POST /join/session" {
		t.Fatalf("opened=%s requests=%s", opened, f.requests())
	}
	inviteOpenURL = original
	out.Reset()
	if err := runOpen(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), opened) {
		t.Fatal("piped open hid session URL")
	}
	if allCalls(t, f.log) != "" {
		t.Fatalf("open touched daemon/browser under a pipe: %s", allCalls(t, f.log))
	}
	after, _ := os.ReadFile(daemon.DefaultConfigPath())
	if !bytes.Equal(before, after) {
		t.Fatal("open changed config")
	}
}

func TestInviteCommandsRequireInviteAndRedeemAlias(t *testing.T) {
	f := inviteSandbox(t)
	for _, run := range []func(context.Context, io.Writer) error{runLeave, runOpen} {
		if err := run(context.Background(), io.Discard); err == nil || !strings.Contains(err.Error(), "hev join") {
			t.Fatalf("missing invite error: %v", err)
		}
	}
	c, args, err := rootCmd.Find([]string{"redeem", "code"})
	if err != nil || c != joinCmd || len(args) != 1 {
		t.Fatalf("alias = %v %v %v", c, args, err)
	}
	if f.requests() != "" || allCalls(t, f.log) != "" {
		t.Fatal("missing invite had side effects")
	}
}

func TestJoinRefusesShellOverridesBeforeRedeeming(t *testing.T) {
	f := inviteSandbox(t)
	t.Setenv("LAYER_NAMESPACE", "other")
	if err := runJoin(context.Background(), io.Discard, "code"); err == nil || !strings.Contains(err.Error(), "unset LAYER_NAMESPACE") {
		t.Fatal(err)
	}
	if f.requests() != "" {
		t.Fatal("redeemed with override")
	}
}

func TestInviteWriterLockPreventsConfigSwitch(t *testing.T) {
	f := inviteSandbox(t)
	path := writeConfig(t, f.home, hostedConfig)
	lock, err := os.OpenFile(filepath.Join(f.home, ".hev", "daemon.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := runJoin(ctx, io.Discard, "code"); err == nil {
		t.Fatal("switched while writer locked")
	}
	if raw, _ := os.ReadFile(path); string(raw) != hostedConfig {
		t.Fatal("config changed while writer locked")
	}
}

func TestInviteMetadataSurvivesBucketConfigWrite(t *testing.T) {
	f := inviteSandbox(t)
	writeConfig(t, f.home, "[[buckets]]\nname = \"local\"\n")
	doc := readInviteDoc(t)
	if err := daemon.WriteInviteConfig(doc, f.server.URL, "kit-graham-traces", "invite-test-key", daemon.InviteConfig{KeyID: "key-1", Machine: 2, MachinesLeft: 1, Until: "2026-12-31"}); err != nil {
		t.Fatal(err)
	}
	if _, err := daemon.SetActiveBucket("local"); err != nil {
		t.Fatal(err)
	}
	var config struct {
		Invite daemon.InviteConfig `toml:"invite"`
	}
	if _, err := toml.DecodeFile(daemon.DefaultConfigPath(), &config); err != nil || config.Invite.Machine != 2 {
		t.Fatalf("invite lost: %+v %v", config, err)
	}
}

func TestRejoinRefreshesMetadataAndRetainsOriginalLocalTarget(t *testing.T) {
	f := inviteSandbox(t)
	previous := "[layer]\nendpoint = \"http://127.0.0.1:8080\"\napi_key = \"local\"\nnamespace = \"original-traces\"\nstore = \"pgvector\"\n[local]\nproject = \"original\"\n"
	writeConfig(t, f.home, previous)
	if err := runJoin(context.Background(), io.Discard, "code"); err != nil {
		t.Fatal(err)
	}
	salt := readInviteDoc(t)["capture"].(map[string]any)["redact_salt"]
	f.rejoining, f.machines, f.key = true, 2, "rotated-invite-key"
	os.WriteFile(f.log, nil, 0600)
	if err := runJoin(context.Background(), io.Discard, "code"); err != nil {
		t.Fatal(err)
	}
	doc := readInviteDoc(t)
	if doc["invite_local"].(map[string]any)["namespace"] != "original-traces" {
		t.Fatal("rejoin replaced local archive backup with hosted target")
	}
	if doc["capture"].(map[string]any)["redact_salt"] != salt {
		t.Fatal("rejoin rotated redaction identity")
	}
	cfg, err := daemon.LoadConfig()
	if err != nil || cfg.Invite.Machine != 2 || cfg.Invite.MachinesLeft != 1 || cfg.LayerAPIKey != "rotated-invite-key" {
		t.Fatalf("rejoin config: %+v, %v", cfg, err)
	}
}

func TestJoinChecksRedactionIdentityBeforeStartingDaemon(t *testing.T) {
	f := inviteSandbox(t)
	writeConfig(t, f.home, "[capture]\nredact_salt = \"invalid\"\n")
	err := runJoin(context.Background(), io.Discard, "code")
	if err == nil || !strings.Contains(err.Error(), "enable hosted redaction") {
		t.Fatalf("redaction error = %v", err)
	}
	if strings.Contains(allCalls(t, f.log), "bootstrap") {
		t.Fatal("started capture with invalid redaction identity")
	}
}
