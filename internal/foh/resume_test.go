package foh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hev/kit/internal/trace"
)

type fakeTraceSource struct {
	units []trace.Unit
	turns map[string][]trace.Turn
}

func (f fakeTraceSource) Describe() string             { return "fake" }
func (f fakeTraceSource) Units() ([]trace.Unit, error) { return f.units, nil }
func (f fakeTraceSource) Read(u trace.Unit) ([]trace.Turn, error) {
	return f.turns[u.Key], nil
}

func TestResumeLocalFOHAndOtherHarnessSessions(t *testing.T) {
	dir := t.TempDir()
	// 1. Native hev session JSONL.
	fohFile := filepath.Join(dir, "01TESTFOH.jsonl")
	fohData := strings.Join([]string{
		`{"v":1,"session":"01TESTFOH","type":"meta","cwd":"/work/kit"}`,
		`{"v":1,"session":"01TESTFOH","type":"user","content":"check cluster health"}`,
		`{"v":1,"session":"01TESTFOH","type":"assistant","content":"All 5 nodes are alive."}`,
	}, "\n") + "\n"
	if err := os.WriteFile(fohFile, []byte(fohData), 0o600); err != nil {
		t.Fatal(err)
	}

	// 2. Cross-harness session (claude_code) via trace.Source.
	now := time.Now().Add(time.Minute)
	src := fakeTraceSource{
		units: []trace.Unit{{Key: "/fake/claude-1.jsonl", Modified: now.UnixMilli()}},
		turns: map[string][]trace.Turn{
			"/fake/claude-1.jsonl": {
				{SessionID: "claude-sess-1", Harness: "claude_code", Workdir: "/work/layer", Role: "user", Blocks: []trace.Block{{Type: "text", Text: "fix the gateway timeout"}}},
				{SessionID: "claude-sess-1", Harness: "claude_code", Workdir: "/work/layer", Role: "assistant", Blocks: []trace.Block{{Type: "text", Text: "Increased timeout to 30s."}}},
			},
		},
	}

	sessions := ListLocalSessions(dir, []trace.Source{src}, 10)
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2: %+v", len(sessions), sessions)
	}
	if sessions[0].Harness != "claude" || sessions[0].Title != "fix the gateway timeout" {
		t.Fatalf("first session = %+v", sessions[0])
	}
	if sessions[1].Harness != "hev" || sessions[1].Title != "check cluster health" {
		t.Fatalf("second session = %+v", sessions[1])
	}

	// 3. Load cross-harness session into TUI via resumeLoadedMsg.
	msgs, err := LoadSessionMessages(sessions[0])
	if err != nil || len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Content != "Increased timeout to 30s." {
		t.Fatalf("loaded msgs = %+v err=%v", msgs, err)
	}
	m, a := newTestTUI(t, &scripted{})
	m.Update(resumeLoadedMsg{session: sessions[0], msgs: msgs})
	if got := a.History(); len(got) != 2 || got[0].Content != "fix the gateway timeout" {
		t.Fatalf("agent history = %+v", got)
	}
	v := plain(m)
	if !strings.Contains(v, "resumed [claude]") || !strings.Contains(v, "Increased timeout to 30s.") {
		t.Fatalf("TUI missing resumed transcript:\n%s", v)
	}
	// /res typeahead matches /resume.
	typeText(m, "/res")
	if matches := m.matchingCommands(); len(matches) != 1 || matches[0].Name != "/resume" {
		t.Fatalf("matches for /res = %+v", matches)
	}
	m.Update(key(tea.KeyTab))
	if m.input.Value() != "/resume " {
		t.Fatalf("Tab on /res = %q", m.input.Value())
	}
}
