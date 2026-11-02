package main

import (
	"bytes"
	"context"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/expedite"
	"github.com/hev/kit/skills"
)

func TestExpediteOrderSnapshotAndAuthority(t *testing.T) {
	p := expedite.Plan{Name: "expedite-test", Reply: "desk/foh", Authority: "review", Runs: []expedite.Run{{Host: "worker", Name: "task", Repo: "sample"}}}
	o, text, err := expediteOrder(dispatch.Order{Name: p.Name, Host: "worker", Repo: "sample", Reply: p.Reply, Every: "10m", MaxLifetime: "24h"}, p)
	if err != nil {
		t.Fatal(err)
	}
	if o.Every != "10m" || o.Until != "all followed runs merged or abandoned" || o.Reply != p.Reply {
		t.Fatalf("%+v", o)
	}
	got, err := expeditePlan(text)
	if err != nil || len(got.Runs) != 1 || got.Runs[0].Name != "task" || got.Authority != "review" {
		t.Fatalf("%+v %v", got, err)
	}
	for _, want := range []string{"Authority: review", "NEVER call merge", "independent", "high reasoning effort", "amend", "kit "} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s", want)
		}
	}
	b, err := skills.FS.ReadFile("hev-expedite/SKILL.md")
	if err != nil || len(b) == 0 {
		t.Fatal(err)
	}
	p.Authority = "admin"
	if _, _, err = expediteOrder(o, p); err == nil {
		t.Fatal("invalid authority accepted")
	}
	o.Every = "0s"
	p.Authority = "review"
	if _, _, err = expediteOrder(o, p); err == nil {
		t.Fatal("invalid duration")
	}
}

func TestExpediteDispatchStatusStop(t *testing.T) {
	oldInv, oldRPC, oldStart, oldOwn, oldNow := expediteInventory, expediteRPC, expediteStart, expediteOwnAddress, expediteNow
	t.Cleanup(func() {
		expediteInventory, expediteRPC, expediteStart, expediteOwnAddress, expediteNow = oldInv, oldRPC, oldStart, oldOwn, oldNow
	})
	config := filepath.Join(t.TempDir(), "dispatch.toml")
	if err := os.WriteFile(config, []byte("default_host = 'worker'\n[hosts.worker]\nlocal = true\ncap = 4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	expediteOwnAddress = func(context.Context) (string, error) { return "desk/foh", nil }
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	expediteNow = func() time.Time { return clock }
	rows := []dispatch.Row{{Meta: dispatch.Meta{Order: dispatch.Order{Name: "task", Repo: "sample", Reply: "desk/foh"}}, State: "done"}, {Meta: dispatch.Meta{Order: dispatch.Order{Name: "unrelated", Repo: "sample", Reply: "desk/else"}}, State: "done"}}
	expediteInventory = func(dispatch.Config, string) map[string]dispatch.Response {
		return map[string]dispatch.Response{"worker": {Rows: rows}}
	}
	var order string
	expediteStart = func(_ *cobra.Command, _ dispatch.Config, _ dispatch.Host, o dispatch.Order, text, path string) error {
		if o.Every != "10m" || o.MaxLifetime != "24h" {
			t.Fatalf("not repeating: %+v", o)
		}
		order = text
		rows = append(rows, dispatch.Row{Meta: dispatch.Meta{Order: o}, State: "running", Schedule: dispatch.Schedule{Rounds: 2, Last: clock, Next: clock.Add(10 * time.Minute)}})
		return nil
	}
	var actions []string
	expediteRPC = func(_ context.Context, _ dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
		actions = append(actions, q.Action)
		return dispatch.Response{Files: map[string]string{"order.md": order}}, nil
	}
	run := func(args ...string) (string, error) {
		c := newExpediteCommand()
		var b bytes.Buffer
		c.SetOut(&b)
		c.SetErr(&b)
		c.SetArgs(append([]string{"--config", config}, args...))
		err := c.Execute()
		return b.String(), err
	}
	if _, err := run(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(order, `"name": "unrelated"`) {
		t.Fatal("snapshot leaked another session")
	}
	s, err := run("status")
	if err != nil || !strings.Contains(s, "rounds=2") || !strings.Contains(s, "follows worker/task") {
		t.Fatal(s, err)
	}
	if _, err = run("stop"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(actions, ",") != "show,show,stop" {
		t.Fatal(actions)
	}
}
