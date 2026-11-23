package cost

import (
	"math"
	"reflect"
	"testing"
)

func TestNormalizeModel(t *testing.T) {
	for in, want := range map[string]string{
		"claude-opus-5-5":                     "claude-opus-5-5",
		"claude-haiku-4-5-20251001":           "claude-haiku-4-5",
		"anthropic/claude-sonnet-5-5":         "claude-sonnet-5-5",
		"claude-opus-5-5[1m]":                 "claude-opus-5-5",
		"gemini-4-neon-medium":                "gemini-4-neon",
		"gemini-4-carbon-high":                "gemini-4-carbon",
		"gpt-5.6-sol-xhigh":                   "gpt-5.6-sol",
		"claude-sonnet-4-5@20250929":          "claude-sonnet-4-5",
		"Gemini-4-Neon-High":                  "gemini-4-neon",
		"claude-sonnet-4-5-20250929-thinking": "claude-sonnet-4-5-20250929-thinking",
	} {
		if got := NormalizeModel(in); got != want {
			t.Errorf("NormalizeModel(%q)=%q want %q", in, got, want)
		}
	}
}

func TestLookupRate(t *testing.T) {
	rates := []Rate{
		{Model: "claude-opus-5", Effective: "2026-08-23", Input: 5},
		{Model: "claude-opus-5-5", Effective: "2026-09-22", Input: 4},
		{Model: "claude-haiku-4-5-20251001", Effective: "2025-10-15", Input: 1},
		{Model: "gemini-4-carbon", Effective: "2026-11-10", Input: 2},
		{Model: "gemini-4-carbon", Effective: "2026-12-01", Input: 4},
	}
	for _, tc := range []struct {
		model, date string
		input       float64 // 0 means no rate
		dated       bool
	}{
		{"claude-opus-5-5", "2026-10-01", 4, true},
		{"claude-opus-5", "2026-10-01", 5, true},        // a version never borrows another's row
		{"claude-opus-5-5-high", "2026-10-01", 4, true}, // effort suffix
		{"claude-haiku-4-5", "2026-01-01", 1, true},     // row carries the snapshot date
		{"claude-haiku-4-5-20251001", "2026-01-01", 1, true},
		{"gemini-4-carbon-high", "2026-11-19", 2, true},   // intro price
		{"gemini-4-carbon-medium", "2026-12-02", 4, true}, // after the increase
		{"gemini-4-carbon", "", 4, false},                 // undated: latest, approximate
		{"gemini-4-carbon", "2026-01-01", 2, false},       // before launch: earliest, approximate
		{"gemini-4-neon", "2026-11-19", 0, false},         // unknown
		{"claude-opus-9", "2026-11-19", 0, false},         // never an Opus stand-in
	} {
		rate, dated := LookupRate(rates, tc.model, tc.date)
		if tc.input == 0 {
			if rate != nil {
				t.Errorf("%s %s priced at %+v", tc.model, tc.date, rate)
			}
			continue
		}
		if rate == nil || rate.Input != tc.input || dated != tc.dated {
			t.Errorf("%s %s: rate=%+v dated=%v want input=%v dated=%v", tc.model, tc.date, rate, dated, tc.input, tc.dated)
		}
	}
}

func TestEmbeddedRatesMatchPricesFile(t *testing.T) {
	embedded, err := DefaultRates()
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig("../../prices.toml", "../../subscriptions.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(embedded, c.Rates) {
		t.Fatal("embedded rates differ from prices.toml")
	}
	if _, err := ParseRates([]byte("[[rates]]\nmodel = \"x\"\neffective = \"bad\"\n")); err == nil {
		t.Fatal("invalid rate accepted")
	}
}

func TestRateUSDLongContextPerRequest(t *testing.T) {
	r := Rate{Input: 0.1, Output: 0.5, LongContext: 100000, LongInput: 5, LongOutput: 5}
	if usd, ok := r.USD(Usage{Input: 100001, Output: 1000}, true); !ok || math.Abs(usd-(100001*0.5+1000*2.5)/1e6) > 1e-12 {
		t.Fatalf("long-context request: %v %v", usd, ok)
	}
	if _, ok := r.USD(Usage{Input: 100001}, false); ok {
		t.Fatal("aggregate usage cannot pick a long-context tier")
	}
}
