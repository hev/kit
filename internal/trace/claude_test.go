package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A transcript is mostly line types this parser has no business reading. Only
// user and assistant carry a message; mode, attachment, ai-title, cost-state
// and the rest are harness bookkeeping, and a parser that guessed at them would
// index the harness's own noise as if the operator had said it.
func TestReadSkipsNonMessageLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	lines := []string{
		`{"type":"mode","sessionId":"s1"}`,
		`{"type":"ai-title","sessionId":"s1","aiTitle":"  Fix the preflight check  "}`,
		`{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-09-05T10:00:00Z","cwd":"/w/repo","gitBranch":"main","message":{"role":"user","content":"fix the preflight"}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"s1","timestamp":"2026-09-05T10:00:01Z","cwd":"/w/repo","message":{"role":"assistant","content":[{"type":"thinking","thinking":"the key is missing"},{"type":"text","text":"I will check the env"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"env | grep KEY"}}]}}`,
		`{"type":"user","uuid":"u2","parentUuid":"a1","sessionId":"s1","timestamp":"2026-09-05T10:00:02Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"KEY not set"}]}]}}`,
		`{"type":"cost-state","sessionId":"s1"}`,
		`not json at all`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := &ClaudeSource{Root: dir}
	turns, err := src.Read(Unit{Key: path})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(turns) != 3 {
		t.Fatalf("turns = %d, want 3 (the two user turns and the assistant)", len(turns))
	}
	for i, turn := range turns {
		if turn.Summary != "Fix the preflight check" {
			t.Errorf("turn %d summary = %q", i, turn.Summary)
		}
	}

	if turns[0].Blocks[0].Text != "fix the preflight" {
		t.Errorf("string content did not become a text block: %+v", turns[0].Blocks)
	}
	if turns[0].Workdir != "/w/repo" || turns[0].Branch != "main" {
		t.Errorf("repo context lost: workdir=%q branch=%q", turns[0].Workdir, turns[0].Branch)
	}

	a := turns[1]
	if len(a.Blocks) != 3 {
		t.Fatalf("assistant blocks = %d, want thinking+text+tool_use", len(a.Blocks))
	}
	if a.Blocks[0].Type != "thinking" || a.Blocks[2].Type != "tool_use" {
		t.Errorf("block types = %q,%q,%q", a.Blocks[0].Type, a.Blocks[1].Type, a.Blocks[2].Type)
	}
	// A tool call is only searchable if its input rides along: nobody queries
	// for "Bash", they query for the command.
	if !strings.Contains(a.Blocks[2].Text, "env | grep KEY") {
		t.Errorf("tool_use text dropped its input: %q", a.Blocks[2].Text)
	}
	if a.Blocks[2].ToolName != "Bash" || a.Blocks[2].ToolUseID != "t1" {
		t.Errorf("tool_use lost its identity: %+v", a.Blocks[2])
	}
	if a.ParentUUID != "u1" {
		t.Errorf("threading lost: parent = %q", a.ParentUUID)
	}

	r := turns[2].Blocks[0]
	if r.Type != "tool_result" || r.Text != "KEY not set" || r.ToolUseID != "t1" {
		t.Errorf("tool_result not flattened to its text: %+v", r)
	}

	// Seq orders turns within the session regardless of what was skipped.
	for i, tn := range turns {
		if tn.Seq != int64(i) {
			t.Errorf("turn %d has seq %d", i, tn.Seq)
		}
	}
}

func TestNormalizeBlocksRendersSalientToolInput(t *testing.T) {
	content := json.RawMessage(`[
		{"type":"tool_use","id":"bash","name":"Bash","input":{"command":"env | grep KEY","description":"Check whether the key exists"}},
		{"type":"tool_use","id":"read","name":"Read","input":{"file_path":"/tmp/config.json","offset":10}},
		{"type":"tool_use","id":"grep","name":"Grep","input":{"pattern":"API_KEY","path":"/work/repo"}},
		{"type":"tool_use","id":"search","name":"WebSearch","input":{"query":"missing API token"}},
		{"type":"tool_use","id":"todo","name":"TodoWrite","input":{"todos":[{"content":"inspect the token"}]}}
	]`)

	blocks := normalizeBlocks(content)
	want := []string{
		"Bash env | grep KEY",
		"Read /tmp/config.json",
		"Grep /work/repo API_KEY",
		"WebSearch missing API token",
		"TodoWrite",
	}
	if len(blocks) != len(want) {
		t.Fatalf("blocks = %d, want %d", len(blocks), len(want))
	}
	for i := range want {
		if blocks[i].Text != want[i] {
			t.Errorf("block %d text = %q, want %q", i, blocks[i].Text, want[i])
		}
		if strings.ContainsAny(blocks[i].Text, "{}\"") {
			t.Errorf("block %d still looks like raw JSON: %q", i, blocks[i].Text)
		}
	}
}

func TestChunkIDIsContentAddressed(t *testing.T) {
	turn := Turn{SessionID: "s1", TurnUUID: "u1", Blocks: []Block{{Type: "text", Text: "hello"}}}
	a, b := Chunks(turn), Chunks(turn)
	if a[0].ID != b[0].ID {
		t.Fatal("same input produced different ids — re-indexing would duplicate every row")
	}

	turn.Blocks[0].Text = "hello!"
	if c := Chunks(turn); c[0].ID == a[0].ID {
		t.Fatal("different text produced the same id")
	}
}

func TestSplitOverlapsAndKeepsRunesWhole(t *testing.T) {
	// Multi-byte throughout, so a byte-wise split would produce invalid UTF-8.
	text := strings.Repeat("é", MaxRunes+400)
	parts := split(text)
	if len(parts) < 2 {
		t.Fatalf("parts = %d, want a split", len(parts))
	}
	for i, p := range parts {
		if !utf8Valid(p) {
			t.Fatalf("part %d is not valid UTF-8", i)
		}
		if n := len([]rune(p)); n > MaxRunes {
			t.Fatalf("part %d has %d runes, over the %d ceiling", i, n, MaxRunes)
		}
	}
	// Consecutive parts share Overlap runes, which is what stitching relies on.
	first, second := []rune(parts[0]), []rune(parts[1])
	tail := string(first[len(first)-Overlap:])
	if !strings.HasPrefix(string(second), tail) {
		t.Error("consecutive parts do not overlap")
	}
}

func utf8Valid(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

// Against the real corpus when there is one. Skipped on a machine with no
// transcripts, which is every CI runner.
func TestAgainstRealTranscripts(t *testing.T) {
	root := DefaultClaudeRoot()
	if _, err := os.Stat(root); err != nil {
		t.Skip("no ~/.claude/projects on this machine")
	}
	src := &ClaudeSource{Root: root}
	units, err := src.Units()
	if err != nil {
		t.Fatalf("Units: %v", err)
	}
	if len(units) == 0 {
		t.Skip("no transcripts")
	}

	var turns, chunks, empty int
	byTier := map[string]int{}
	for _, u := range units {
		ts, err := src.Read(u)
		if err != nil {
			t.Errorf("Read(%s): %v", u.Key, err)
			continue
		}
		turns += len(ts)
		for _, tn := range ts {
			if tn.SessionID == "" || tn.TS == "" {
				empty++
			}
			for _, c := range Chunks(tn) {
				chunks++
				byTier[c.Tier]++
			}
		}
	}
	t.Logf("%d units → %d turns → %d chunks; tiers=%v", len(units), turns, chunks, byTier)
	if turns == 0 {
		t.Fatal("parsed no turns from a non-empty corpus")
	}
	if empty > 0 {
		t.Errorf("%d turns missing session id or timestamp", empty)
	}
}
