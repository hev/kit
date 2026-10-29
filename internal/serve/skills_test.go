package serve

import (
	"github.com/hev/kit/internal/trace"
	"net/url"
	"testing"
	"time"
)

func TestSkillSessionFilter(t *testing.T) {
	q := url.Values{"skill": {"review"}, "window": {"all"}}
	match, ok, err := inMemoryFilter(q, time.Now())
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if !match(trace.SessionRow{Skills: []string{"review"}}) || match(trace.SessionRow{Skills: []string{"other"}}) || match(trace.SessionRow{}) {
		t.Fatal("skill selection mismatch")
	}
	if facetValue["skill"](trace.SessionRow{Skills: []string{"review"}})[0] != "review" {
		t.Fatal("facet missing")
	}
}
