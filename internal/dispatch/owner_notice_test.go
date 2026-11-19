package dispatch

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// notify_route: push still leaves an orchestrator notice the owner could not
// be reached for queued, so the owner's pull delivers it.
func TestPushRouteUnreachableOwnerStaysPullable(t *testing.T) {
	orchHome(t)
	raw := strings.Replace(orchestratorExample, "reply: owner", "reply: Adams-MacBook-Air/foh-bcc\nnotify_route: push", 1)
	s, err := ApplyOrchestrator(raw, false)
	if err != nil || s.NotifyRoute != "push" {
		t.Fatal(s.NotifyRoute, err)
	}
	for _, reason := range []string{"ssh: Could not resolve hostname macbook: Name or service not known", "ssh: connect to host 100.66.207.41 port 22: Connection refused"} {
		flushOrchestratorNotices(s, func(context.Context, OrchestratorNotice) error { return errors.New(reason) })
	}
	r, err := Handle(Request{Action: "orchestrator-notices"})
	if err != nil || len(r.Notices) == 0 {
		t.Fatalf("push failure dropped the notice: %+v %v", r.Notices, err)
	}
	for _, n := range r.Notices {
		if n.Status != "queued" || n.To != "Adams-MacBook-Air/foh-bcc" {
			t.Fatalf("notice %+v", n)
		}
	}
}
