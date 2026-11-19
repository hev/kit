package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/followthrough"
)

// v07ExpeditePlan is v0.7's snapshot reader, copied verbatim from
// cmd/hev/expedite.go at 4b914ef (only the plan type is renamed). Its
// `hev expedite status` and `stop` call it on every order they find, and
// reject anything it cannot parse with "not an expediter order".
func v07ExpeditePlan(text string) (followthrough.Plan, error) {
	var p followthrough.Plan
	_, tail, ok := strings.Cut(text, "<!-- hev-expedite-plan\n")
	if !ok {
		return p, fmt.Errorf("not an expediter order")
	}
	raw, _, ok := strings.Cut(tail, "\n-->")
	if !ok {
		return p, fmt.Errorf("invalid expediter snapshot")
	}
	err := json.Unmarshal([]byte(raw), &p)
	return p, err
}

// v07HandoffSnapshot is the old owner's board-handoff snapshot cut
// (internal/dispatch/expedite.go at 4b914ef).
func v07HandoffSnapshot(snapshot string) string {
	if _, tail, ok := strings.Cut(snapshot, "<!-- hev-expedite-plan\n"); ok {
		snapshot, _, _ = strings.Cut(tail, "\n-->")
	}
	return snapshot
}

// Old reader, new writer: an order written by this build must still parse
// with the v0.7 client's reader (status, stop) and the v0.7 owner's handoff,
// and yield the same snapshot the new reader sees.
func TestFollowOrderReadableByV07Client(t *testing.T) {
	p := followthrough.Plan{Name: "follow-20261118-120000", Reply: "desk/owner", Authority: "merge-on-green", Runs: []followthrough.Run{{Host: "worker", Name: "task", Repo: "sample", Model: "m"}, {Host: "oven", Name: "other <-->", Repo: "kit"}}}
	_, text, err := followOrder(dispatch.Order{Name: p.Name, Host: "worker", Repo: "sample", Reply: p.Reply, Every: "10m", MaxLifetime: "24h"}, p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "<!-- hev-follow-plan\n") || !strings.Contains(text, "<!-- hev-expedite-plan\n") {
		t.Fatal("order must carry both snapshot markers:", text)
	}
	if strings.Index(text, "<!-- hev-follow-plan\n") > strings.Index(text, "<!-- hev-expedite-plan\n") {
		t.Fatal("the new marker leads; the legacy copy follows it")
	}
	old, err := v07ExpeditePlan(text)
	if err != nil {
		t.Fatal("v0.7 client rejects a new order:", err)
	}
	cur, err := followPlan(text)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(old, p) || !reflect.DeepEqual(cur, p) {
		t.Fatalf("snapshots differ:\nold %+v\nnew %+v\nwant %+v", old, cur, p)
	}
	var fromHandoff followthrough.Plan
	if err = json.Unmarshal([]byte(v07HandoffSnapshot(text)), &fromHandoff); err != nil || !reflect.DeepEqual(fromHandoff, p) {
		t.Fatal("v0.7 owner handoff snapshot", fromHandoff, err)
	}
	// The order an older owner stores is also what the new build re-reads.
	if o, err := dispatch.Parse(text); err != nil || o.Until != "all followed runs merged or abandoned" {
		t.Fatal(o, err)
	}
}

// New client, old owner: the owner's list answer has a version but no
// follow_names, so the new client names the orchestrator expedite-* (the
// only prefix a v0.7 owner counts toward stall notices). A current owner
// gets follow-*.
func TestFollowLaunchNamesForOwnerVersion(t *testing.T) {
	for _, tc := range []struct {
		name  string
		owner dispatch.Response
		want  string
	}{
		{"v0.7 owner", dispatch.Response{Version: "v0.7.3"}, "expedite-20261118-120000"},
		{"current owner", dispatch.Response{Version: "v0.7.4", FollowNames: true}, "follow-20261118-120000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldInv, oldRPC, oldStart, oldOwn, oldNow := followInventory, followRPC, followStart, followOwnAddress, followNow
			t.Cleanup(func() {
				followInventory, followRPC, followStart, followOwnAddress, followNow = oldInv, oldRPC, oldStart, oldOwn, oldNow
			})
			config := filepath.Join(t.TempDir(), "dispatch.toml")
			if err := os.WriteFile(config, []byte("default_host = 'owner'\n[hosts.owner]\nlocal = true\n[hosts.worker]\nlocal = true\n"), 0600); err != nil {
				t.Fatal(err)
			}
			followOwnAddress = func(context.Context) (string, error) { return "desk/foh", nil }
			followNow = func() time.Time { return time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC) }
			followInventory = func(dispatch.Config, string) map[string]dispatch.Response {
				worker := dispatch.Response{Version: "v0.7.4", FollowNames: true, Rows: []dispatch.Row{{Meta: dispatch.Meta{Order: dispatch.Order{Name: "task", Repo: "sample", Reply: "desk/foh"}}, State: "done"}}}
				return map[string]dispatch.Response{"owner": tc.owner, "worker": worker}
			}
			var name, text string
			followStart = func(_ *cobra.Command, _ dispatch.Config, _ dispatch.Host, o dispatch.Order, t2, _ string) error {
				name, text = o.Name, t2
				return nil
			}
			c := newOrchestrateCommand()
			c.SetOut(&bytes.Buffer{})
			c.SetErr(&bytes.Buffer{})
			c.SetArgs([]string{"follow", "--config", config})
			if err := c.Execute(); err != nil {
				t.Fatal(err)
			}
			if name != tc.want {
				t.Fatalf("name %q, want %q", name, tc.want)
			}
			p, err := v07ExpeditePlan(text)
			if err != nil || p.Name != tc.want {
				t.Fatal("v0.7 reader", p, err)
			}
			if !dispatch.IsFollowName(name) {
				t.Fatal("new build must recognize", name)
			}
		})
	}
}
