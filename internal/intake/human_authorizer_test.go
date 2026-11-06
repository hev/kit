package intake

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestHumanAuthorizers(t *testing.T) {
	for _, name := range []string{"bot assignment", "bot creator", "app actor", "app creator", "human assignment bot transition", "human assignment human transition"} {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			switch name {
			case "bot assignment":
				f.assignmentBot = true
			case "bot creator":
				f.emptyHistory = true
				f.creator = testCreator(false, true)
				f.creator.ID = "bot-id"
			case "app actor":
				f.actorApp = true
			case "app creator":
				f.emptyHistory = true
				f.creator = testCreator(false, true)
				f.creator.IsApp = true
			case "human assignment bot transition":
				f.transitionBot = true
			}
			f.poll(t)
			var d Decision
			if err := json.Unmarshal(f.out.Bytes(), &d); err != nil {
				t.Fatal(err)
			}
			if name == "human assignment human transition" {
				if d.Decision != "would-dispatch" || d.AssignmentActor != "actor-id" || d.TransitionActor != "actor-id" {
					t.Fatal(d)
				}
			} else if d.Decision != "skipped" || d.Reason != "no human authorizer" {
				t.Fatal(d)
			}
			if f.calls != 2 {
				t.Fatalf("requests = %d, want 2", f.calls)
			}
		})
	}
}

func TestCachedAppAuthorizerRejected(t *testing.T) {
	f := setup(t)
	f.poll(t)
	f.memberApp = true
	now := f.p.now()
	f.p.now = func() time.Time { return now.Add(15 * time.Minute) }
	f.out.Reset()
	f.poll(t)
	if !strings.Contains(f.out.String(), "no human authorizer") || strings.Contains(f.out.String(), "would-dispatch") {
		t.Fatal(f.out.String())
	}
}

func TestLegacyAuthorizationCacheRefreshed(t *testing.T) {
	f := setup(t)
	f.poll(t)
	c := f.p.state.Cache[f.issue().ID]
	c.AuthorizerBot = ""
	f.p.state.Cache[f.issue().ID] = c
	f.assignmentBot = true
	f.out.Reset()
	f.poll(t)
	if !strings.Contains(f.out.String(), "no human authorizer") || f.histories != 2 {
		t.Fatal(f.out.String(), f.histories)
	}
}
