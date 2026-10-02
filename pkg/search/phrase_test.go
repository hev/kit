package search

import "testing"

func TestMatchesPhrase(t *testing.T) {
	for _, tc := range []struct {
		text, q string
		want    bool
	}{
		{"prefix QUARTZ\n\tAmber suffix", " quartz amber ", true},
		{"amber quartz", "quartz amber", false},
		{"quartz blue amber", "quartz amber", false},
		{"quartz, amber", "quartz amber", false},
		{"café", "cafe", false},
		{"quartz amber", " \t", false},
		{"unquartz amberish", "quartz amber", true},
	} {
		if got := MatchesPhrase(tc.text, tc.q); got != tc.want {
			t.Errorf("%q in %q = %v", tc.q, tc.text, got)
		}
	}
}
