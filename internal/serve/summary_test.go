package serve

import (
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

func TestSummaryAndReasonListAndDetail(t *testing.T) {
	for _, tc := range []struct{ summary, reason string }{
		{summary: "Fixed archive summary generation"},
		{summary: " \n "},
		{reason: "Configured summarizer returned an empty summary; retry summary repair"},
		{},
	} {
		s := New(&fakeReader{sessions: []trace.SessionRow{{SessionID: "s1", Summary: tc.summary, SummaryReason: tc.reason, SummaryBackfilled: true}}})
		list := requestJSON(t, s, "/api/sessions?window=all")["sessions"].([]any)[0].(map[string]any)
		detail := requestJSON(t, s, "/api/session/s1")
		for _, got := range []map[string]any{list, detail} {
			if got["summary"] != strings.TrimSpace(tc.summary) {
				t.Fatalf("summary lost: %v", got)
			}
			reason, _ := got["summary_reason"].(string)
			if strings.TrimSpace(tc.summary) == "" && reason == "" {
				t.Fatal("missing summary has no reason")
			}
			if tc.reason != "" && reason != tc.reason {
				t.Fatalf("reason lost: %v", got)
			}
		}
		if list["summary_backfilled"] != (strings.TrimSpace(tc.summary) != "") {
			t.Fatalf("invalid done flag: %v", list)
		}
	}
}
