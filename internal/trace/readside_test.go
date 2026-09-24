package trace

import (
	"strings"
	"testing"
)

func TestBlocksStayWholeAndReassembleInOrder(t *testing.T) {
	long := strings.Repeat("é", MaxRunes+50)
	turns := []Turn{
		{SessionID: "s", TurnUUID: "u", TS: "2026-09-05T10:00:00Z", Role: "user", Blocks: []Block{{Type: "text", Text: long}}},
		{SessionID: "s", TurnUUID: "a", TS: "2026-09-05T10:00:01Z", Role: "assistant", RequestID: "r", Model: "claude-sonnet-5", Blocks: []Block{
			{Type: "text", Text: "checking"},
			{Type: "tool_use", Text: `Bash {"command":"false"}`, ToolName: "Bash", ToolUseID: "tool-1"},
		}},
		{SessionID: "s", TurnUUID: "r", TS: "2026-09-05T10:00:04Z", Role: "user", Blocks: []Block{
			{Type: "tool_result", Text: "failed output", ToolUseID: "tool-1", IsError: true},
		}},
	}
	rows := Blocks(turns)
	if len(rows) != 4 {
		t.Fatalf("rows=%d, want one per block", len(rows))
	}
	if rows[0].Text != long {
		t.Fatal("long block was split or changed")
	}
	for i, row := range rows {
		if row.Seq != int64(i) {
			t.Fatalf("row %d seq=%d", i, row.Seq)
		}
	}
	if rows[2].MS != 3000 || rows[2].End != rows[3].Start || rows[2].OK {
		t.Fatalf("tool span/result not joined: use=%+v result=%+v", rows[2], rows[3])
	}
	if rows[2].ID == Blocks(turns)[2].ID { /* stable, expected */
	} else {
		t.Fatal("block id is not stable")
	}
}

func TestSessionsAggregateRequestsWithoutDoubleCountingUsage(t *testing.T) {
	usage1 := Usage{Input: 10, Output: 2, CacheRead: 30, CacheCreation: 4}
	usage2 := Usage{Input: 10, Output: 5, CacheRead: 30, CacheCreation: 4}
	turns := []Turn{
		{SessionID: "s", TurnUUID: "u", TS: "2026-09-05T10:00:00Z", Role: "user", Harness: "claude_code", Workdir: "/repo", Branch: "main", Summary: "Do the thing", Blocks: []Block{{Type: "text", Text: "do it"}}},
		{SessionID: "s", TurnUUID: "a1", TS: "2026-09-05T10:00:01Z", Role: "assistant", RequestID: "req", Model: "claude-sonnet-5", Usage: usage1, Blocks: []Block{{Type: "text", Text: "one"}}},
		{SessionID: "s", TurnUUID: "a2", TS: "2026-09-05T10:00:02Z", Role: "assistant", RequestID: "req", Model: "claude-sonnet-5", Usage: usage2, Blocks: []Block{{Type: "tool_use", Text: "Read x", ToolName: "Read"}}},
	}
	sessions := Sessions(turns, func(string) string { return "git@github.com:hev/kit.git" }, "host-a")
	if len(sessions) != 1 {
		t.Fatalf("sessions=%d", len(sessions))
	}
	s := sessions[0]
	if s.RequestCount != 1 || s.InputTokens != 10 || s.OutputTokens != 5 || s.ToolCount != 1 || s.PromptCount != 1 {
		t.Fatalf("bad aggregate: %+v", s)
	}
	if s.FirstPrompt != "do it" || s.RepoURL == "" || s.Model != "claude-sonnet-5" || s.WallMS != 2000 {
		t.Fatalf("metadata missing: %+v", s)
	}
	if s.Summary != "Do the thing" {
		t.Fatalf("harness summary missing: %+v", s)
	}
	withoutSummary := append([]Turn(nil), turns...)
	withoutSummary[0].Summary = ""
	if Sessions(withoutSummary, func(string) string { return "git@github.com:hev/kit.git" }, "host-a")[0].ID != s.ID {
		t.Fatal("adding a summary changed the existing session row id")
	}
	changed := append([]Turn(nil), turns...)
	changed[0].Blocks = []Block{{Type: "text", Text: "do something else"}}
	if Sessions(changed, nil, "host-a")[0].ID != s.ID {
		t.Fatal("session id changed with content")
	}
}

func TestSessionIdentityStableAcrossHosts(t *testing.T) {
	turns := []Turn{{SessionID: "same-session", TurnUUID: "same-turn"}}
	a, b := Sessions(turns, nil, "mini")[0], Sessions(turns, nil, "laptop")[0]
	if a.ID != b.ID || a.Host != "mini" || b.Host != "laptop" {
		t.Fatalf("host identity lost: %+v %+v", a, b)
	}
}

func TestGrowingSessionKeepsIdentityAndSearchMetadata(t *testing.T) {
	turns := []Turn{
		{SessionID: "growing", TurnUUID: "u1", Role: "user", TS: "2026-09-07T12:00:00Z", Blocks: []Block{{Type: "text", Text: "one"}, {Type: "text", Text: "two"}}},
		{SessionID: "growing", TurnUUID: "a1", Role: "assistant", TS: "2026-09-07T12:01:00Z", Usage: Usage{Input: 10, Output: 20, CacheRead: 30, CacheCreation: 40}, Blocks: []Block{{Type: "tool_use", ToolName: "Read"}, {Type: "tool_use", ToolName: "Bash"}, {Type: "tool_use", ToolName: "Read"}}},
	}
	first := Sessions(turns[:1], nil, "host")[0]
	grown := Sessions(turns, nil, "host")[0]
	if first.ID != "growing" || grown.ID != first.ID || grown.End <= first.End {
		t.Fatalf("identity/state: %+v %+v", first, grown)
	}
	if grown.TotalTokens != 100 || len(grown.PromptTS) != 1 || grown.PromptTS[0] != 1788782400000 || len(grown.ToolNames) != 2 || grown.ToolNames[0] != "Bash" || grown.ToolNames[1] != "Read" {
		t.Fatalf("metadata: %+v", grown)
	}
}

func TestShortPromptUnicode(t *testing.T) {
	full := strings.Repeat("界🙂", 500)
	if got := ShortPrompt(full); len([]rune(got)) != 600 || got != strings.Repeat("界🙂", 300) {
		t.Fatal("preview splits or overflows characters")
	}
	if ShortPrompt("short") != "short" {
		t.Fatal("short preview changed")
	}
	rows := Sessions([]Turn{{SessionID: "s", Role: "user", Blocks: []Block{{Type: "text", Text: full}}}}, nil, "")
	if rows[0].FirstPrompt != full || rows[0].FirstPromptShort != ShortPrompt(full) {
		t.Fatal("index did not populate both prompts")
	}
}
