package intake

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBotActorDenialSurvivesMembershipRecovery(t *testing.T) {
	f := setup(t)
	f.botActor = true
	now := f.p.now()
	for step := 0; step < 3; step++ {
		f.memberInactive = step == 1
		f.p.now = func() time.Time { return now.Add(time.Duration(step) * 15 * time.Minute) }
		f.out.Reset()
		f.poll(t)
		c := f.p.state.Cache[f.issue().ID]
		if !c.NonHumanEvent || c.HumanVersion != 3 || c.Reason != "no human authorizer" || !strings.Contains(f.out.String(), `"decision":"skipped"`) {
			t.Fatalf("step %d: cache=%+v log=%s", step, c, f.out.String())
		}
	}
	if f.histories != 1 || f.calls != 8 {
		t.Fatalf("histories=%d calls=%d", f.histories, f.calls)
	}
}

func TestCreatorFallbackTransitionRequiresAllowedHuman(t *testing.T) {
	for _, kind := range []string{"guest", "non-member", "inactive"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			f.creator = testCreator(kind == "guest", kind != "non-member")
			f.creator.Active = kind != "inactive"
			f.historyNodes = []any{map[string]any{"createdAt": f.entered, "toAssigneeId": "bot-id", "actor": testCreator(false, true)}}
			f.poll(t)
			if !strings.Contains(f.out.String(), `"reason":"transition actor outside allowed team"`) || !strings.Contains(f.out.String(), `"decision":"skipped"`) {
				t.Fatal(f.out.String())
			}
		})
	}
}

func TestOnlyAssignmentActorBecomesInactive(t *testing.T) {
	f := setup(t)
	assign, transition := testCreator(false, true), testCreator(false, true)
	assign.ID, transition.ID = "assign-human", "transition-human"
	f.historyNodes = []any{
		map[string]any{"createdAt": f.entered, "toAssigneeId": "bot-id", "actor": assign},
		map[string]any{"createdAt": f.entered, "toStateId": "state-id", "actor": transition},
	}
	f.poll(t)
	if !strings.Contains(f.out.String(), "would-dispatch") {
		t.Fatal(f.out.String())
	}
	f.inactiveMemberID = assign.ID
	now := f.p.now()
	f.p.now = func() time.Time { return now.Add(15 * time.Minute) }
	f.out.Reset()
	f.poll(t)
	if !strings.Contains(f.out.String(), `"reason":"assigning actor outside allowed team"`) || !strings.Contains(f.out.String(), `"decision":"skipped"`) {
		t.Fatal(f.out.String())
	}
	if f.histories != 1 || f.calls != 5 {
		t.Fatalf("histories=%d calls=%d", f.histories, f.calls)
	}
}

func TestRepeatedHistoryCursorRejected(t *testing.T) {
	f := setup(t)
	f.historyPage, f.repeatHistoryCursor = true, true
	_, err := f.p.history(context.Background(), f.issue(), "bot-id")
	if err == nil || err.Error() != "invalid history pagination" {
		t.Fatalf("error=%v", err)
	}
	if f.histories != 2 {
		t.Fatalf("history requests=%d", f.histories)
	}
}

func TestVersionTwoCacheRefresh(t *testing.T) {
	f := setup(t)
	f.poll(t)
	c := f.p.state.Cache[f.issue().ID]
	c.HumanVersion = 2
	f.p.state.Cache[f.issue().ID] = c
	f.botActor = true
	f.out.Reset()
	f.poll(t)
	if f.histories != 2 || !strings.Contains(f.out.String(), "no human authorizer") {
		t.Fatal(f.histories, f.out.String())
	}
}
