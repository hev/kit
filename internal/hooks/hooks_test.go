package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Real payloads captured from Claude Code 2.1 and Codex 0.162 on oven.
const (
	claudePre    = `{"session_id":"c-1","transcript_path":"/t/c.jsonl","cwd":"/w","permission_mode":"bypassPermissions","hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"/w/a.txt","content":"secret"},"tool_use_id":"toolu_1"}`
	claudePost   = `{"session_id":"c-1","cwd":"/w","hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"sleep 20; echo pending"},"tool_response":{"stdout":"pending","stderr":"","interrupted":false},"tool_use_id":"toolu_2"}`
	claudePrompt = `{"session_id":"c-1","cwd":"/w","hook_event_name":"UserPromptSubmit","prompt":"do the thing"}`
	claudeStart  = `{"session_id":"c-1","cwd":"/w","hook_event_name":"SessionStart","source":"startup"}`
	claudeStop   = `{"session_id":"c-1","cwd":"/w","hook_event_name":"Stop","stop_hook_active":true}`
	claudeCmpct  = `{"session_id":"c-1","cwd":"/w","hook_event_name":"PreCompact","trigger":"auto","custom_instructions":""}`
	codexPre     = `{"session_id":"x-1","turn_id":"t-1","transcript_path":"/t/x.jsonl","cwd":"/tmp/cxhook","hook_event_name":"PreToolUse","model":"gpt-6.1-sol","permission_mode":"bypassPermissions","tool_name":"Bash","tool_input":{"command":"git push origin HEAD"},"tool_use_id":"exec-1"}`
	codexPatch   = `{"session_id":"x-1","turn_id":"t-1","cwd":"/tmp/cxhook","hook_event_name":"PostToolUse","tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** Add File: /tmp/cxhook/x.txt\n+hi\n*** End Patch"},"tool_response":"Exit code: 0","tool_use_id":"exec-2"}`
	codexPrompt  = `{"session_id":"x-1","turn_id":"t-1","cwd":"/tmp/cxhook","hook_event_name":"UserPromptSubmit","model":"gpt-6.1-sol","prompt":"Run echo hello"}`
)

func TestNormalizePerHarness(t *testing.T) {
	now := time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC)
	cases := []struct {
		harness, event, raw string
		check               func(t *testing.T, ev Event)
	}{
		{"claude", "pre-tool", claudePre, func(t *testing.T, ev Event) {
			if ev.Tool == nil || ev.Tool.Name != "Write" || ev.Tool.Kind != KindMutate || ev.Tool.UseID != "toolu_1" || ev.SessionID != "c-1" || ev.TranscriptPath != "/t/c.jsonl" {
				t.Fatalf("%+v %+v", ev, ev.Tool)
			}
		}},
		{"claude", "post-tool", claudePost, func(t *testing.T, ev Event) {
			if ev.Tool.Kind != KindRead || ev.Tool.Command != "sleep 20; echo pending" || ev.Tool.ResponseBytes == 0 {
				t.Fatalf("%+v", ev.Tool)
			}
		}},
		{"claude", "prompt-submit", claudePrompt, func(t *testing.T, ev Event) {
			if ev.Prompt != "do the thing" || ev.PromptBytes != 12 || ev.Tool != nil {
				t.Fatalf("%+v", ev)
			}
		}},
		{"claude", "session-start", claudeStart, func(t *testing.T, ev Event) {
			if ev.Source != "startup" {
				t.Fatalf("%+v", ev)
			}
		}},
		{"claude", "stop", claudeStop, func(t *testing.T, ev Event) {
			if !ev.StopHookActive {
				t.Fatalf("%+v", ev)
			}
		}},
		{"claude", "pre-compact", claudeCmpct, func(t *testing.T, ev Event) {
			if ev.Source != "auto" {
				t.Fatalf("%+v", ev)
			}
		}},
		{"codex", "pre-tool", codexPre, func(t *testing.T, ev Event) {
			if ev.TurnID != "t-1" || ev.Model != "gpt-6.1-sol" || ev.Tool.Kind != KindMutate || ev.Tool.Command != "git push origin HEAD" {
				t.Fatalf("%+v %+v", ev, ev.Tool)
			}
		}},
		{"codex", "post-tool", codexPatch, func(t *testing.T, ev Event) {
			if ev.Tool.Name != "apply_patch" || ev.Tool.Kind != KindMutate {
				t.Fatalf("%+v", ev.Tool)
			}
		}},
		{"codex", "prompt-submit", codexPrompt, func(t *testing.T, ev Event) {
			if ev.Prompt != "Run echo hello" || ev.Harness != "codex" || ev.NativeEvent != "UserPromptSubmit" {
				t.Fatalf("%+v", ev)
			}
		}},
	}
	for _, c := range cases {
		ev, err := Normalize(c.harness, c.event, []byte(c.raw), now)
		if err != nil {
			t.Fatalf("%s %s: %v", c.harness, c.event, err)
		}
		if ev.Schema != Schema || ev.Event != c.event || ev.Harness != c.harness || !ev.Time.Equal(now) || ev.ID == "" {
			t.Fatalf("%s %s: envelope %+v", c.harness, c.event, ev)
		}
		c.check(t, ev)
	}
}

func TestNormalizeRejects(t *testing.T) {
	for _, c := range [][3]string{{"pi", "pre-tool", claudePre}, {"claude", "post", claudePre}, {"claude", "pre-tool", ""}, {"codex", "pre-tool", "{not json"}} {
		if _, err := Normalize(c[0], c[1], []byte(c[2]), time.Now()); err == nil {
			t.Fatalf("%v accepted", c[:2])
		}
	}
}

func TestRedactedDropsContent(t *testing.T) {
	ev, _ := Normalize("claude", "pre-tool", []byte(claudePre), time.Now())
	b, _ := json.Marshal(ev.Redacted())
	if strings.Contains(string(b), "secret") || strings.Contains(string(b), "a.txt") || !strings.Contains(string(b), `"input_bytes"`) {
		t.Fatalf("redacted event leaks content: %s", b)
	}
	p, _ := Normalize("claude", "prompt-submit", []byte(claudePrompt), time.Now())
	if b, _ := json.Marshal(p.Redacted()); strings.Contains(string(b), "do the thing") {
		t.Fatalf("prompt leaked: %s", b)
	}
	if ev.Tool.Input == nil {
		t.Fatal("redaction must not change the handler's copy")
	}
}

func TestClassifyShell(t *testing.T) {
	for cmd, want := range map[string]string{
		"sleep 20; echo pending":                   KindRead,
		"./fake-ci status 2>&1":                    KindRead,
		"go test ./... >/dev/null 2>&1":            KindRead,
		"git status && git log -1":                 KindRead,
		"git -C wt push origin HEAD":               KindMutate,
		"cd wt && git commit -m x":                 KindMutate,
		"echo hi > progress.txt":                   KindMutate,
		"echo hi >> progress.txt":                  KindMutate,
		"gh pr view 12 --json state":               KindRead,
		"gh pr create --draft --fill":              KindMutate,
		"gh -R hev/kit pr merge 3":                 KindMutate,
		"gh api repos/hev/kit/pulls":               KindRead,
		"gh api -X POST repos/hev/kit/issues":      KindMutate,
		"gh run rerun 1234":                        KindMutate,
		"hev dispatch ls":                          KindRead,
		"hev dispatch show x":                      KindRead,
		"hev dispatch order.md":                    KindMutate,
		"hev dispatch rework x 'n'":                KindMutate,
		"/home/u/.local/bin/hev steer ack st-1":    KindSteer,
		"'/x/hev' steer ack st-1 && cat inbox.md": KindSteer,
		"rm -rf build":                             KindMutate,
		"sed -i s/a/b/ f.go":                       KindMutate,
		"sed -n 1,5p f.go":                         KindRead,
		"FOO=1 timeout 5 git push":                 KindMutate,
		"":                                         KindOther,
	} {
		if got := ClassifyShell(cmd); got != want && !(cmd == "" && got == KindOther) {
			t.Errorf("%q: got %s, want %s", cmd, got, want)
		}
	}
	if Classify("Edit", "") != KindMutate || Classify("Read", "") != KindRead || Classify("mcp__x__y", "") != KindOther || Classify("Bash", "git push") != KindMutate {
		t.Fatal("tool classification")
	}
}

func TestRenderNativeShapes(t *testing.T) {
	decode := func(b []byte) map[string]any {
		t.Helper()
		var v map[string]any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("%s: %v", b, err)
		}
		return v
	}
	for _, h := range Harnesses {
		if b := Render(h, "pre-tool", Response{Decision: Continue}); b != nil {
			t.Fatalf("continue must print nothing: %s", b)
		}
		v := decode(Render(h, "pre-tool", Response{Decision: Block, Reason: "ack first"}))
		hs := v["hookSpecificOutput"].(map[string]any)
		if hs["hookEventName"] != "PreToolUse" || hs["permissionDecision"] != "deny" || hs["permissionDecisionReason"] != "ack first" || v["continue"] != nil {
			t.Fatalf("%s block: %v", h, v)
		}
		v = decode(Render(h, "post-tool", Response{Context: "URGENT"}))
		hs = v["hookSpecificOutput"].(map[string]any)
		if hs["hookEventName"] != "PostToolUse" || hs["additionalContext"] != "URGENT" {
			t.Fatalf("%s context: %v", h, v)
		}
		v = decode(Render(h, "prompt-submit", Response{Context: "note", SystemMessage: "held"}))
		if v["hookSpecificOutput"].(map[string]any)["hookEventName"] != "UserPromptSubmit" || v["systemMessage"] != "held" {
			t.Fatalf("%s prompt: %v", h, v)
		}
		v = decode(Render(h, "pre-tool", Response{Decision: Halt, Reason: "end your turn"}))
		hs = v["hookSpecificOutput"].(map[string]any)
		if hs["permissionDecision"] != "deny" || hs["permissionDecisionReason"] != "end your turn" {
			t.Fatalf("%s halt: %v", h, v)
		}
		v = decode(Render(h, "post-tool", Response{Decision: Halt, Reason: "end"}))
		switch h {
		case "claude":
			if v["continue"] != false || v["stopReason"] != "end your turn" && v["stopReason"] != "end" || v["hookSpecificOutput"] != nil {
				t.Fatalf("claude post halt: %v", v)
			}
			if p := decode(Render(h, "pre-tool", Response{Decision: Halt, Reason: "x"})); p["continue"] != false || p["stopReason"] != "x" {
				t.Fatalf("claude pre halt must end the turn: %v", p)
			}
		case "codex":
			// Codex fails a hook answer that carries continue or stopReason.
			if _, ok := v["continue"]; ok {
				t.Fatalf("codex post halt: %v", v)
			}
			if v["hookSpecificOutput"].(map[string]any)["additionalContext"] != "end" {
				t.Fatalf("codex post halt context: %v", v)
			}
			if p := decode(Render(h, "pre-tool", Response{Decision: Halt, Reason: "x"})); p["continue"] != nil || p["stopReason"] != nil {
				t.Fatalf("codex pre halt: %v", p)
			}
		}
		// Stop and pre-compact cannot carry model context.
		if b := Render(h, "stop", Response{Context: "x"}); b != nil {
			t.Fatalf("stop context rendered: %s", b)
		}
	}
}

func TestMerge(t *testing.T) {
	r := Merge(Response{Context: "a"}, Response{Decision: Block, Reason: "b1", Context: "c"}, Response{Decision: Block, Reason: "b2"}, Response{})
	if r.Decision != Block || r.Reason != "b1\n\nb2" || r.Context != "a\n\nc" {
		t.Fatalf("%+v", r)
	}
	r = Merge(Response{Decision: Block, Reason: "b"}, Response{Decision: Halt, Reason: "h"})
	if r.Decision != Halt || r.Reason != "h" {
		t.Fatalf("%+v", r)
	}
	if Merge().Decision != Continue {
		t.Fatal("empty merge")
	}
}

func TestRunFailsOpen(t *testing.T) {
	var mu sync.Mutex
	var warnings []string
	warn := func(s string) { mu.Lock(); warnings = append(warnings, s); mu.Unlock() }
	ev := Event{Event: "pre-tool", Harness: "claude"}
	hang := make(chan struct{})
	defer close(hang)
	start := time.Now()
	r := Run(context.Background(), ev, []Handler{
		{Name: "ok", Run: func(context.Context, Event) (Response, error) { return Response{Context: "kept"}, nil }},
		{Name: "error", Run: func(context.Context, Event) (Response, error) {
			return Response{Decision: Block, Reason: "x"}, errors.New("boom")
		}},
		{Name: "panic", Run: func(context.Context, Event) (Response, error) { panic("kaboom") }},
		{Name: "hang", Run: func(context.Context, Event) (Response, error) { <-hang; return Response{Decision: Halt}, nil }},
	}, 150*time.Millisecond, warn)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("runner waited %s for a hung handler", elapsed)
	}
	if r.Decision != Continue || r.Context != "kept" {
		t.Fatalf("failed handlers changed the answer: %+v", r)
	}
	all := strings.Join(warnings, "\n")
	for _, want := range []string{"handler error failed", "boom", "handler panic failed", "kaboom", "handler hang did not answer"} {
		if !strings.Contains(all, want) {
			t.Fatalf("missing warning %q in:\n%s", want, all)
		}
	}
}

func TestUserHandlers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell handlers")
	}
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write("hev-hook-block", `read ev; case "$ev" in *'"kind":"mutate"'*) echo '{"decision":"block","reason":"no writes"}';; esac`)
	write("hev-hook-crash", "echo oops >&2; exit 3")
	pidFile := filepath.Join(dir, "hang.pid")
	write("hev-hook-hang", "echo $$ > "+pidFile+"\nsleep 30 &\necho $! >> "+pidFile+"\nwait")
	write("hev-hook-garbage", "echo not-json")
	if err := os.WriteFile(filepath.Join(dir, "hev-hook-notexec"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	hs := UserHandlers(dir + string(os.PathListSeparator) + dir)
	if len(hs) != 4 {
		t.Fatalf("discovered %d handlers", len(hs))
	}
	ev, _ := Normalize("claude", "pre-tool", []byte(claudePre), time.Now())
	var warnings []string
	start := time.Now()
	r := Run(context.Background(), ev, hs, 500*time.Millisecond, func(s string) { warnings = append(warnings, s) })
	if time.Since(start) > 2*time.Second {
		t.Fatal("hung handler held the hook")
	}
	if r.Decision != Block || r.Reason != "no writes" {
		t.Fatalf("%+v", r)
	}
	if len(warnings) != 3 {
		t.Fatalf("warnings: %q", warnings)
	}
	// The hung handler and its child are killed, not orphaned.
	b, _ := os.ReadFile(pidFile)
	pids := strings.Fields(string(b))
	if len(pids) != 2 {
		t.Fatalf("pids %q", pids)
	}
	for _, p := range pids {
		// SIGKILL is asynchronous: allow the kernel a moment to finish it.
		deadline := time.Now().Add(2 * time.Second)
		for processAlive(p) && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if processAlive(p) {
			t.Fatalf("handler process %s outlived the hook", p)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	c, w := LoadConfig(p)
	if !c.Enabled || c.Timeout != DefaultTimeout || c.HaltTimeout != DefaultHaltTimeout || w != "" {
		t.Fatalf("%+v %q", c, w)
	}
	os.WriteFile(p, []byte("[hooks]\nenabled = false\ntimeout = \"500ms\"\nhalt_timeout = \"30s\"\n"), 0600)
	if c, _ = LoadConfig(p); c.Enabled || c.Timeout != 500*time.Millisecond || c.HaltTimeout != 30*time.Second {
		t.Fatalf("%+v", c)
	}
	os.WriteFile(p, []byte("[hooks\nbroken"), 0600)
	if c, w = LoadConfig(p); !c.Enabled || w == "" {
		t.Fatalf("broken config must fail open with a warning: %+v %q", c, w)
	}
	for _, v := range []string{"off", "OFF", "0", "false"} {
		if !Disabled(v) {
			t.Fatal(v)
		}
	}
	if Disabled("") || Disabled("on") {
		t.Fatal("enabled values")
	}
}
