package serve

import (
	"math"
	"testing"
	"time"

	"github.com/hev/kit/internal/trace"
)

// Blocks() stamps a request's full cost and usage on every block; a subagent
// must count each request once, like the main timeline.
func TestBuildAgentsCountsEachRequestOnce(t *testing.T) {
	turns := []trace.Turn{
		{SessionID: "s", TurnUUID: "u", TS: "2026-11-19T10:00:00Z", Role: "user", Harness: "claude_code", AgentID: "a1", Blocks: []trace.Block{{Type: "text", Text: "go"}}},
		{SessionID: "s", TurnUUID: "t1", TS: "2026-11-19T10:00:01Z", Role: "assistant", Harness: "claude_code", AgentID: "a1", RequestID: "r1", Model: "claude-opus-5-5", Usage: trace.Usage{Input: 1000, Output: 100, CacheRead: 10, CacheCreation: 1},
			Blocks: []trace.Block{{Type: "thinking", Text: "hm"}, {Type: "text", Text: "ok"}, {Type: "tool_use", ToolUseID: "x", ToolName: "Read"}, {Type: "tool_use", ToolUseID: "y", ToolName: "Bash"}}},
		{SessionID: "s", TurnUUID: "t2", TS: "2026-11-19T10:00:02Z", Role: "assistant", Harness: "claude_code", AgentID: "a1", Model: "claude-opus-5-5", Usage: trace.Usage{Input: 2000, Output: 200},
			Blocks: []trace.Block{{Type: "text", Text: "done"}, {Type: "text", Text: "really"}}},
	}
	rows := trace.Blocks(turns)
	agents := buildAgents(map[string][]trace.BlockRow{"a1": rows}, "", nil)
	if len(agents) != 1 {
		t.Fatalf("agents=%d", len(agents))
	}
	a := agents[0]
	want := (1000*4+100*20+10*0.2+1*5)/1e6 + (2000*4+200*20)/1e6
	if got := a["cost"].(float64); math.Abs(got-want) > 1e-12 {
		t.Fatalf("cost=%v want %v (counted per block?)", got, want)
	}
	u := a["usage"].(map[string]int64)
	if u["input"] != 3000 || u["output"] != 300 || u["cache_read"] != 10 || u["cache_write"] != 1 {
		t.Fatalf("usage=%v", u)
	}
	reqs := a["requests"].([]map[string]any)
	if len(reqs) != 2 || reqs[0]["id"] != "r1" || reqs[1]["id"] != "t2" {
		t.Fatalf("requests=%v", reqs)
	}
	if n := len(reqs[0]["blocks"].([]map[string]any)); n != 4 {
		t.Fatalf("r1 blocks=%d", n)
	}
	if tools := reqs[0]["tools"].([]string); len(tools) != 2 {
		t.Fatalf("r1 tools=%v", tools)
	}
	if a["n_tools"].(int) != 2 {
		t.Fatalf("n_tools=%v", a["n_tools"])
	}
}

func TestSessionReportsRatesKnown(t *testing.T) {
	turn := func(model string) []trace.Turn {
		return []trace.Turn{{SessionID: "s", TurnUUID: "t", RequestID: "r", TS: "2026-11-21T10:00:01Z", Role: "assistant", Harness: "agy", Model: model, Usage: trace.Usage{Input: 1000}, Blocks: []trace.Block{{Type: "text", Text: "x"}}}}
	}
	for model, want := range map[string]bool{
		"gemini-4-neon-medium":          true,
		"gemini-4-hydrogen-tiered-high": true,
		"mystery-model":                 false,
	} {
		turns := turn(model)
		s := trace.Sessions(turns, nil, "")[0]
		if got := buildSession(s, trace.Blocks(turns))["rates_known"]; got != want {
			t.Errorf("%s: rates_known=%v want %v", model, got, want)
		}
	}
	// A stored row that predates the field decodes as known, as it claimed.
	if !(trace.SessionRow{}).RatesKnown() {
		t.Fatal("legacy row reported unknown")
	}
}

func TestDisplayRatesUsesPricesTOMLByDate(t *testing.T) {
	nov := displayRates(time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC))
	if got, ok := nov["gemini-4-neon"]; !ok || got != [4]float64{3, 15, 3, 0.15} {
		t.Fatalf("gemini-4-neon=%v ok=%v", got, ok)
	}
	if got, ok := nov["gpt-6.2-sol"]; !ok || got != [4]float64{2, 10, 2.5, 0.1} {
		t.Fatalf("gpt-6.2-sol=%v ok=%v", got, ok)
	}
	// Never falls back to the legacy $15/$75 mock rates.
	if got, ok := nov["claude-opus-5"]; !ok || got != [4]float64{5, 25, 6.25, 0.5} {
		t.Fatalf("claude-opus-5=%v ok=%v", got, ok)
	}
	// Respects effective dates: carbon's Dec 1 increase does not apply in November.
	if got := nov["gemini-4-carbon"]; got != [4]float64{2, 10, 2, 0.1} {
		t.Fatalf("nov gemini-4-carbon=%v", got)
	}
	dec := displayRates(time.Date(2026, 12, 2, 12, 0, 0, 0, time.UTC))
	if got := dec["gemini-4-carbon"]; got != [4]float64{4, 20, 4, 0.2} {
		t.Fatalf("dec gemini-4-carbon=%v", got)
	}
}
