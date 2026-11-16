package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"io"
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

func TestExpeditePresetParity(t *testing.T) {
	p := expedite.Plan{Name: "expedite-test", Reply: "desk/owner", Authority: "review", Runs: []expedite.Run{{Host: "worker", Name: "task", Repo: "sample"}}}
	o, text, e := expediteOrder(dispatch.Order{Name: p.Name, Host: "worker", Repo: "sample", Reply: p.Reply, Every: "7m", MaxLifetime: "2h"}, p)
	if e != nil {
		t.Fatal(e)
	}
	s := expeditePreset(o, text)
	if e = s.Validate(); e != nil {
		t.Fatal(e)
	}
	if s.Mode != "once" || strings.Join(s.Manages.Runs.Names, ",") != "task" || s.Reply != p.Reply || strings.Join(s.Authority, ",") != "say,rework,notify,validate" || s.Idle != "skip" || s.Every != "7m" || s.MaxLifetime != "2h" {
		t.Fatal(s)
	}
	snapshot, e := expeditePlan(s.Brief)
	if e != nil || snapshot.Authority != p.Authority || len(snapshot.Runs) != 1 || snapshot.Runs[0].Name != "task" {
		t.Fatal(snapshot, e)
	}
}

func TestExpediteCrossHostAdmission(t *testing.T) {
	old := expediteRPC
	t.Cleanup(func() { expediteRPC = old })
	for _, refuse := range []bool{false, true} {
		t.Run(fmt.Sprint(refuse), func(t *testing.T) {
			p := expedite.Plan{Name: "expedite-cross", Reply: "desk/owner", Authority: "review", Runs: []expedite.Run{{Host: "owner", Name: "local", Repo: "sample"}, {Host: "remote", Name: "remote", Repo: "sample"}}}
			o, text, e := expediteOrder(dispatch.Order{Name: p.Name, Host: "owner", Repo: "sample", Reply: p.Reply, Every: "10m", MaxLifetime: "1h"}, p)
			if e != nil {
				t.Fatal(e)
			}
			var calls []string
			expediteRPC = func(_ context.Context, h dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
				calls = append(calls, h.SSH+":"+q.Action)
				if h.SSH == "remote" {
					if q.ScopeOwner == nil || !q.AdmissionOnly {
						t.Fatal("would launch remote supervisor", q)
					}
					if refuse {
						return dispatch.Response{}, fmt.Errorf("unreachable")
					}
					return dispatch.Response{ScopeRegistered: true}, nil
				}
				return dispatch.Response{}, nil
			}
			cfg := dispatch.Config{Hosts: map[string]dispatch.Host{"remote": {SSH: "remote"}}}
			c := &cobra.Command{}
			c.SetOut(io.Discard)
			e = startExpediteOrchestrator(c, cfg, dispatch.Host{SSH: "owner"}, o, text, "")
			if refuse {
				if e == nil || !strings.Contains(e.Error(), "remote") || len(calls) != 1 {
					t.Fatal(e, calls)
				}
			} else if e != nil || strings.Join(calls, ",") != "remote:orchestrator-apply,owner:orchestrator-apply" {
				t.Fatal(e, calls)
			}
		})
	}
}

func TestExpediteStatusUndeliveredNotices(t *testing.T) {
	oldInv, oldRPC := expediteInventory, expediteRPC
	t.Cleanup(func() { expediteInventory, expediteRPC = oldInv, oldRPC })
	p := expedite.Plan{Name: "expedite-status", Reply: "desk/owner", Authority: "review", Runs: []expedite.Run{{Host: "worker", Name: "task", Repo: "sample"}}}
	o, text, e := expediteOrder(dispatch.Order{Name: p.Name, Host: "worker", Repo: "sample", Reply: p.Reply, Every: "10m", MaxLifetime: "1h"}, p)
	if e != nil {
		t.Fatal(e)
	}
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if e = os.WriteFile(cfg, []byte("[hosts.worker]\nlocal=true\ncap=4\n"), 0600); e != nil {
		t.Fatal(e)
	}
	expediteInventory = func(dispatch.Config, string) map[string]dispatch.Response {
		return map[string]dispatch.Response{"worker": {Rows: []dispatch.Row{{Meta: dispatch.Meta{Order: o}, State: "stalled", Schedule: dispatch.Schedule{Reason: "[needs you] resolve checks"}}}}}
	}
	expediteRPC = func(context.Context, dispatch.Host, dispatch.Request) (dispatch.Response, error) {
		return dispatch.Response{Files: map[string]string{"order.md": text, "spec.md": "---\nkind: orchestrator\nname: expedite-status\ngoal: Review\nharness: claude\nmodel: claude-fable-5-2\nmanages:\n  runs: {names: [task]}\n---\nReview"}, Notices: []dispatch.OrchestratorNotice{{Message: "review ready", Error: "pull required"}}}, nil
	}
	c := newExpediteCommand()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"--config", cfg, "status", p.Name})
	if e = c.Execute(); e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"stalled", "harness=claude model=claude-fable-5-2", "[needs you] resolve checks", "undelivered=1 newest=review ready error=pull required"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal(out.String())
		}
	}
}

func TestExpediteHarnessModelPrecedence(t *testing.T) {
	var cfg dispatch.Config
	h, m := expediteModel(cfg, "", "")
	if h != "claude" || m != "claude-fable-5-2" {
		t.Fatal(h, m)
	}
	cfg.Expedite.Harness = "codex"
	cfg.Expedite.Model = "configured"
	h, m = expediteModel(cfg, "", "")
	if h != "codex" || m != "configured" {
		t.Fatal(h, m)
	}
	h, m = expediteModel(cfg, "claude", "flag")
	if h != "claude" || m != "flag" {
		t.Fatal(h, m)
	}
	h, m = expediteModel(cfg, "", "flag")
	if h != "codex" || m != "flag" {
		t.Fatal(h, m)
	}
	s := expeditePreset(dispatch.Order{Harness: h, Model: m}, "")
	if s.Harness != h || s.Model != m {
		t.Fatal(s)
	}
}

func TestExpediteStopContinuesAcrossOwnersRows(t *testing.T) {
	oldInv, oldRPC, oldOwn := expediteInventory, expediteRPC, expediteOwnAddress
	t.Cleanup(func() { expediteInventory, expediteRPC, expediteOwnAddress = oldInv, oldRPC, oldOwn })
	config := filepath.Join(t.TempDir(), "dispatch.toml")
	if e := os.WriteFile(config, []byte("default_host = 'worker'\n[hosts.worker]\nlocal = true\n[hosts.verifier]\nlocal = true\n[validation]\nhost = 'verifier'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	expediteOwnAddress = func(context.Context) (string, error) { return "desk/owner", nil }
	for _, state := range []string{"running", "stopped", "complete", "handed-off"} {
		t.Run(state, func(t *testing.T) {
			rows := []dispatch.Row{}
			for _, name := range []string{"old", "new"} {
				r := dispatch.Row{Meta: dispatch.Meta{Order: dispatch.Order{Name: name, Reply: "desk/owner", Until: "all followed runs merged or abandoned"}}, State: "running"}
				if name == "old" {
					r.State = state
				}
				rows = append(rows, r)
			}
			expediteInventory = func(dispatch.Config, string) map[string]dispatch.Response {
				return map[string]dispatch.Response{"worker": {Rows: rows}}
			}
			var stops, removals []string
			expediteRPC = func(_ context.Context, _ dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
				p := expedite.Plan{Name: q.Name, Reply: "desk/owner", Authority: "review", Runs: []expedite.Run{{Host: "worker", Name: "task", Repo: "kit"}}}
				_, text, e := expediteOrder(dispatch.Order{Name: q.Name, Repo: "kit", Every: "10m", MaxLifetime: "24h"}, p)
				if e != nil {
					t.Fatal(e)
				}
				if q.Action == "stop" {
					stops = append(stops, q.Name)
				}
				if q.Action == "orchestrator-scope-remove" {
					removals = append(removals, q.Name)
					if q.Name == "old" {
						return dispatch.Response{}, fmt.Errorf("old host refuses cleanup")
					}
				}
				return dispatch.Response{Files: map[string]string{"order.md": text}}, nil
			}
			c := newExpediteCommand()
			c.SetOut(io.Discard)
			c.SetErr(io.Discard)
			c.SetArgs([]string{"--config", config, "stop"})
			e := c.Execute()
			if state == "running" {
				if e == nil || !strings.Contains(e.Error(), "old host refuses cleanup") || strings.Join(stops, ",") != "old,new" || strings.Join(removals, ",") != "old,new" {
					t.Fatal(e, stops, removals)
				}
			} else if e != nil || strings.Join(stops, ",") != "new" || strings.Join(removals, ",") != "new" {
				t.Fatal(e, stops, removals)
			}
		})
	}
}
