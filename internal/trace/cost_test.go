package trace

import (
	"math"
	"testing"
)

func TestRequestCostUsesPricesToml(t *testing.T) {
	for _, tc := range []struct {
		model, ts string
		usage     Usage
		want      float64
		known     bool
	}{
		// agy's step from agy_test.go: Gemini list rates, not Opus.
		{"gemini-4-neon-medium", "2026-11-19T12:59:21Z", Usage{Input: 10404, CacheRead: 2000, Output: 267}, (10404*3 + 2000*0.15 + 267*15) / 1e6, true},
		{"claude-opus-5-5", "2026-11-19T00:00:00Z", Usage{Input: 1000, Output: 1000, CacheRead: 1000, CacheCreation: 1000}, (1000*4 + 1000*20 + 1000*0.2 + 1000*5) / 1e6, true},
		{"claude-opus-5", "2026-11-19T00:00:00Z", Usage{Input: 1000, Output: 1000}, (1000*5 + 1000*25) / 1e6, true},
		{"claude-haiku-4-5-20251001", "2026-11-19T00:00:00Z", Usage{Input: 1000, CacheCreation: 1000}, (1000*1 + 1000*1.25) / 1e6, true},
		{"gpt-5.6-sol", "2026-11-19T00:00:00Z", Usage{Input: 1000, Output: 1000}, (1000*4 + 1000*20) / 1e6, true},
		// Unknown model: $0 and unknown, never the old $15/$75 stand-in.
		{"mystery-model", "2026-11-19T00:00:00Z", Usage{Input: 1000000, Output: 1000000}, 0, false},
		{"claude-opus-9", "2026-11-19T00:00:00Z", Usage{Input: 1000000}, 0, false},
		// Before launch or undated: the nearest row, flagged approximate.
		{"gemini-4-neon", "2026-11-01T00:00:00Z", Usage{Input: 1000000}, 3, false},
		{"gemini-4-neon", "", Usage{Input: 1000000}, 3, false},
		// No tokens costs nothing whatever the model.
		{"mystery-model", "2026-11-19T00:00:00Z", Usage{}, 0, true},
	} {
		got, known := requestCost(tc.model, tc.ts, tc.usage)
		if math.Abs(got-tc.want) > 1e-12 || known != tc.known {
			t.Errorf("%s %s: got %v known=%v want %v known=%v", tc.model, tc.ts, got, known, tc.want, tc.known)
		}
	}
}

func TestTurnCostOpenCodeExplicitZero(t *testing.T) {
	u := Usage{Input: 1000000, Output: 1000000}
	if usd, known := turnCost(Turn{Harness: "opencode", Model: "big-pickle", NativeCost: true, Usage: u, TS: "2026-11-19T00:00:00Z"}); usd != 0 || !known {
		t.Fatalf("explicit cost: 0 billed: %v %v", usd, known)
	}
	if usd, known := turnCost(Turn{Harness: "opencode", Model: "big-pickle", Cost: 0.5, Usage: u}); usd != 0.5 || !known {
		t.Fatalf("native cost lost: %v %v", usd, known)
	}
	// Absent cost falls back to list rates.
	if usd, known := turnCost(Turn{Harness: "opencode", Model: "claude-sonnet-5", Usage: u, TS: "2026-11-19T00:00:00Z"}); math.Abs(usd-12) > 1e-9 || !known {
		t.Fatalf("absent cost not list-priced: %v %v", usd, known)
	}
	if usd, known := turnCost(Turn{Harness: "opencode", Model: "big-pickle", Usage: u, TS: "2026-11-19T00:00:00Z"}); usd != 0 || known {
		t.Fatalf("absent cost on an unknown model: %v %v", usd, known)
	}
}

func TestSessionsFlagUnknownRates(t *testing.T) {
	turn := func(id, model string) Turn {
		return Turn{SessionID: "s", TurnUUID: id, RequestID: id, TS: "2026-11-19T10:00:01Z", Role: "assistant", Harness: "agy", Model: model, Usage: Usage{Input: 1000000}, Blocks: []Block{{Type: "text", Text: "x"}}}
	}
	known := Sessions([]Turn{turn("a", "gemini-4-neon-medium")}, nil, "")[0]
	if !known.RatesKnown() || math.Abs(known.Cost-3) > 1e-9 {
		t.Fatalf("known session: %+v", known)
	}
	mixed := Sessions([]Turn{turn("a", "gemini-4-neon-medium"), turn("b", "mystery-model")}, nil, "")[0]
	if mixed.RatesKnown() || math.Abs(mixed.Cost-3) > 1e-9 {
		t.Fatalf("unknown model must cost $0 and flag the session: %+v", mixed)
	}
}
