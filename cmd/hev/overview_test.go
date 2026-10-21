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
"nodes list --json")
 printf '%s\n' '[{"id":"oven","status":"observed","host":"oven","owner":"node-owner","brief":"secret"}]';;
"orders list --json")
 case "$OVERVIEW_MODE" in
 missing) exit 1;;
 badjson) printf '%s\n' '{';;
 malformed) printf '%s\n' '{"bad":true}';;
 invalid-row) printf '%s\n' '[{}]';;
 timeout) exec sleep 5;;
 empty) printf '%s\n' '[]';;
 *) printf '%s\n' '[{"id":"order-1","owner":"foh-1","status":"blocked","phase":"validation","next_action":"inspect evidence","brief":{"secret":"hidden"}},{"id":"order-2","status":"done"},{"id":"order-3","status":"cancelled"}]';;
 esac;;
"ls --json")
 printf '%s\n' '{"version":1,"host":"oven","at":100,"panes":[{"address":"oven/foh","role":"foh","state":"working","title":"secret"},{"address":"oven/boh","role":"boh","state":"working"}],"hosts":[{"host":"mini","source":"snapshot","at":50,"stale":true,"panes":[]}]}';;
*) exit 99;;
esac
`
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_COORDINATION_BACKEND", script)
	t.Setenv("HEV_PEERS_BACKEND", script)
	t.Setenv("OVERVIEW_AUDIT", audit)
	t.Setenv("HEV_CONFIG", filepath.Join(dir, "absent", "config.toml"))
	return script, audit
}
func TestOverviewHealthyAndReadOnly(t *testing.T) {
	_, audit := overviewFixture(t)
	doc := observeOverview(context.Background(), time.Second)
	if doc.Nodes.State != "ok" || doc.Orders.State != "ok" || doc.Peers.State != "ok" {
		t.Fatalf("%+v", doc)
	}
	if len(doc.Orders.Items) != 1 || doc.Orders.ClosedCount != 2 || doc.Orders.Items[0]["next_action"] != "inspect evidence" {
		t.Fatalf("%+v", doc.Orders)
	}
	if len(doc.Peers.Items) != 2 || doc.Peers.Items[0]["role"] != "foh" || doc.Peers.Items[1]["role"] != "boh" || doc.Peers.Sources[1]["stale"] != true {
		t.Fatalf("%+v", doc.Peers)
	}
	raw, _ := json.Marshal(doc)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "hidden") {
		t.Fatal("private fields leaked")
	}
	if _, err := os.Stat(os.Getenv("HEV_CONFIG")); !os.IsNotExist(err) {
		t.Fatalf("config created: %v", err)
	}
	calls, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
		counts[line]++
	}
	if !reflect.DeepEqual(counts, map[string]int{"nodes list --json": 1, "orders list --json": 1, "ls --json": 1}) {
		t.Fatalf("unexpected operations: %s", calls)
	}
}
func TestOverviewIndependentFailures(t *testing.T) {
	for _, tc := range []struct{ mode, code string }{{"empty", ""}, {"missing", "read_failed"}, {"malformed", "malformed_data"}, {"badjson", "malformed_data"}, {"invalid-row", "malformed_data"}, {"timeout", "timeout"}} {
		t.Run(tc.mode, func(t *testing.T) {
			overviewFixture(t)
			t.Setenv("OVERVIEW_MODE", tc.mode)
			start := time.Now()
			doc := observeOverview(context.Background(), 250*time.Millisecond)
			if time.Since(start) > time.Second {
				t.Fatal("read was not bounded")
			}
			if doc.Nodes.State != "ok" || doc.Peers.State != "ok" {
				t.Fatalf("healthy sources lost: %+v", doc)
			}
			if tc.code == "" {
				if doc.Orders.State != "ok" || len(doc.Orders.Items) != 0 {
					t.Fatal(doc.Orders)
				}
				return
			}
			if doc.Orders.State != "unknown" || doc.Orders.Error == nil || doc.Orders.Error.Code != tc.code {
				t.Fatalf("%+v", doc.Orders)
			}
		})
	}
}
func TestOverviewMissingExecutable(t *testing.T) {
	overviewFixture(t)
	t.Setenv("HEV_COORDINATION_BACKEND", filepath.Join(t.TempDir(), "missing"))
	doc := observeOverview(context.Background(), time.Second)
	if doc.Nodes.Error.Code != "backend_unavailable" || doc.Orders.Error.Code != "backend_unavailable" || doc.Peers.State != "ok" {
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
				if json.Unmarshal(out.Bytes(), &doc) != nil || doc.Version != 1 || doc.Nodes.DetailCommand != "hev nodes" {
					t.Fatal(out.String())
				}
			} else {
				for _, detail := range []string{"hev nodes", "hev orders", "hev peers", "selected node", "role=foh", "role=boh"} {
					if !strings.Contains(out.String(), detail) {
						t.Fatalf("missing %s: %s", detail, out.String())
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
	binary := filepath.Join(t.TempDir(), "hev")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	for _, args := range [][]string{{"--help"}, {"browse", "--help"}, {"nodes", "--help"}, {"orders", "--help"}} {
		if out, err := exec.Command(binary, args...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	if _, err := os.Stat(audit); !os.IsNotExist(err) {
		t.Fatal("help performed backend reads")
	}
	for _, args := range [][]string{{"--json"}, {}, {"nodes", "list", "--json"}, {"orders", "list", "--json"}, {"peers", "list", "--json"}} {
		if out, err := exec.Command(binary, args...).CombinedOutput(); err != nil {
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
	if json.Unmarshal(out, &doc) != nil || doc.Orders.State != "unknown" || doc.Nodes.State != "ok" {
		t.Fatalf("invalid partial JSON: %s", out)
	}
	if reflect.ValueOf(tuiCmd.RunE).Pointer() != reflect.ValueOf(runTUI).Pointer() {
		t.Fatal("browse handler changed")
	}
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
