package foh

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type hevCall struct {
	args  []string
	stdin string
	order string
}

func fakeHev(calls *[]hevCall, out string, err error) HevRunner {
	return func(_ context.Context, args []string, stdin string) ([]byte, error) {
		c := hevCall{args: append([]string(nil), args...), stdin: stdin}
		if len(args) > 1 && args[0] == "dispatch" && strings.HasSuffix(args[1], "order.md") {
			b, _ := os.ReadFile(args[1])
			c.order = string(b)
		}
		*calls = append(*calls, c)
		return []byte(out), err
	}
}

func TestHevToolArgvAndCards(t *testing.T) {
	for _, tt := range []struct {
		tool, args string
		want       string
		stdin      string
	}{
		{"hev_dispatch", `{"action":"ls"}`, "dispatch ls --json", ""},
		{"hev_dispatch", `{"action":"show","name":"run-1"}`, "dispatch show run-1 --json", ""},
		{"hev_dispatch", `{"action":"hosts"}`, "dispatch hosts --json", ""},
		{"hev_peers", `{"action":"ls"}`, "peers ls --json", ""},
		{"hev_peers", `{"action":"read","target":"oven/d-x","lines":20}`, "peers read oven/d-x -n 20", ""},
		{"hev_peers", `{"action":"send","target":"macbook/foh","message":"hello there"}`, "peers send macbook/foh -", "hello there"},
		{"hev_board", `{"action":"search","query":"flaky test"}`, "board search flaky test", ""},
		{"hev_board", `{"action":"read","id":"570"}`, "board read 570", ""},
		{"hev_board", `{"action":"post","board":"kit","title":"T","body":"B"}`, "board post --board kit --title T", "B"},
		{"hev_traces", `{"action":"search","query":"timeout","limit":5}`, "traces search timeout --json --limit 5", ""},
		{"hev_traces", `{"action":"stats"}`, "traces stats --json", ""},
		{"hev_status", `{}`, "--json", ""},
	} {
		var calls []hevCall
		ts := NewToolset(t.TempDir(), fakeHev(&calls, `{"runs":[{"name":"a"},{"name":"b"}]}`, nil))
		res := ts.Run(context.Background(), call("1", tt.tool, tt.args))
		if res.IsError || len(calls) != 1 {
			t.Fatalf("%s %s: %+v calls=%v", tt.tool, tt.args, res, calls)
		}
		if got := strings.Join(calls[0].args, " "); got != tt.want || calls[0].stdin != tt.stdin {
			t.Errorf("%s %s: argv %q stdin %q", tt.tool, tt.args, got, calls[0].stdin)
		}
		if res.Card == nil || res.Card.Tool != tt.tool || !res.Card.OK ||
			strings.TrimSpace(res.Card.Body) == "" || strings.TrimSpace(res.Card.Summary) == "" ||
			strings.Contains(res.Card.Body, `"runs"`) {
			t.Errorf("%s card = %+v", tt.tool, res.Card)
		}
		if !strings.Contains(res.Content, `"runs"`) {
			t.Errorf("%s model output lost the JSON payload: %q", tt.tool, res.Content)
		}
	}
}

func TestHevDispatchSend(t *testing.T) {
	var calls []hevCall
	ts := NewToolset(t.TempDir(), fakeHev(&calls, `{"name":"fix-x","host":"oven"}`, nil))
	res := ts.Run(context.Background(), call("1", "hev_dispatch",
		`{"action":"send","name":"fix-x","repo":"kit","host":"oven","base":"v0.7","harness":"claude","body":"Fix the thing."}`))
	if res.IsError || len(calls) != 1 {
		t.Fatalf("send: %+v", res)
	}
	c := calls[0]
	if c.args[0] != "dispatch" || c.args[2] != "--json" {
		t.Fatalf("argv = %v", c.args)
	}
	want := "---\nname: fix-x\nhost: oven\nrepo: kit\nbase: v0.7\nharness: claude\n---\n\nFix the thing.\n"
	if c.order != want {
		t.Fatalf("order = %q", c.order)
	}
	if _, err := os.Stat(c.args[1]); !os.IsNotExist(err) {
		t.Fatal("staged order not removed")
	}
	// Refused: bad names, missing fields, front-matter injection.
	for _, args := range []string{
		`{"action":"send","name":"Bad Name","repo":"kit","body":"x"}`,
		`{"action":"send","name":"ok","body":"x"}`,
		`{"action":"send","name":"ok","repo":"kit"}`,
		`{"action":"send","name":"ok","repo":"kit\nhost: evil","body":"x"}`,
		`{"action":"show"}`,
		`{"action":"nuke"}`,
	} {
		calls = nil
		res := ts.Run(context.Background(), call("1", "hev_dispatch", args))
		if !res.IsError || len(calls) != 0 || res.Card == nil || res.Card.OK {
			t.Errorf("%s accepted: %+v", args, res)
		}
	}
}

func TestHevToolFailureAndRedaction(t *testing.T) {
	var calls []hevCall
	ts := NewToolset(t.TempDir(), fakeHev(&calls, "partial", fmt.Errorf("hev board: auth failed for Bearer abcdefghijklmnopqrstu")), "SECRETVALUE")
	res := ts.Run(context.Background(), call("1", "hev_board", `{"action":"search","query":"x"}`))
	if !res.IsError || res.Card == nil || res.Card.OK || !strings.HasPrefix(res.Card.Summary, "failed:") {
		t.Fatalf("failure = %+v", res)
	}
	if strings.Contains(res.Content, "abcdefghijklmnopqrstu") || strings.Contains(res.Card.Summary, "abcdefghijklmnopqrstu") {
		t.Fatal("bearer token leaked")
	}
	if !strings.Contains(res.Content, "partial") || !strings.Contains(res.Content, "auth failed") {
		t.Fatalf("lost context: %q", res.Content)
	}
}

func TestLocalTools(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, "a", "b", "x.go"), []byte("package b\nfunc Hello() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "a", "y.md"), []byte("Hello docs\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".git", "z.go"), []byte("Hello git\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "bin.dat"), []byte("Hello\x00binary"), 0o644)
	ts := NewToolset(dir, noHev)
	run := func(name, args string) ToolResult { return ts.Run(context.Background(), call("1", name, args)) }

	if r := run("glob", `{"pattern":"**/*.go"}`); r.Content != "a/b/x.go" {
		t.Fatalf("glob = %q", r.Content)
	}
	if r := run("glob", `{"pattern":"a/*.{md,txt}"}`); r.Content != "a/y.md" {
		t.Fatalf("glob braces = %q", r.Content)
	}
	if r := run("grep", `{"pattern":"Hello"}`); r.Content != "a/b/x.go:2:func Hello() {}\na/y.md:1:Hello docs" {
		t.Fatalf("grep = %q", r.Content)
	}
	if r := run("grep", `{"pattern":"Hello","glob":"*.md"}`); r.Content != "a/y.md:1:Hello docs" {
		t.Fatalf("grep glob = %q", r.Content)
	}
	if r := run("read_file", `{"path":"a/b/x.go","offset":2,"limit":1}`); r.Content != "func Hello() {}" {
		t.Fatalf("read_file = %q", r.Content)
	}
	if r := run("read_file", `{"path":"missing"}`); !r.IsError {
		t.Fatal("missing file must error")
	}
	if r := run("edit_file", `{"path":"a/y.md","old_string":"Hello","new_string":"Bye"}`); r.IsError {
		t.Fatalf("edit = %+v", r)
	}
	if r := run("edit_file", `{"path":"a/y.md","old_string":"nope","new_string":"x"}`); !r.IsError {
		t.Fatal("edit of absent text must error")
	}
	if r := run("edit_file", `{"path":"new/file.txt","new_string":"fresh"}`); r.IsError {
		t.Fatalf("create = %+v", r)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "new", "file.txt")); string(b) != "fresh" {
		t.Fatalf("created = %q", b)
	}
	if r := run("read_file", `not json`); !r.IsError {
		t.Fatal("bad JSON must error")
	}
	if r := run("nope", `{}`); !r.IsError {
		t.Fatal("unknown tool must error")
	}
}

func TestBashToolTimeoutAndCap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell commands")
	}
	ts := NewToolset(t.TempDir(), noHev)
	start := time.Now()
	r := ts.Run(context.Background(), call("1", "bash", `{"command":"echo started; sleep 30","timeout_seconds":1}`))
	if !r.IsError || !strings.Contains(r.Content, "timed out") || !strings.Contains(r.Content, "started") {
		t.Fatalf("timeout = %+v", r)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("timeout not enforced")
	}
	r = ts.Run(context.Background(), call("1", "bash", `{"command":"head -c 200000 /dev/zero | tr '\\0' a"}`))
	if len(r.Content) > MaxToolOutput || !strings.Contains(r.Content, "truncated") {
		t.Fatalf("cap: len %d", len(r.Content))
	}
	r = ts.Run(context.Background(), call("1", "bash", `{"command":"exit 3"}`))
	if !r.IsError || !strings.Contains(r.Content, "exit 3") {
		t.Fatalf("exit = %+v", r)
	}
}
