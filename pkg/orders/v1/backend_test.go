package orders

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRPCBackendRejectsUnmappedTeamAndRoute(t *testing.T) {
	backend := RPCBackend{"team": {Host: "worker"}}
	for _, tc := range []struct{ team, host string }{{"other", "worker"}, {"team", "other"}} {
		_, err := backend.Execute(context.Background(), tc.team, Request{Action: "list", Host: tc.host})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("team %s and host %s: %v", tc.team, tc.host, err)
		}
	}
}

func TestDocumentSharesDispatchValidation(t *testing.T) {
	doc, err := Document(map[string]any{"name": "test-run", "repo": "kit", "host": "worker", "timeout": float64(5)}, "## Acceptance\n- Pass.")
	if err != nil {
		t.Fatal(err)
	}
	id, host, err := Destination(doc)
	if err != nil || id != "test-run" || host != "worker" {
		t.Fatalf("dispatch parsed %q / %q: %v", id, host, err)
	}
	if _, _, err := Destination(strings.Replace(doc, "## Acceptance", "## Description", 1)); !errors.Is(err, ErrRejected) {
		t.Fatalf("missing acceptance: %v", err)
	}
}

func TestAPIRequiresExplicitLocalAdmission(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HEV_DISPATCH_CONFIG", "")
	doc := "---\nname: test-run\nrepo: kit\nhost: worker\n---\n## Acceptance\n- Pass."
	_, err := ExecuteLocal(context.Background(), Request{Action: "start", ID: "test-run", Host: "worker", Order: doc})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("start without an explicit admission allowlist: %v", err)
	}
}
