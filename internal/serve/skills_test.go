package serve

import (
	"github.com/hev/kit/internal/trace"
	"net/url"
	"reflect"
	"testing"
	"time"
)

func TestSkillFilterAndDashboard(t *testing.T) {
	rows := []trace.SessionRow{{SessionID: "yes", Skills: trace.StringList{"board", "peers"}}, {SessionID: "no"}}
	q := url.Values{"window": {"all"}, "skill": {"board"}}
	filter, _, err := sessionFilter(q, rows)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(filter, []any{"skills", "ContainsAny", []string{"board"}}) {
		t.Fatalf("filter=%#v", filter)
	}
	match, ok, err := inMemoryFilter(q, time.Now())
	if err != nil || !ok || !match(rows[0]) || match(rows[1]) {
		t.Fatalf("local filter ok=%v err=%v", ok, err)
	}
	if !reflect.DeepEqual(facets(rows)["skill"], []string{"board", "peers"}) {
		t.Fatal("missing facet")
	}
	var listSkills any
	for _, row := range corpus(rows) {
		if row["id"] == "yes" {
			listSkills = row["skills"]
		}
	}
	if !reflect.DeepEqual(listSkills, []string(rows[0].Skills)) {
		t.Fatal("missing list skills")
	}
	if !reflect.DeepEqual(buildSession(rows[0], nil)["skills"], []string(rows[0].Skills)) {
		t.Fatal("missing detail skills")
	}
}
