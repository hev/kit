package serve

import (
	"testing"
	"time"
)

func TestExcludeFacets(t *testing.T) {
	for _, ttl := range []time.Duration{DefaultCacheTTL, 0} {
		s, _ := dashboardFixture(t)
		s.WithCacheTTL(ttl)
		for _, query := range []string{
			"exclude_project=alpha", "exclude_project=https%3A%2F%2Fgithub.com%2Fexample%2Falpha",
			"exclude_model=model-a", "exclude_host=laptop", "exclude_harness=claude_code", "project=alpha&project=beta&exclude_project=alpha",
		} {
			// The fixture has three alpha sessions and three beta sessions.
			url := "?window=all&" + query
			list := requestJSON(t, s, "/api/sessions"+url)
			rows := list["sessions"].([]any)
			if len(rows) != 3 {
				t.Fatalf("ttl=%v %s: %d sessions", ttl, query, len(rows))
			}
			for _, raw := range rows {
				if raw.(map[string]any)["project"] == "alpha" {
					t.Fatalf("excluded session: %v", raw)
				}
			}
			stats := requestJSON(t, s, "/api/stats"+url)
			if stats["traces"] != float64(3) {
				t.Fatalf("%s: stats %v", query, stats["traces"])
			}
			for _, facet := range []string{"project", "model", "harness", "host", "tool"} {
				values := requestJSON(t, s, "/api/values"+url+"&facet="+facet)["values"].([]any)
				total := float64(0)
				for _, raw := range values {
					total += raw.(map[string]any)["n"].(float64)
				}
				// Own inclusions are ignored, but exclusions always apply to counts.
				if total != 3 {
					t.Fatalf("%s facet=%s: %v", query, facet, values)
				}
			}
		}
		for _, query := range []string{"exclude_project=alpha&exclude_project=beta", "project=alpha&exclude_project=alpha"} {
			if got := requestJSON(t, s, "/api/sessions?window=all&"+query)["sessions"].([]any); len(got) != 0 {
				t.Fatalf("%s: %v", query, got)
			}
		}
	}
}
