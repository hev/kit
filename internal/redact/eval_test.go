package redact

import (
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

func TestEvalScrubsProseOnly(t *testing.T) {
	s, err := New([]byte(strings.Repeat("s", 32)))
	if err != nil {
		t.Fatal(err)
	}
	key := "sk-ant-api03-FAKEfixture0123456789abcdefXYZ" // synthetic
	in := trace.Eval{Session: "sess", Role: "validator", Marks: map[string]int{"quality": 2}, Summary: "saw " + key, Evidence: map[string]string{"log": key}, Findings: []string{"leaked " + key}}
	out, counts := s.Eval(in)
	for _, v := range []string{out.Summary, out.Evidence["log"], out.Findings[0]} {
		if strings.Contains(v, key) || !strings.Contains(v, "[REDACTED:") {
			t.Fatalf("not scrubbed: %q", v)
		}
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	if total != 3 {
		t.Fatalf("counts = %v", counts)
	}
	if out.Session != "sess" || out.Role != "validator" || out.Marks["quality"] != 2 {
		t.Fatalf("identifiers changed: %+v", out)
	}
	if !strings.Contains(in.Summary, key) || in.Evidence["log"] != key || !strings.Contains(in.Findings[0], key) {
		t.Fatal("input modified")
	}
	var nilScrubber *Scrubber
	if got, _ := nilScrubber.Eval(in); got.Summary != in.Summary {
		t.Fatal("nil scrubber (redaction opted out) changed the eval")
	}
}
