package filearchive

import (
	"os"
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

// A synthetic credential, not a real one.
const fakeKey = "sk-ant-api03-FAKEfixture0123456789abcdefXYZ"

func TestWriteEvalsRedactsAtRest(t *testing.T) {
	s := open(t)
	e := trace.Eval{
		Session:  "s1",
		TS:       "2026-11-18T10:00:00Z",
		Role:     "validator",
		Marks:    map[string]int{"quality": 1},
		Summary:  "worker pasted " + fakeKey + " into the log",
		Evidence: map[string]string{"log": "export ANTHROPIC_API_KEY=" + fakeKey},
		Findings: []string{"credential " + fakeKey + " leaked"},
	}
	if res, err := s.WriteEvals([]trace.Eval{e}); err != nil || res.RowsUpserted != 1 {
		t.Fatalf("write = %+v, %v", res, err)
	}
	if !strings.Contains(e.Summary, fakeKey) {
		t.Fatal("WriteEvals modified its input")
	}
	raw, err := os.ReadFile(s.evalsPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-ant-api03-") {
		t.Fatalf("credential stored verbatim:\n%s", raw)
	}
	rows, err := s.ListEvalRows(nil)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	r := rows[0]
	for name, field := range map[string]string{"summary": r.Summary, "evidence": r.Evidence, "findings": r.Findings, "text": r.Text} {
		if !strings.Contains(field, "[REDACTED:") || strings.Contains(field, "sk-ant-api03-") {
			t.Errorf("%s not redacted: %s", name, field)
		}
	}
}
