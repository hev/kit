package intake

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCreatorFallback(t *testing.T) {
	for _, tc := range []struct {
		name      string
		creator   *Actor
		history   bool
		candidate bool
	}{
		{"allowed", testCreator(false, true), false, true},
		{"guest", testCreator(true, true), false, false},
		{"non-member", testCreator(false, false), false, false},
		{"null", nil, false, false},
		{"history overrides denied creator", testCreator(true, true), true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			f.creator = tc.creator
			f.emptyHistory = !tc.history
			f.poll(t)
			if got := strings.Contains(f.out.String(), "would-dispatch"); got != tc.candidate {
				t.Fatal(f.out.String())
			}
			if f.calls != 2 {
				t.Fatalf("got %d requests, want 2", f.calls)
			}
			if tc.candidate {
				c := f.p.state.Cache[f.issue().ID]
				want := f.entered.Add(-time.Hour)
				if tc.history {
					want = f.entered
				}
				if !c.Entered.Equal(want) {
					t.Fatalf("entered %v, want %v", c.Entered, want)
				}
			}
		})
	}
}

func testCreator(guest, member bool) *Actor {
	a := &Actor{ID: "creator-id", Active: true, IsGuest: guest}
	if member {
		a.Teams.Nodes = append(a.Teams.Nodes, struct{ Key string }{"TEST"})
	}
	return a
}

func TestHistoryDenialOverridesAllowedCreator(t *testing.T) {
	f := setup(t)
	f.creator = testCreator(false, true)
	f.guest = true
	f.poll(t)
	if strings.Contains(f.out.String(), "would-dispatch") {
		t.Fatal(f.out.String())
	}
}

func TestFakeRejectsEmptyHistoryCursor(t *testing.T) {
	f := setup(t)
	var result any
	err := f.p.gql(context.Background(), historyQuery, map[string]any{"id": "issue-id", "after": ""}, &result)
	if err == nil || err.Error() != "Linear GraphQL error" {
		t.Fatalf("empty cursor error: %v", err)
	}
}

func TestNullHistoryActorOverridesAllowedCreator(t *testing.T) {
	f := setup(t)
	f.creator = testCreator(false, true)
	f.transitionSystem = true
	f.poll(t)
	if strings.Contains(f.out.String(), "would-dispatch") {
		t.Fatal(f.out.String())
	}
}
