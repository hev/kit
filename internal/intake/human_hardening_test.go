package intake

import (
	"strings"
	"testing"
	"time"
)

func TestHumanAuthorizerHardening(t *testing.T) {
	for _, name := range []string{"history botActor", "creator botActor", "inactive actor", "inactive creator", "null assignment", "guest transition fallback assignment", "non-member transition fallback assignment", "tie human first", "tie human last", "transition tie human first", "transition tie human last"} {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			human := testCreator(false, true)
			event := func(actor *Actor, assignment bool) map[string]any {
				n := map[string]any{"createdAt": f.entered, "actor": actor}
				if assignment {
					n["toAssigneeId"] = "bot-id"
				} else {
					n["toStateId"] = "state-id"
				}
				return n
			}
			switch name {
			case "history botActor":
				f.botActor = true
			case "creator botActor":
				f.emptyHistory = true
				f.creator = human
				f.creatorBotActor = true
			case "inactive actor":
				f.inactive = true
			case "inactive creator":
				f.emptyHistory = true
				f.creator = human
				human.Active = false
			case "null assignment":
				f.creator = human
				f.historyNodes = []any{event(human, false), event(nil, true)}
			case "guest transition fallback assignment":
				f.creator = human
				f.historyNodes = []any{event(testCreator(true, true), false)}
			case "non-member transition fallback assignment":
				f.creator = human
				f.historyNodes = []any{event(testCreator(false, false), false)}
			case "tie human first", "tie human last", "transition tie human first", "transition tie human last":
				assignment := !strings.HasPrefix(name, "transition")
				denied := event(human, assignment)
				denied["botActor"] = map[string]string{"id": "integration"}
				f.historyNodes = []any{event(human, false), event(human, true), denied}
				if strings.HasSuffix(name, "human last") {
					f.historyNodes = []any{denied, event(human, true), event(human, false)}
				}
			}
			f.poll(t)
			if !strings.Contains(f.out.String(), `"decision":"skipped"`) {
				t.Fatal(f.out.String())
			}
			if f.calls != 2 {
				t.Fatalf("requests = %d, want 2", f.calls)
			}
		})
	}
}

func TestCreatorFallbackPeriodicMembershipCheck(t *testing.T) {
	f := setup(t)
	f.emptyHistory = true
	f.creator = testCreator(false, true)
	f.poll(t)
	if !strings.Contains(f.out.String(), "would-dispatch") {
		t.Fatal(f.out.String())
	}
	f.memberInactive = true
	now := f.p.now()
	f.p.now = func() time.Time { return now.Add(15 * time.Minute) }
	f.out.Reset()
	f.poll(t)
	if strings.Contains(f.out.String(), "would-dispatch") {
		t.Fatal(f.out.String())
	}
	if f.histories != 1 || f.calls != 5 {
		t.Fatalf("histories=%d calls=%d", f.histories, f.calls)
	}
}

func TestHardeningQuerySelections(t *testing.T) {
	if strings.Count(historyQuery, "botActor { id }") != 2 || strings.Count(historyQuery, "id guest app active teams") != 2 {
		t.Fatal("history must select botActor and active for history and creator")
	}
	if !strings.Contains(membershipQuery, "id guest app active teams") {
		t.Fatal("membership must select active")
	}
}

func TestPreHardeningCacheRefresh(t *testing.T) {
	f := setup(t)
	f.poll(t)
	c := f.p.state.Cache[f.issue().ID]
	c.HumanVersion = 0
	f.p.state.Cache[f.issue().ID] = c
	f.botActor = true
	f.out.Reset()
	f.poll(t)
	if f.histories != 2 || !strings.Contains(f.out.String(), "no human authorizer") {
		t.Fatal(f.out.String(), f.histories)
	}
	now := f.p.now()
	f.p.now = func() time.Time { return now.Add(15 * time.Minute) }
	f.out.Reset()
	f.poll(t)
	if strings.Contains(f.out.String(), "would-dispatch") {
		t.Fatal("membership check laundered botActor event", f.out.String())
	}
}
