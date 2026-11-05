package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const orchestratorExample = "---\nkind: orchestrator\nname: release-train\ngoal: Keep the release green\nmode: loop\nevery: 20m\nuntil: goal\nmax_lifetime: 7d\nharness: claude\nreply: owner\nmanages:\n  runs: {selector: \"issue=REL-*\"}\n  repos: [app]\nauthority: [say, rework, retry, validate, dispatch]\nbudget: {concurrent_runs: 3, runs_per_day: 20, usd_per_day: 40}\nhandoff: {rotate_after: 2}\nidle: skip\nsource: none\n---\nKeep moving.\n"

func orchHome(t *testing.T) {
	t.Helper()
	oldSender := orchestratorNoticeSender
	orchestratorNoticeSender = func(context.Context, OrchestratorNotice) error { return nil }
	t.Cleanup(func() { orchestratorNoticeSender = oldSender })
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_ORCHESTRATOR", "")
}
func applyOrch(t *testing.T) OrchestratorSpec {
	t.Helper()
	s, e := ApplyOrchestrator(orchestratorExample, false)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestOrchestratorInventoryOwnerFields(t *testing.T) {
	orchHome(t)
	for _, owner := range []struct{ reply, host string }{
		{"owner", "build-1"},
		{"next-owner", "build-2"},
		{"", ""},
	} {
		text := strings.Replace(orchestratorExample, "reply: owner", "reply: "+owner.reply+"\nhost: "+owner.host, 1)
		if _, err := ApplyOrchestrator(text, false); err != nil {
			t.Fatal(err)
		}
		state, err := orchestratorState("release-train")
		if err != nil || state.Reply != owner.reply || state.Host != owner.host {
			t.Fatalf("persisted owner fields: %+v, %v", state, err)
		}
		rows, err := ListOrchestrators()
		if err != nil || len(rows) != 1 {
			t.Fatalf("inventory: %+v, %v", rows, err)
		}
		data, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		var wire []map[string]any
		if err := json.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		if wire[0]["reply"] != owner.reply {
			t.Fatalf("reply contract: %s", data)
		}
		if owner.host != "" && wire[0]["host"] != owner.host {
			t.Fatalf("host contract: %s", data)
		}
		if owner.host == "" {
			if _, present := wire[0]["host"]; present {
				t.Fatalf("unset host must be absent: %s", data)
			}
		}
	}
}
func TestOrchestratorSpecValidation(t *testing.T) {
	orchHome(t)
	s, e := ApplyOrchestrator(orchestratorExample, true)
	if e != nil {
		t.Fatal(e)
	}
	if s.Brief != "Keep moving." || s.Budget.ConcurrentRuns != 3 {
		t.Fatalf("bad decode %+v", s)
	}
	if exists(OrchestratorConfigDir()) || exists(Root()) {
		t.Fatal("dry run created state")
	}
	for _, tt := range []struct{ name, old, new string }{
		{"kind", "kind: orchestrator", "kind: worker"}, {"unknown", "idle: skip", "typo: true"}, {"merge", "say, rework", "merge, rework"},
		{"lifetime", "max_lifetime: 7d", "max_lifetime: -1h"}, {"idle", "idle: skip", "idle: bogus"}, {"budget", "concurrent_runs: 3", "concurrent_runs: -1"},
		{"scope", "selector: \"issue=REL-*\"", "selector: \"all\""}, {"scope typo", "selector: \"issue=REL-*\"", "selectr: \"issue=REL-*\""},
		{"duplicate", "name: release-train", "name: release-train\nname: other"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, e := ParseOrchestrator(strings.Replace(orchestratorExample, tt.old, tt.new, 1)); e == nil {
				t.Fatal("accepted invalid spec")
			}
		})
	}
	if !s.Includes(Order{Name: "work", Repo: "app", Issue: "REL-1"}) || s.Includes(Order{Name: "work", Repo: "other", Issue: "REL-1"}) {
		t.Fatal("scope mismatch")
	}
}
func TestOrchestratorIdleRequiredCheckpointRotation(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	st, _ := orchestratorState(s.Name)
	calls := 0
	missing := false
	fail := false
	inbox := ""
	d := OrchestratorDriver{Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
		return []OrchestratorEvidence{}, nil
	}, Session: func(_ context.Context, _ OrchestratorSpec, dir string, p OrchestratorPacket) error {
		calls++
		if p.Inbox != inbox {
			t.Fatal("packet inbox mismatch")
		}
		if fail {
			return errors.New("session failed")
		}
		if missing {
			return nil
		}
		return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Goal: pending\nNext: inspect\n"), 0600)
	}}
	round := func() error { return RunOrchestratorRound(context.Background(), s, &st, d) }
	if e := round(); e != nil {
		t.Fatal(e)
	}
	if e := round(); e != nil {
		t.Fatal(e)
	}
	if calls != 1 {
		t.Fatal("unchanged evidence called model")
	}
	inbox = "new instruction"
	_ = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "inbox.md"), []byte(inbox), 0600)
	missing = true
	if e := round(); e == nil || !strings.Contains(e.Error(), "checkpoint") {
		t.Fatalf("missing checkpoint accepted: %v", e)
	}
	b, _ := os.ReadFile(filepath.Join(OrchestratorDir(s.Name), "checkpoint.md"))
	if !strings.Contains(string(b), "pending") {
		t.Fatal("previous checkpoint lost")
	}
	missing = false
	fail = true
	if e := round(); e == nil {
		t.Fatal("session failure accepted")
	}
	fail = false
	if e := round(); e != nil {
		t.Fatal(e)
	}
	if st.Lineage != 0 || !exists(filepath.Join(OrchestratorDir(s.Name), "lineages", "004.md")) {
		t.Fatal("rotation did not archive")
	}
	if e := round(); e != nil {
		t.Fatal(e)
	}
	if calls != 4 {
		t.Fatalf("calls=%d", calls)
	}
}
func TestOrchestratorAdmissionAndBudgets(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	dir := filepath.Join(Root(), "work")
	_ = os.MkdirAll(dir, 0700)
	m := Meta{Order: Order{Name: "work", Repo: "app", Issue: "REL-1"}}
	_ = WriteJSON(filepath.Join(dir, "meta.json"), m)
	q := Request{Orchestrator: s.Name, Name: "work", Action: "say"}
	if e := admitOrchestrator(q); e != nil {
		t.Fatal(e)
	}
	for _, a := range []string{"merge", "stop", "amend", "clean", "amend-import"} {
		q.Action = a
		if e := admitOrchestrator(q); e == nil {
			t.Fatalf("accepted unauthorized %s", a)
		}
	}
	q.Action = "say"
	q.Name = "outside"
	if e := admitOrchestrator(q); e == nil {
		t.Fatal("accepted out of scope")
	}
	q.Name = "work"
	m.Repo = "outside"
	if e := WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
		t.Fatal(e)
	}
	if _, e := Handle(q); e == nil || !strings.Contains(e.Error(), "outside manages scope") {
		t.Fatalf("scope entrypoint rejection: %v", e)
	}
	m.Repo = "app"
	if e := WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HEV_ORCHESTRATOR", s.Name)
	if _, e := Handle(Request{Name: "work", Action: "merge"}); e == nil {
		t.Fatal("environment identity bypassed")
	}
	t.Setenv("HEV_ORCHESTRATOR", "")
	q.Action = "rework"
	// The public target entrypoint must reject before inspecting processes.
	q.Action = "merge"
	if _, e := Handle(q); e == nil {
		t.Fatal("Handle bypassed admission")
	}
	q.Action = "rework"
	if e := reserveOrchestratorBudget(q, map[string]bool{}); e != nil {
		t.Fatal(e)
	}
	for i := 1; i < 20; i++ {
		if e := reserveOrchestratorBudget(q, nil); e != nil {
			t.Fatal(e)
		}
	}
	if e := reserveOrchestratorBudget(q, nil); e == nil {
		t.Fatal("daily budget exceeded")
	}
	_ = os.Remove(filepath.Join(OrchestratorDir(s.Name), "budget.json"))
	live := map[string]bool{}
	for _, n := range []string{"one", "two", "three"} {
		live[n] = true
		m.Name = n
		d := filepath.Join(Root(), n)
		_ = os.MkdirAll(d, 0700)
		_ = WriteJSON(filepath.Join(d, "meta.json"), m)
	}
	if e := reserveOrchestratorBudget(q, live); e == nil {
		t.Fatal("concurrency budget exceeded")
	}
	if e := OrchestratorControl(s.Name, "pause", ""); e != nil {
		t.Fatal(e)
	}
	if e := admitOrchestrator(q); e == nil {
		t.Fatal("paused orchestrator admitted")
	}
}
func TestOrchestratorNesting(t *testing.T) {
	base, _ := ParseOrchestrator(orchestratorExample)
	mk := func(n, child string, allow bool) OrchestratorSpec {
		s := base
		s.Name = n
		s.AllowNested = allow
		s.Manages.Runs.Selector = ""
		s.Manages.Runs.Names = []string{child}
		return s
	}
	for _, tt := range []struct {
		name  string
		specs map[string]OrchestratorSpec
		bad   bool
	}{
		{"ordinary", map[string]OrchestratorSpec{"a": mk("a", "work", false)}, false},
		{"guard", map[string]OrchestratorSpec{"a": mk("a", "b", false), "b": mk("b", "work", false)}, true},
		{"allowed", map[string]OrchestratorSpec{"a": mk("a", "b", true), "b": mk("b", "work", false)}, false},
		{"cycle", map[string]OrchestratorSpec{"a": mk("a", "b", true), "b": mk("b", "a", true)}, true},
		{"depth", map[string]OrchestratorSpec{"a": mk("a", "b", true), "b": mk("b", "c", true), "c": mk("c", "work", false)}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if e := ValidateOrchestratorNesting(tt.specs); (e != nil) != tt.bad {
				t.Fatalf("error=%v", e)
			}
		})
	}
}
func TestOrchestratorSupervisorPauseWakeHandoff(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	clock := time.Now()
	calls := 0
	waits := 0
	_ = OrchestratorControl(s.Name, "pause", "")
	d := OrchestratorDriver{Now: func() time.Time { return clock }, Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }, Session: func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
		calls++
		if calls == 1 {
			return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Wait: review; release: green\n"), 0600)
		}
		t.Fatal("unexpected call")
		return nil
	}, Wait: func(context.Context) error {
		waits++
		clock = clock.Add(time.Second)
		switch waits {
		case 1:
			if calls != 0 {
				t.Fatal("pause ignored")
			}
			_ = os.Remove(filepath.Join(OrchestratorDir(s.Name), "pause"))
		case 2:
			_ = OrchestratorControl(s.Name, "wake", "")
		case 3:
			_ = OrchestratorControl(s.Name, "handoff", "new-owner")
		case 10:
			t.Fatal("supervisor did not stop")
		}
		return nil
	}}
	if e := SuperviseOrchestrator(context.Background(), s.Name, d); e != nil {
		t.Fatal(e)
	}
	st, _ := orchestratorState(s.Name)
	if st.State != "handed-off" || calls != 1 {
		t.Fatalf("state=%+v calls=%d", st, calls)
	}
	b, _ := os.ReadFile(filepath.Join(OrchestratorDir(s.Name), "checkpoint.md"))
	if !strings.Contains(string(b), "new-owner") {
		t.Fatal("handoff missing recipient")
	}
}
func TestOrchestratorDefaultOff(t *testing.T) {
	orchHome(t)
	rows, e := ListOrchestrators()
	if e != nil || len(rows) != 0 {
		t.Fatalf("rows %v: %v", rows, e)
	}
	if exists(OrchestratorConfigDir()) || exists(Root()) {
		t.Fatal("read-only inventory created orchestrator state")
	}
}

func TestOrchestratorRoundEvidenceAndIdleRun(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	st, _ := orchestratorState(s.Name)
	head := "a"
	calls := 0
	d := OrchestratorDriver{Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
		return []OrchestratorEvidence{{Row: Row{Head: head}}}, nil
	}, Session: func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
		calls++
		return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Next: inspect"), 0600)
	}}
	for _, next := range []string{"a", "a", "b"} {
		head = next
		if e := RunOrchestratorRound(context.Background(), s, &st, d); e != nil {
			t.Fatal(e)
		}
	}
	if calls != 2 {
		t.Fatalf("head change calls %d", calls)
	}
	s.Idle = "run"
	if e := RunOrchestratorRound(context.Background(), s, &st, d); e != nil {
		t.Fatal(e)
	}
	if calls != 3 {
		t.Fatal("idle run skipped model")
	}
	// Reapply must invalidate evidence identity even if only the brief changes.
	if e := WriteJSON(filepath.Join(OrchestratorDir(s.Name), "state.json"), st); e != nil {
		t.Fatal(e)
	}
	if _, e := ApplyOrchestrator(orchestratorExample+"New priority.\n", false); e != nil {
		t.Fatal(e)
	}
	st, _ = orchestratorState(s.Name)
	if st.Digest != "" {
		t.Fatal("reapply reused old spec digest")
	}
}
func TestOrchestratorSupervisorBounds(t *testing.T) {
	for _, kind := range []string{"stop", "lifetime", "date", "goal", "missing-checkpoint"} {
		t.Run(kind, func(t *testing.T) {
			orchHome(t)
			s := applyOrch(t)
			st, _ := orchestratorState(s.Name)
			clock := st.Start
			calls := 0
			switch kind {
			case "stop":
				_ = OrchestratorControl(s.Name, "stop", "")
			case "lifetime":
				clock = clock.Add(8 * 24 * time.Hour)
			case "date":
				text := strings.Replace(orchestratorExample, "until: goal", "until: 2020-01-01", 1)
				var e error
				s, e = ApplyOrchestrator(text, false)
				if e != nil {
					t.Fatal(e)
				}
			case "missing-checkpoint":
				text := strings.Replace(orchestratorExample, "mode: loop", "mode: once", 1)
				var e error
				s, e = ApplyOrchestrator(text, false)
				if e != nil {
					t.Fatal(e)
				}
			}
			d := OrchestratorDriver{Now: func() time.Time { return clock }, Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }, Wait: func(context.Context) error { t.Fatal("unexpected wait"); return nil }, Session: func(ctx context.Context, s OrchestratorSpec, dir string, _ OrchestratorPacket) error {
				calls++
				if e := SuperviseOrchestrator(ctx, s.Name, OrchestratorDriver{}); e == nil {
					t.Fatal("overlapping supervisor admitted")
				}
				if kind == "missing-checkpoint" {
					return nil
				}
				if e := os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Goal: complete"), 0600); e != nil {
					return e
				}
				return WriteJSON(filepath.Join(dir, "goal-met.json"), map[string]bool{"met": true})
			}}
			e := SuperviseOrchestrator(context.Background(), s.Name, d)
			if (e != nil) != (kind == "missing-checkpoint") {
				t.Fatalf("error %v", e)
			}
			st, _ = orchestratorState(s.Name)
			want := "complete"
			if kind == "stop" {
				want = "stopped"
			}
			if kind == "missing-checkpoint" {
				want = "failed"
			}
			if st.State != want {
				t.Fatalf("state %s want %s", st.State, want)
			}
			expected := 0
			if kind == "goal" || kind == "missing-checkpoint" {
				expected = 1
			}
			if calls != expected {
				t.Fatalf("calls %d want %d", calls, expected)
			}
		})
	}
}
func TestOrchestratorChildAdmission(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	t.Setenv("HEV_ORCHESTRATOR", s.Name)
	if _, e := ApplyOrchestrator(strings.Replace(orchestratorExample, "name: release-train", "name: child", 1), true); e == nil {
		t.Fatal("nested apply bypassed guard")
	}
	if e := OrchestratorControl(s.Name, "stop", ""); e == nil {
		t.Fatal("self lifecycle bypassed guard")
	}
}
func TestOrchestratorDailyReset(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	p := filepath.Join(OrchestratorDir(s.Name), "budget.json")
	if e := WriteJSON(p, map[string]any{"day": "2000-01-01", "runs": 100}); e != nil {
		t.Fatal(e)
	}
	if e := reserveOrchestratorBudget(Request{Orchestrator: s.Name}, nil); e != nil {
		t.Fatal(e)
	}
	var b struct {
		Runs int `json:"runs"`
	}
	if e := readJSON(p, &b); e != nil || b.Runs != 1 {
		t.Fatalf("daily reset %+v %v", b, e)
	}
}

func TestOrchestratorOwnerBudgetAcrossHosts(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	row := func(n, state string) Row {
		return Row{Meta: Meta{Order: Order{Name: n, Repo: "app", Issue: "REL-1"}}, State: state}
	}
	inv := map[string]Response{"first": {Rows: []Row{row("one", "running"), row("done", "done")}}, "second": {Rows: []Row{row("two", "running"), row("three", "launch-unconfirmed")}}}
	if e := reserveOrchestratorInventoryCounts(s, inv); e == nil {
		t.Fatal("cross-host concurrent budget bypassed")
	}
	inv["second"] = Response{Rows: []Row{row("two", "done")}}
	if e := reserveOrchestratorInventoryCounts(s, inv); e != nil {
		t.Fatal(e)
	}
	if !exists(filepath.Join(OrchestratorDir(s.Name), "budget-owner.json")) {
		t.Fatal("owner receipt missing")
	}
	inv["second"] = Response{Error: "unreachable"}
	if e := reserveOrchestratorInventoryCounts(s, inv); e == nil {
		t.Fatal("unreachable target hidden from budget")
	}
}

func TestExpediteOncePresetRetainsLegacyCadence(t *testing.T) {
	orchHome(t)
	text := strings.Replace(orchestratorExample, "mode: loop", "mode: once", 1)
	text = strings.Replace(text, "idle: skip", "idle: run", 1)
	s, e := ApplyOrchestrator(text, false)
	if e != nil {
		t.Fatal(e)
	}
	legacy := strings.Replace(good, "name: example", "name: expedite-test\nevery: 7m\nmax_lifetime: 2h\nuntil: all followed runs merged or abandoned", 1)
	if e = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "order.md"), []byte(legacy), 0600); e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	calls := 0
	waits := 0
	d := OrchestratorDriver{Now: func() time.Time { return now }, Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }}
	d.Session = func(_ context.Context, spec OrchestratorSpec, dir string, p OrchestratorPacket) error {
		calls++
		if spec.Mode != "once" || p.Spec.Mode != "once" {
			t.Fatal("adapter changed preset declaration")
		}
		if calls == 2 {
			if e := OrchestratorControl(s.Name, "stop", ""); e != nil {
				t.Fatal(e)
			}
		}
		return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("pending"), 0600)
	}
	d.Wait = func(context.Context) error {
		waits++
		st, e := orchestratorState(s.Name)
		if e != nil {
			t.Fatal(e)
		}
		if st.Next.Sub(now) != 7*time.Minute {
			t.Fatal("legacy every ignored", st.Next.Sub(now))
		}
		now = st.Next
		return nil
	}
	if e := SuperviseOrchestrator(context.Background(), s.Name, d); e != nil {
		t.Fatal(e)
	}
	if calls != 2 || waits != 1 {
		t.Fatal(calls, waits)
	}
}
