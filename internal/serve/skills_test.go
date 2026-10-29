package serve

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/trace"
)

func TestSkillsDashboard(t *testing.T) {
	rows := []trace.SessionRow{{ID: "yes", SessionID: "yes", Skills: trace.StringList{"board", "peers"}}, {ID: "no", SessionID: "no"}}
	q := url.Values{"skill": {"peers"}, "window": {"all"}}
	kept, err := localRows(rows, q)
	if err != nil || len(kept) != 1 || kept[0].ID != "yes" {
		t.Fatalf("local filter: %+v %v", kept, err)
	}
	if got := facets(rows)["skill"]; len(got) != 2 {
		t.Fatalf("facet=%v", got)
	}
	if got := facetValue["skill"](rows[0]); len(got) != 2 {
		t.Fatalf("values=%v", got)
	}
	pred, ok, err := inMemoryFilter(q, time.Now())
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if !pred(rows[0]) || pred(rows[1]) {
		t.Fatal("archive skill filter")
	}
	load := trace.SkillLoad{Name: "peers", Path: "/repo/skills/peers/SKILL.md", TurnUUID: "t"}
	view := buildSession(rows[0], []trace.BlockRow{{SkillLoads: trace.SkillLoads{load}}})
	for _, data := range []any{corpus(rows), view} {
		b, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"skills":["board","peers"]`) {
			t.Fatalf("missing skills: %s", b)
		}
	}
	if len(view["skill_loads"].([]trace.SkillLoad)) != 1 {
		t.Fatal("missing load evidence")
	}
}
