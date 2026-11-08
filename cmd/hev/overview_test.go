package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func overviewFixture(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture; parser tests run on Windows")
	}
	dir := t.TempDir()
	audit := filepath.Join(dir, "audit")
	script := filepath.Join(dir, "reader")
	body := `#!/bin/sh
printf '%s\n' "$*" >> "$OVERVIEW_AUDIT"
case "$*" in
"ls --json")
 case "$OVERVIEW_MODE" in
 missing) exit 1;;
 badjson) printf '%s\n' '{';;
 malformed) printf '%s\n' '{"version":1}';;
 empty) printf '%s\n' '{"version":1,"host":"oven","at":100,"panes":[]}';;
 *) printf '%s\n' '{"version":1,"host":"oven","at":100,"panes":[{"address":"oven/foh","role":"foh","state":"working","title":"Implement orientation","cwd":"/work/project","branch":"topic","brief":"secret"},{"address":"oven/boh","role":"boh","state":"working"}],"hosts":[{"host":"mini","source":"snapshot","at":50,"stale":true,"panes":[]}]}';;
 esac;;
*) exit 99;;
esac
`
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_PEERS_BACKEND", script)
	t.Setenv("OVERVIEW_AUDIT", audit)
	t.Setenv("HEV_CONFIG", filepath.Join(dir, "absent", "config.toml"))
	return script, audit
}
func TestOverviewHealthyAndReadOnly(t *testing.T) {
	_, audit := overviewFixture(t)
	doc := observeOverview(context.Background(), 10*time.Second)
	if doc.Version != 2 || doc.Peers.State != "ok" {
		t.Fatalf("%+v", doc)
	}
	if len(doc.Peers.Items) != 2 || doc.Peers.Items[0]["role"] != "foh" || doc.Peers.Items[1]["role"] != "boh" || doc.Peers.Sources[1]["stale"] != true {
		t.Fatalf("%+v", doc.Peers)
	}
	if doc.Peers.Items[0]["title"] != "Implement orientation" || doc.Peers.Items[0]["cwd"] != "/work/project" {
		t.Fatalf("%+v", doc.Peers)
	}
	raw, _ := json.Marshal(doc)
	if strings.Contains(string(raw), "secret") {
		t.Fatal("private fields leaked")
	}
	var sections map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"nodes", "orders"} {
		if _, ok := sections[removed]; ok {
			t.Fatalf("removed section %s present: %s", removed, raw)
		}
	}
	if _, err := os.Stat(os.Getenv("HEV_CONFIG")); !os.IsNotExist(err) {
		t.Fatalf("config created: %v", err)
	}
	calls, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(calls)) != "ls --json" {
		t.Fatalf("unexpected operations: %s", calls)
	}
}
func TestOverviewPeersFailures(t *testing.T) {
	for _, tc := range []struct{ mode, code string }{{"empty", ""}, {"missing", "read_failed"}, {"malformed", "malformed_data"}, {"badjson", "malformed_data"}} {
		t.Run(tc.mode, func(t *testing.T) {
			overviewFixture(t)
			t.Setenv("OVERVIEW_MODE", tc.mode)
			doc := observeOverview(context.Background(), 10*time.Second)
			if tc.code == "" {
				if doc.Peers.State != "ok" || len(doc.Peers.Items) != 0 {
					t.Fatal(doc.Peers)
				}
				return
			}
			if doc.Peers.State != "unknown" || doc.Peers.Error == nil || doc.Peers.Error.Code != tc.code {
				t.Fatalf("%+v", doc.Peers)
			}
		})
	}
}
func TestOverviewMissingExecutable(t *testing.T) {
	overviewFixture(t)
	t.Setenv("HEV_PEERS_BACKEND", filepath.Join(t.TempDir(), "missing"))
	doc := observeOverview(context.Background(), 10*time.Second)
	if doc.Peers.Error == nil || doc.Peers.Error.Code != "backend_unavailable" {
		t.Fatalf("%+v", doc)
	}
}
func TestOverviewPeersMalformedAndPartial(t *testing.T) {
	for _, raw := range []string{` null `, `{}`, `{"version":1,"host":"oven","at":1,"panes":[{}]}`, `{"version":1,"host":"oven","at":1,"panes":null}`} {
		s := overviewSection{}
		if overviewPeers([]byte(raw), &s) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	s := overviewSection{State: "ok"}
	err := overviewPeers([]byte(`{"version":1,"host":"oven","at":1,"panes":[{"address":"oven/a","role":"foh"}],"hosts":[{"host":"mini","source":"ssh","error":"secret"}]}`), &s)
	if err != nil || s.State != "unknown" || len(s.Items) != 1 || s.Sources[1]["state"] != "unknown" {
		t.Fatalf("%+v %v", s, err)
	}
}
func TestOverviewOutputAndExit(t *testing.T) {
	overviewFixture(t)
	for _, mode := range []string{"", "missing"} {
		t.Setenv("OVERVIEW_MODE", mode)
		for _, asJSON := range []bool{true, false} {
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.Flags().Bool("json", asJSON, "")
			var out bytes.Buffer
			cmd.SetOut(&out)
			err := runOverview(cmd, nil)
			if (err != nil) != (mode == "missing") {
				t.Fatalf("exit %s: %v", mode, err)
			}
			if asJSON {
				var doc overviewDocument
				if json.Unmarshal(out.Bytes(), &doc) != nil || doc.Version != 2 || doc.Peers.DetailCommand != "hev peers" {
					t.Fatal(out.String())
				}
			} else if mode == "" {
				if strings.Contains(out.String(), "{\"") || strings.Contains(out.String(), "secret") {
					t.Fatal(out.String())
				}
				for _, detail := range []string{"Peers — ok — hev peers", "role=foh", "role=boh", "title=Implement orientation", "cwd=/work/project", "branch=topic", "source mini snapshot"} {
					if !strings.Contains(out.String(), detail) {
						t.Fatalf("missing %s: %s", detail, out.String())
					}
				}
				for _, removed := range []string{"Nodes", "Orders", "hev orders", "hev runs", "coordination"} {
					if strings.Contains(out.String(), removed) {
						t.Fatalf("removed section %q rendered: %s", removed, out.String())
					}
				}
			}
		}
	}
}
func TestOverviewBufferBound(t *testing.T) {
	var b overviewBuffer
	if _, err := b.Write(make([]byte, overviewMaxBytes+1)); err == nil || b.Len() != 0 {
		t.Fatal("unbounded output")
	}
	var copied overviewBuffer
	_, err := io.Copy(&copied, io.LimitReader(strings.NewReader(strings.Repeat("x", overviewMaxBytes+1)), overviewMaxBytes+1))
	if err == nil || copied.Len() > overviewMaxBytes {
		t.Fatal("copy bypassed bound")
	}
}
func TestOverviewCLICompatibility(t *testing.T) {
	_, audit := overviewFixture(t)
	// No coordination executable is reachable; overview must not need one.
	t.Setenv("PATH", filepath.Dir(goTool(t)))
	binary := filepath.Join(t.TempDir(), "hev")
	build := exec.Command(goTool(t), "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	for _, args := range [][]string{{"--help"}, {"browse", "--help"}, {"nodes", "--help"}, {"peers", "--help"}} {
		if out, err := exec.Command(binary, args...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	if _, err := os.Stat(audit); !os.IsNotExist(err) {
		t.Fatal("help performed backend reads")
	}
	help, _ := exec.Command(binary, "--help").CombinedOutput()
	for _, verb := range []string{"orders", "runs", "apply", "get", "describe", "controller", "logs", "wait", "api-resources"} {
		if strings.Contains(string(help), "\n  "+verb+" ") {
			t.Fatalf("help lists removed %s:\n%s", verb, help)
		}
		out, err := exec.Command(binary, verb).CombinedOutput()
		if err == nil || !strings.Contains(string(out), `unknown command "`+verb+`"`) {
			t.Fatalf("%s: %v %s", verb, err, out)
		}
	}
	for _, args := range [][]string{{"--json"}, {}} {
		out, err := exec.Command(binary, args...).CombinedOutput()
		if err != nil || strings.Contains(string(out), "coordination") {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	t.Setenv("OVERVIEW_MODE", "missing")
	out, err := exec.Command(binary, "--json").CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("exit: %v", err)
	}
	var doc overviewDocument
	if json.Unmarshal(out, &doc) != nil || doc.Version != 2 || doc.Peers.State != "unknown" {
		t.Fatalf("invalid partial JSON: %s", out)
	}
	if reflect.ValueOf(tuiCmd.RunE).Pointer() != reflect.ValueOf(runTUI).Pointer() {
		t.Fatal("browse handler changed")
	}
}

func goTool(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	return path
}

func TestOverviewRowsRejectNull(t *testing.T) {
	for _, raw := range []string{"null", " null ", "[null]", "{}", "["} {
		if _, err := overviewRows([]byte(raw), "id"); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
func TestOverviewTerminalBound(t *testing.T) {
	if len([]rune(overviewText(strings.Repeat("x", 1000)))) != 257 {
		t.Fatal("unbounded terminal field")
	}
}

func TestOverviewTitleSanitization(t *testing.T) {
	s := overviewSection{}
	raw := `{"version":1,"host":"oven","at":1,"panes":[{"address":"a","title":"task\u001b\n` + strings.Repeat("x", 500) + `","message":"private","brief":"private"}]}`
	if err := overviewPeers([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	title := s.Items[0]["title"].(string)
	if strings.ContainsAny(title, "\n\x1b") || len([]rune(title)) > 101 {
		t.Fatal(title)
	}
	if _, ok := s.Items[0]["message"]; ok {
		t.Fatal("message leaked")
	}
}

// Inject the blocking transport so process startup cannot consume the deadline
// intended to exercise cancellation.
func TestOverviewSlowReadAndDeadline(t *testing.T) {
	timeout := 100 * time.Millisecond
	read := func(ctx context.Context, _ string, _ []string) ([]byte, string) {
		<-ctx.Done()
		return nil, "timeout"
	}
	start := time.Now()
	doc := observeOverviewWithReader(context.Background(), timeout, read)
	if elapsed := time.Since(start); elapsed > timeout+10*time.Second {
		t.Fatalf("cancellation took %v", elapsed)
	}
	if doc.Peers.Error == nil || doc.Peers.Error.Code != "timeout" {
		t.Fatalf("unexpected result: %+v", doc)
	}
}

func TestOverviewPeerSourcesJSONProjection(t *testing.T) {
	// Controls precede the long Unicode suffix so both sanitization and rune bounds matter.
	host := "host\x1b\n\u202e" + strings.Repeat("界", 2000)
	source := "source\t\x00\u200b" + strings.Repeat("源", 2000)
	panes := []map[string]any{{"address": "peer/a"}}
	raw, err := json.Marshal(map[string]any{
		"version": 1, "host": host, "at": 1, "panes": panes,
		"hosts": []map[string]any{
			{"host": host, "source": source, "at": 2, "stale": true, "panes": panes},
			{"host": host, "source": source, "error": "private failure"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	section := overviewSection{State: "ok"}
	if err := overviewPeers(raw, &section); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(overviewDocument{Version: 1, Peers: section})
	if err != nil {
		t.Fatal(err)
	}
	var doc overviewDocument
	if err := json.Unmarshal(encoded, &doc); err != nil {
		t.Fatal(err)
	}
	peers := doc.Peers
	if len(peers.Sources) != 3 || len(peers.Items) != 2 || peers.State != "unknown" || peers.Error == nil || peers.Error.Code != "read_failed" {
		t.Fatalf("unexpected partial result: %+v", peers)
	}
	wantHost := "host   " + strings.Repeat("界", 249) + "…"
	wantSource := "source   " + strings.Repeat("源", 247) + "…"
	for i, summary := range peers.Sources {
		expectedSource := wantSource
		if i == 0 {
			expectedSource = "local"
		}
		if summary["host"] != wantHost || summary["source"] != expectedSource {
			t.Fatalf("source %d did not sanitize and bound metadata: %+v", i, summary)
		}
		if i < 2 && (summary["host"] != peers.Items[i]["host"] || summary["source"] != peers.Items[i]["source"]) {
			t.Fatalf("source %d differs from item projection", i)
		}
	}
	if peers.Sources[1]["stale"] != true || peers.Sources[1]["observed_peer_count"] != float64(1) || peers.Sources[2]["state"] != "unknown" {
		t.Fatalf("source metadata lost: %+v", peers.Sources)
	}
	if strings.Contains(string(encoded), "private failure") {
		t.Fatal("raw host error leaked")
	}
}

// Exercise CommandContext's kill/wait path with a process that cannot finish
// naturally. exec preserves the recorded shell PID when replacing it with sleep.
func TestOverviewRealProcessTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	script := filepath.Join(dir, "reader")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho $$ > \"$1\"\nexec sleep 1000\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_, code := overviewRead(ctx, script, []string{pidFile})
	if code != "timeout" {
		t.Fatalf("code=%q", code)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("cancellation took %v", elapsed)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal("fixture did not start: ", err)
	}
	pid := strings.TrimSpace(string(raw))
	if _, err := strconv.Atoi(pid); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("kill", "-0", pid).Run(); err == nil {
		// Clean up even if a regression left the fixture alive.
		_ = exec.Command("kill", "-KILL", pid).Run()
		t.Fatalf("child %s remains alive after timeout", pid)
	}
}

func TestOverviewSlowReadWithinBudget(t *testing.T) {
	if overviewTimeout != 5*time.Second {
		t.Fatalf("default timeout=%v", overviewTimeout)
	}
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	dir := t.TempDir()
	ready, release := filepath.Join(dir, "ready"), filepath.Join(dir, "release")
	script := filepath.Join(dir, "reader")
	body := "#!/bin/sh\ntouch \"$1\"\nwhile [ ! -f \"$2\" ]; do sleep 0.01; done\nprintf '[]'\n"
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	type result struct {
		data []byte
		code string
	}
	done := make(chan result, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		data, code := overviewRead(ctx, script, []string{ready, release})
		done <- result{data, code}
	}()
	defer func() { cancel(); <-finished }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture never became ready")
		case <-ticker.C:
		}
	}
	select {
	case <-done:
		t.Fatal("reader returned before release")
	default:
	}
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	r := <-done
	if r.code != "" || string(r.data) != "[]" {
		t.Fatalf("data=%q code=%q", r.data, r.code)
	}
}
