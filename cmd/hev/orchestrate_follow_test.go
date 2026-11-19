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
	"github.com/hev/kit/internal/followthrough"
	"github.com/hev/kit/skills"
)

func TestFollowOrderSnapshotAndAuthority(t *testing.T) {
	p := followthrough.Plan{Name: "follow-test", Reply: "desk/foh", Authority: "review", Runs: []followthrough.Run{{Host: "worker", Name: "task", Repo: "sample"}}}
	o, text, err := followOrder(dispatch.Order{Name: p.Name, Host: "worker", Repo: "sample", Reply: p.Reply, Every: "10m", MaxLifetime: "24h"}, p)
	if err != nil {
		t.Fatal(err)
	}
	if o.Every != "10m" || o.Until != "all followed runs merged or abandoned" || o.Reply != p.Reply {
		t.Fatalf("%+v", o)
	}
	got, err := followPlan(text)
	if err != nil || len(got.Runs) != 1 || got.Runs[0].Name != "task" || got.Authority != "review" {
		t.Fatalf("%+v %v", got, err)
	}
	for _, want := range []string{"Authority: review", "NEVER call merge", "independent", "high reasoning effort", "amend", "kit "} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s", want)
		}
	}
	b, err := skills.FS.ReadFile("hev-orchestrate-follow/SKILL.md")
	if err != nil || len(b) == 0 {
		t.Fatal(err)
	}
	p.Authority = "admin"
	if _, _, err = followOrder(o, p); err == nil {
		t.Fatal("invalid authority accepted")
	}
	o.Every = "0s"
	p.Authority = "review"
	if _, _, err = followOrder(o, p); err == nil {
		t.Fatal("invalid duration")
	}
}

// Both spellings must keep doing everything: the new orchestrator verb and the
// hidden, deprecated hev expedite alias that scripts on hosts still call.
func TestFollowDispatchStatusStop(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  func() *cobra.Command
		pre  []string
		warn bool
	}{
		{"orchestrate follow", newOrchestrateCommand, []string{"follow"}, false},
		{"expedite alias", newExpediteAliasCommand, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) { testFollowDispatchStatusStop(t, tc.cmd, tc.pre, tc.warn) })
	}
}

func testFollowDispatchStatusStop(t *testing.T, build func() *cobra.Command, pre []string, warn bool) {
	oldInv, oldRPC, oldStart, oldOwn, oldNow := followInventory, followRPC, followStart, followOwnAddress, followNow
	t.Cleanup(func() {
		followInventory, followRPC, followStart, followOwnAddress, followNow = oldInv, oldRPC, oldStart, oldOwn, oldNow
	})
	config := filepath.Join(t.TempDir(), "dispatch.toml")
	if err := os.WriteFile(config, []byte("default_host = 'worker'\n[hosts.worker]\nlocal = true\ncap = 4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	followOwnAddress = func(context.Context) (string, error) { return "desk/foh", nil }
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	followNow = func() time.Time { return clock }
	rows := []dispatch.Row{{Meta: dispatch.Meta{Order: dispatch.Order{Name: "task", Repo: "sample", Reply: "desk/foh"}}, State: "done"}, {Meta: dispatch.Meta{Order: dispatch.Order{Name: "unrelated", Repo: "sample", Reply: "desk/else"}}, State: "done"}}
	followInventory = func(dispatch.Config, string) map[string]dispatch.Response {
		return map[string]dispatch.Response{"worker": {Rows: rows}}
	}
	var order string
	followStart = func(_ *cobra.Command, _ dispatch.Config, _ dispatch.Host, o dispatch.Order, text, path string) error {
		if o.Every != "10m" || o.MaxLifetime != "24h" {
			t.Fatalf("not repeating: %+v", o)
		}
		order = text
		rows = append(rows, dispatch.Row{Meta: dispatch.Meta{Order: o}, State: "running", Schedule: dispatch.Schedule{Rounds: 2, Last: clock, Next: clock.Add(10 * time.Minute)}})
		return nil
	}
	var actions []string
	followRPC = func(_ context.Context, _ dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
		actions = append(actions, q.Action)
		return dispatch.Response{Files: map[string]string{"order.md": order}}, nil
	}
	run := func(args ...string) (string, error) {
		c := build()
		var b, stderr bytes.Buffer
		c.SetOut(&b)
		c.SetErr(&stderr)
		c.SetArgs(append(append(append([]string{}, pre...), args...), "--config", config))
		err := c.Execute()
		const deprecation = "hev expedite is deprecated; use hev orchestrate follow\n"
		if warn != (stderr.String() == deprecation) && err == nil {
			t.Fatalf("stderr %q", stderr.String())
		}
		return b.String(), err
	}
	if _, err := run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(order, "name: \"follow-20260101-120000\"") || !strings.Contains(order, "<!-- hev-follow-plan\n") {
		t.Fatal("new launches write new names:", order)
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

func TestFollowPresetParity(t *testing.T) {
	p := followthrough.Plan{Name: "follow-test", Reply: "desk/owner", Authority: "review", Runs: []followthrough.Run{{Host: "worker", Name: "task", Repo: "sample"}}}
	o, text, e := followOrder(dispatch.Order{Name: p.Name, Host: "worker", Repo: "sample", Reply: p.Reply, Every: "7m", MaxLifetime: "2h"}, p)
	if e != nil {
		t.Fatal(e)
	}
	s := followPreset(o, text)
	if e = s.Validate(); e != nil {
		t.Fatal(e)
	}
	if s.Mode != "once" || strings.Join(s.Manages.Runs.Names, ",") != "task" || s.Reply != p.Reply || strings.Join(s.Authority, ",") != "say,rework,notify,validate" || s.Idle != "skip" || s.Every != "7m" || s.MaxLifetime != "2h" {
		t.Fatal(s)
	}
	snapshot, e := followPlan(s.Brief)
	if e != nil || snapshot.Authority != p.Authority || len(snapshot.Runs) != 1 || snapshot.Runs[0].Name != "task" {
		t.Fatal(snapshot, e)
	}
	// Follow rounds review through the durable notes, not a free-text say.
	for _, want := range []string{"hev review NAME --json", "hev review record NAME --head SHA", "--clean", "hev review send NAME --recorded", "not as a free-text say"} {
		if !strings.Contains(s.Brief, want) {
			t.Errorf("preset brief lacks %q", want)
		}
	}
}

func TestFollowCrossHostAdmission(t *testing.T) {
	old := followRPC
	t.Cleanup(func() { followRPC = old })
	for _, refuse := range []bool{false, true} {
		t.Run(fmt.Sprint(refuse), func(t *testing.T) {
			p := followthrough.Plan{Name: "follow-cross", Reply: "desk/owner", Authority: "review", Runs: []followthrough.Run{{Host: "owner", Name: "local", Repo: "sample"}, {Host: "remote", Name: "remote", Repo: "sample"}}}
			o, text, e := followOrder(dispatch.Order{Name: p.Name, Host: "owner", Repo: "sample", Reply: p.Reply, Every: "10m", MaxLifetime: "1h"}, p)
			if e != nil {
				t.Fatal(e)
			}
			var calls []string
			followRPC = func(_ context.Context, h dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
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
			e = startFollowOrchestrator(c, cfg, dispatch.Host{SSH: "owner"}, o, text, "")
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

func TestFollowStatusUndeliveredNotices(t *testing.T) {
	oldInv, oldRPC := followInventory, followRPC
	t.Cleanup(func() { followInventory, followRPC = oldInv, oldRPC })
	p := followthrough.Plan{Name: "follow-status", Reply: "desk/owner", Authority: "review", Runs: []followthrough.Run{{Host: "worker", Name: "task", Repo: "sample"}}}
	o, text, e := followOrder(dispatch.Order{Name: p.Name, Host: "worker", Repo: "sample", Reply: p.Reply, Every: "10m", MaxLifetime: "1h"}, p)
	if e != nil {
		t.Fatal(e)
	}
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if e = os.WriteFile(cfg, []byte("[hosts.worker]\nlocal=true\ncap=4\n"), 0600); e != nil {
		t.Fatal(e)
	}
	followInventory = func(dispatch.Config, string) map[string]dispatch.Response {
		return map[string]dispatch.Response{"worker": {Rows: []dispatch.Row{{Meta: dispatch.Meta{Order: o}, State: "stalled", Schedule: dispatch.Schedule{Reason: "[needs you] resolve checks"}}}}}
	}
	followRPC = func(context.Context, dispatch.Host, dispatch.Request) (dispatch.Response, error) {
		return dispatch.Response{Files: map[string]string{"order.md": text, "spec.md": "---\nkind: orchestrator\nname: follow-status\ngoal: Review\nharness: claude\nmodel: claude-fable-5-2\nmanages:\n  runs: {names: [task]}\n---\nReview"}, Notices: []dispatch.OrchestratorNotice{{Message: "review ready", Error: "pull required"}}}, nil
	}
	c := newFollowCommand()
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

func TestFollowHarnessModelPrecedence(t *testing.T) {
	var cfg dispatch.Config
	h, m := followModel(cfg, "", "")
	if h != "claude" || m != "claude-fable-5-2" {
		t.Fatal(h, m)
	}
	cfg.Orchestrate.Follow.Harness = "codex"
	cfg.Orchestrate.Follow.Model = "configured"
	h, m = followModel(cfg, "", "")
	if h != "codex" || m != "configured" {
		t.Fatal(h, m)
	}
	h, m = followModel(cfg, "claude", "flag")
	if h != "claude" || m != "flag" {
		t.Fatal(h, m)
	}
	h, m = followModel(cfg, "", "flag")
	if h != "codex" || m != "flag" {
		t.Fatal(h, m)
	}
	s := followPreset(dispatch.Order{Harness: h, Model: m}, "")
	if s.Harness != h || s.Model != m {
		t.Fatal(s)
	}
}

func TestFollowStopContinuesAcrossOwnersRows(t *testing.T) {
	oldInv, oldRPC, oldOwn := followInventory, followRPC, followOwnAddress
	t.Cleanup(func() { followInventory, followRPC, followOwnAddress = oldInv, oldRPC, oldOwn })
	config := filepath.Join(t.TempDir(), "dispatch.toml")
	if e := os.WriteFile(config, []byte("default_host = 'worker'\n[hosts.worker]\nlocal = true\n[hosts.verifier]\nlocal = true\n[validation]\nhost = 'verifier'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	followOwnAddress = func(context.Context) (string, error) { return "desk/owner", nil }
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
			followInventory = func(dispatch.Config, string) map[string]dispatch.Response {
				return map[string]dispatch.Response{"worker": {Rows: rows}}
			}
			var stops, removals []string
			followRPC = func(_ context.Context, _ dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
				p := followthrough.Plan{Name: q.Name, Reply: "desk/owner", Authority: "review", Runs: []followthrough.Run{{Host: "worker", Name: "task", Repo: "kit"}}}
				_, text, e := followOrder(dispatch.Order{Name: q.Name, Repo: "kit", Every: "10m", MaxLifetime: "24h"}, p)
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
			c := newFollowCommand()
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

func TestExpediteAliasIsHidden(t *testing.T) {
	var alias *cobra.Command
	for _, c := range rootCmd.Commands() {
		if c.Name() == "expedite" {
			alias = c
		}
	}
	if alias == nil || !alias.Hidden {
		t.Fatal("hev expedite must stay registered and hidden", alias)
	}
	for _, n := range []string{"status", "stop", "round"} {
		if sub, _, e := alias.Find([]string{n}); e != nil || sub == alias {
			t.Fatalf("alias missing %s", n)
		}
	}
	follow, _, e := newOrchestrateCommand().Find([]string{"follow"})
	if e != nil || follow.Name() != "follow" {
		t.Fatal(e)
	}
	for _, f := range []string{"config", "host"} {
		if follow.PersistentFlags().Lookup(f) == nil || alias.PersistentFlags().Lookup(f) == nil {
			t.Fatal("flag drift", f)
		}
	}
	for _, f := range []string{"harness", "model", "authority", "every", "max-lifetime", "repo"} {
		if follow.Flags().Lookup(f) == nil || alias.Flags().Lookup(f) == nil {
			t.Fatal("flag drift", f)
		}
	}
}

// State written by the v0.7 build (testdata/follow-v0.7, generated from that
// release): expedite-* names, the hev-expedite-plan marker and an [expedite]
// config table. The new build must read all of it.
func TestFollowReadsV07ExpediterState(t *testing.T) {
	order, err := os.ReadFile("testdata/follow-v0.7/order.md")
	if err != nil {
		t.Fatal(err)
	}
	spec, err := os.ReadFile("testdata/follow-v0.7/spec.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(order), "<!-- hev-expedite-plan\n") {
		t.Fatal("fixture is not legacy state")
	}
	p, err := followPlan(string(order))
	if err != nil || p.Name != "expedite-20261117-180405" || len(p.Runs) != 1 || p.Runs[0].Name != "kit-task" {
		t.Fatal(p, err)
	}
	if !dispatch.IsFollowName(p.Name) {
		t.Fatal("legacy name not recognized")
	}
	o, err := dispatch.Parse(string(order))
	if err != nil || o.Until != "all followed runs merged or abandoned" || o.Every != "10m" {
		t.Fatal(o, err)
	}
	cfg, err := dispatch.Load("testdata/follow-v0.7/dispatch.toml")
	if err != nil {
		t.Fatal(err)
	}
	if h, m := followModel(cfg, "", ""); h != "codex" || m != "gpt-legacy" {
		t.Fatal("legacy [expedite] defaults ignored", h, m)
	}
	cfg.Orchestrate.Follow.Model = "new-model"
	if h, m := followModel(cfg, "", ""); h != "codex" || m != "new-model" {
		t.Fatal("[orchestrate.follow] must win per field", h, m)
	}

	oldInv, oldRPC, oldOwn := followInventory, followRPC, followOwnAddress
	t.Cleanup(func() { followInventory, followRPC, followOwnAddress = oldInv, oldRPC, oldOwn })
	followOwnAddress = func(context.Context) (string, error) { return "macbook/shell", nil }
	row := dispatch.Row{Meta: dispatch.Meta{Order: o}, State: "running", Schedule: dispatch.Schedule{Rounds: 4}}
	followInventory = func(dispatch.Config, string) map[string]dispatch.Response {
		return map[string]dispatch.Response{"oven": {Rows: []dispatch.Row{row}}}
	}
	var actions []string
	followRPC = func(_ context.Context, _ dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
		actions = append(actions, q.Action+":"+q.Name)
		return dispatch.Response{Files: map[string]string{"order.md": string(order), "spec.md": string(spec)}}, nil
	}
	for _, args := range [][]string{{"follow", "status"}, {"follow", "status", "expedite-20261117-180405"}, {"follow", "stop"}} {
		c := newOrchestrateCommand()
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetErr(&out)
		c.SetArgs(append(args, "--config", "testdata/follow-v0.7/dispatch.toml"))
		if err = c.Execute(); err != nil {
			t.Fatal(args, err)
		}
		if !strings.Contains(out.String(), "oven/expedite-20261117-180405 running harness=claude model=claude-fable-5-2 rounds=4") || !strings.Contains(out.String(), "follows oven/kit-task") {
			t.Fatal(args, out.String())
		}
	}
	if want := "show:expedite-20261117-180405,show:expedite-20261117-180405,show:expedite-20261117-180405,stop:expedite-20261117-180405"; strings.Join(actions, ",") != want {
		t.Fatal(actions)
	}
}
