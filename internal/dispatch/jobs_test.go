package dispatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/secrets"
)

func jobFixture(t *testing.T) JobSpec {
	t.Helper()
	host, _ := os.Hostname()
	text := fmt.Sprintf(`---
kind: job
name: promo-test
host: %s
exec:
  argv: [/bin/zsh, -f, scripts/run-iteration-channel.sh]
  cwd: %s
schedule:
  every: 30m
  catch_up: none
secrets:
  POST_TOKEN: secret://post-token
  SOURCE_TOKEN: secret://source-token
  MODEL_TOKEN: secret://model-token
timeout: 6h
overlap: skip
retry: {attempts: 0}
reply: operator-session
---
Stub for the three-secret promo runner.
`, host, t.TempDir())
	s, e := ParseJob([]byte(text))
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func jobHome(t *testing.T) {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(h, "dispatch.toml"))
	if err := os.WriteFile(filepath.Join(h, "dispatch.toml"), []byte("cap = 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
}
func mustJob(t *testing.T, name string) JobState {
	t.Helper()
	s, e := Job(name)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

type fakeJobs struct {
	now       time.Time
	starts    []string
	env       []string
	outcomes  map[string]string
	calls     []string
	secretErr error
	startErr  error
	notices   []OrchestratorNotice
}

func (f *fakeJobs) driver() JobDriver {
	return JobDriver{
		Now: func() time.Time { return f.now },
		Resolve: func(_ context.Context, caller string, refs []secrets.Ref) (map[string]string, error) {
			f.calls = append(f.calls, caller)
			if f.secretErr != nil {
				return nil, f.secretErr
			}
			out := map[string]string{}
			for _, r := range refs {
				out[r.Name] = "child-only-value"
			}
			return out, nil
		},
		Start: func(name, host string, s ExecSpec, env []string) error {
			if f.startErr != nil {
				return f.startErr
			}
			f.starts = append(f.starts, name)
			f.env = env
			f.outcomes[name] = "running"
			return nil
		},
		Outcome: func(name string) (string, *Result) {
			s := f.outcomes[name]
			if s == "" {
				return "absent", nil
			}
			return s, nil
		},
		Notify: func(_ context.Context, n OrchestratorNotice) error { f.notices = append(f.notices, n); return nil },
	}
}
func (f *fakeJobs) tick(t *testing.T, recovery bool) {
	t.Helper()
	if e := TickJobs(context.Background(), f.driver(), recovery); e != nil {
		t.Fatal(e)
	}
}
func newFakeJobs() *fakeJobs {
	return &fakeJobs{now: time.Now().UTC().Truncate(time.Second), outcomes: map[string]string{}}
}
func applyFakeJob(t *testing.T, f *fakeJobs, catch string) JobSpec {
	t.Helper()
	s := jobFixture(t)
	s.Schedule.Every = "1m"
	s.Schedule.CatchUp = catch
	if _, e := ApplyJob(s, f.now, false, false); e != nil {
		t.Fatal(e)
	}
	return s
}

func TestJobsFixedSlotsOverlapAndRestart(t *testing.T) {
	jobHome(t)
	f := newFakeJobs()
	s := applyFakeJob(t, f, "none")
	anchor := f.now
	f.tick(t, true)
	if len(f.starts) != 0 {
		t.Fatal("new job fired early")
	}
	f.now = anchor.Add(time.Minute)
	f.tick(t, false)
	if len(f.starts) != 1 {
		t.Fatal("first slot missing")
	}
	first := f.starts[0]
	f.now = anchor.Add(2 * time.Minute)
	f.tick(t, false)
	state := mustJob(t, s.Name)
	if len(state.History) != 2 || state.History[1].Reason != "overlap" {
		t.Fatalf("overlap not recorded: %+v", state.History)
	}
	// A new scheduler driver has no in-memory cursor. Durable state and run
	// identity prevent a duplicate even when observation time goes backwards.
	f.tick(t, true)
	f.now = anchor.Add(time.Minute)
	f.tick(t, true)
	if len(f.starts) != 1 {
		t.Fatal("slot submitted twice")
	}
	f.outcomes[first] = "done"
	f.now = anchor.Add(2*time.Minute + 30*time.Second)
	f.tick(t, true)
	state = mustJob(t, s.Name)
	if !state.Next[0].Equal(anchor.Add(3 * time.Minute)) {
		t.Fatal("restart mid-wait changed next slot", state.Next)
	}
	f.now = anchor.Add(3 * time.Minute)
	f.tick(t, false)
	if len(f.starts) != 2 {
		t.Fatal("next slot lost")
	}
	if !mustJob(t, s.Name).History[2].Slot.Equal(anchor.Add(3 * time.Minute)) {
		t.Fatal("interval drift")
	}
}
func TestJobsCatchUpAfterRestartGap(t *testing.T) {
	for _, mode := range []string{"none", "one"} {
		t.Run(mode, func(t *testing.T) {
			jobHome(t)
			f := newFakeJobs()
			s := applyFakeJob(t, f, mode)
			f.now = f.now.Add(10*time.Minute + 15*time.Second)
			f.tick(t, true)
			state := mustJob(t, s.Name)
			if state.Cursor != 10 {
				t.Fatal("cursor", state.Cursor)
			}
			if mode == "none" {
				if len(f.starts) != 0 || state.History[0].Missed != 10 {
					t.Fatal("none replayed backlog")
				}
			} else {
				if len(f.starts) != 1 || state.History[0].Missed != 9 {
					t.Fatal("one did not coalesce")
				}
			}
			f.tick(t, true)
			if len(f.starts) > 1 {
				t.Fatal("restart repeated catch-up")
			}
		})
	}
}
func TestJobsPauseResumeRemoveAndReapply(t *testing.T) {
	jobHome(t)
	f := newFakeJobs()
	s := applyFakeJob(t, f, "one")
	anchor := f.now
	if _, e := ControlJob(s.Name, "rm", f.now); e == nil {
		t.Fatal("removed enabled job")
	}
	if _, e := ControlJob(s.Name, "pause", f.now); e != nil {
		t.Fatal(e)
	}
	if _, e := ControlJob(s.Name, "run", f.now); e == nil {
		t.Fatal("ran paused")
	}
	f.now = f.now.Add(5 * time.Minute)
	if _, e := ControlJob(s.Name, "resume", f.now); e != nil {
		t.Fatal(e)
	}
	f.tick(t, false)
	if len(f.starts) != 0 {
		t.Fatal("resume replayed paused interval")
	}
	s.Timeout = "2h"
	if _, e := ApplyJob(s, f.now, false, false); e != nil {
		t.Fatal(e)
	}
	state := mustJob(t, s.Name)
	if !state.Anchor.Equal(anchor) || state.Cursor != 5 {
		t.Fatal("other edit reset anchor")
	}
	s.Schedule.Every = "2m"
	if _, e := ApplyJob(s, f.now, false, false); e != nil {
		t.Fatal(e)
	}
	state = mustJob(t, s.Name)
	if !state.Anchor.Equal(f.now) || state.Cursor != 0 {
		t.Fatal("schedule edit did not reset")
	}
	if _, e := ControlJob(s.Name, "run", f.now); e != nil {
		t.Fatal(e)
	}
	f.tick(t, false)
	if _, e := ControlJob(s.Name, "run", f.now); e == nil {
		t.Fatal("manual overlapped")
	}
	if _, e := ControlJob(s.Name, "pause", f.now); e != nil {
		t.Fatal(e)
	}
	if _, e := ApplyJob(s, f.now, false, false); e != nil {
		t.Fatal(e)
	}
	if !mustJob(t, s.Name).Paused {
		t.Fatal("reapply resumed job")
	}
	if _, e := ControlJob(s.Name, "rm", f.now); e == nil {
		t.Fatal("removed active job")
	}
	f.outcomes[f.starts[0]] = "done"
	f.tick(t, false)
	if _, e := ControlJob(s.Name, "rm", f.now); e != nil {
		t.Fatal(e)
	}
	jobs, e := Jobs()
	if e != nil || len(jobs) != 0 {
		t.Fatal("removed schedule listed", e)
	}
	if len(mustJob(t, s.Name).History) == 0 {
		t.Fatal("history erased")
	}
	if _, e := ApplyJob(s, f.now, false, false); e != nil {
		t.Fatal(e)
	}
	if !mustJob(t, s.Name).Anchor.Equal(f.now) {
		t.Fatal("recreated anchor")
	}
}
func TestJobsSecretEnvironmentAttributionAndBudgetDeadline(t *testing.T) {
	jobHome(t)
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "must-not-inherit")
	t.Setenv("HEV_SECRETS_CACHE", "off")
	t.Setenv("UNRELATED_SECRET", "must-not-inherit")
	f := newFakeJobs()
	s := applyFakeJob(t, f, "none")
	s.Timeout = "10s"
	if _, e := ApplyJob(s, f.now, false, false); e != nil {
		t.Fatal(e)
	}
	f.now = f.now.Add(time.Minute)
	f.secretErr = errors.New("host budget exhausted")
	f.tick(t, false)
	if len(f.starts) != 0 {
		t.Fatal("child launched after budget refusal")
	}
	o := mustJob(t, s.Name).History[0]
	if o.State != "queued" {
		t.Fatal("did not defer")
	}
	if len(f.calls) != 1 || f.calls[0] != "job:"+s.Name {
		t.Fatal("wrong caller", f.calls)
	}
	f.now = f.now.Add(11 * time.Second)
	f.tick(t, false)
	o = mustJob(t, s.Name).History[0]
	if o.State != "failed" || len(f.starts) != 0 {
		t.Fatal("deadline did not fail without launch", o.State)
	}
	if len(f.calls) != 1 {
		t.Fatal("read attempted after deadline")
	}
	f.secretErr = nil
	f.now = f.now.Add(49 * time.Second)
	f.tick(t, false)
	if len(f.starts) != 1 {
		t.Fatal("cache recovery did not launch")
	}
	env := strings.Join(f.env, "\n")
	if !strings.Contains(env, "POST_TOKEN=child-only-value") || !strings.Contains(env, "HEV_JOB_NAME="+s.Name) || !strings.Contains(env, "HEV_JOB_OCCURRENCE=") {
		t.Fatal("missing child env")
	}
	if strings.Contains(env, "must-not-inherit") || strings.Contains(env, "HEV_SECRETS_CACHE=") {
		t.Fatal("inherited bootstrap/cache/secret env")
	}
	b, e := os.ReadFile(jobPath(s.Name))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), "child-only-value") {
		t.Fatal("secret persisted")
	}
	if _, ok := os.LookupEnv("POST_TOKEN"); ok {
		t.Fatal("parent environment mutated")
	}
}
func TestJobsFailuresAggregateAndRecover(t *testing.T) {
	jobHome(t)
	f := newFakeJobs()
	s := applyFakeJob(t, f, "none")
	for i := 0; i < 3; i++ {
		f.now = f.now.Add(time.Minute)
		f.tick(t, false)
		f.outcomes[f.starts[len(f.starts)-1]] = "failed"
		f.now = f.now.Add(time.Second)
		f.tick(t, false)
	}
	if len(f.notices) != 1 {
		t.Fatal("failure notice spam", len(f.notices))
	}
	state := mustJob(t, s.Name)
	if state.Failures != 3 {
		t.Fatal("incident did not aggregate")
	}
	f.now = f.now.Add(time.Minute)
	f.tick(t, false)
	f.outcomes[f.starts[len(f.starts)-1]] = "done"
	f.now = f.now.Add(time.Second)
	f.tick(t, false)
	if len(f.notices) != 2 || f.notices[1].Kind != "recovery" {
		t.Fatal("missing recovery")
	}
}
func TestJobsUnknownHoldsUntilOwnerReconciles(t *testing.T) {
	jobHome(t)
	f := newFakeJobs()
	s := applyFakeJob(t, f, "one")
	f.now = f.now.Add(time.Minute)
	f.tick(t, false)
	id := f.starts[0]
	f.outcomes[id] = "unknown"
	f.now = f.now.Add(time.Second)
	f.tick(t, true)
	if !mustJob(t, s.Name).Held {
		t.Fatal("ambiguous work not held")
	}
	f.now = f.now.Add(10 * time.Minute)
	f.tick(t, true)
	if len(f.starts) != 1 {
		t.Fatal("replayed ambiguous work")
	}
	if _, e := ControlJob(s.Name, "resume", f.now); e == nil {
		t.Fatal("resumed held work")
	}
	if _, e := ReconcileJob(s.Name, id, "failed", "owner verified no external effect", f.now); e != nil {
		t.Fatal(e)
	}
	state := mustJob(t, s.Name)
	if state.Held || !state.Paused || state.Active != "" {
		t.Fatal("bad reconciled state")
	}
}
func TestJobsDryRunAndPlacement(t *testing.T) {
	jobHome(t)
	s := jobFixture(t)
	if _, e := ApplyJob(s, time.Now(), false, true); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(JobsRoot()); !os.IsNotExist(e) {
		t.Fatal("dry run mutated state")
	}
	s.Host = "not-this-host"
	if _, e := ApplyJob(s, time.Now(), false, false); e == nil {
		t.Fatal("placement silently overridden")
	}
}

func TestJobsCompletionAfterDueSlotStillSkipsOverlap(t *testing.T) {
	jobHome(t)
	f := newFakeJobs()
	s := applyFakeJob(t, f, "none")
	anchor := f.now
	f.now = anchor.Add(time.Minute)
	f.tick(t, false)
	first := f.starts[0]
	f.now = anchor.Add(2*time.Minute + time.Second)
	d := f.driver()
	d.Outcome = func(name string) (string, *Result) {
		if name == first {
			return "done", &Result{End: anchor.Add(2*time.Minute + 500*time.Millisecond)}
		}
		return "absent", nil
	}
	if e := TickJobs(context.Background(), d, false); e != nil {
		t.Fatal(e)
	}
	state := mustJob(t, s.Name)
	if len(f.starts) != 1 || state.History[1].Reason != "overlap" {
		t.Fatal("admitted an overlapping logical slot")
	}
}
func TestJobsUnsupportedSchemaHoldsOnlyThatJob(t *testing.T) {
	jobHome(t)
	f := newFakeJobs()
	s := applyFakeJob(t, f, "none")
	state := mustJob(t, s.Name)
	state.Schema = 2
	if e := saveJob(state); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(jobPath(s.Name))
	f.now = f.now.Add(time.Minute)
	f.tick(t, true)
	after, _ := os.ReadFile(jobPath(s.Name))
	if string(before) != string(after) {
		t.Fatal("unsupported schema rewritten")
	}
	if !mustJob(t, s.Name).Held || len(f.starts) != 0 || len(f.notices) != 1 {
		t.Fatal("unsupported schema not held/notified")
	}
}

func TestJobLogRedactsSecretsAcrossWrites(t *testing.T) {
	var out bytes.Buffer
	l := newCommandLog(&out, []string{"TOKEN=one-secret-value", "OTHER=other-secret"})
	for _, chunk := range []string{"prefix one-se", "cret-value other-", "secret suffix"} {
		if _, e := l.Write([]byte(chunk)); e != nil {
			t.Fatal(e)
		}
	}
	if e := l.Flush(); e != nil {
		t.Fatal(e)
	}
	if out.String() != "prefix [REDACTED] [REDACTED] suffix" {
		t.Fatal("split secret leaked or output lost")
	}
}
func TestJobUnknownAdmissionCannotBeReclassifiedByDispatchInspection(t *testing.T) {
	jobHome(t)
	dir := filepath.Join(Root(), "j-unknown")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Exec: &ExecSpec{}}); e != nil {
		t.Fatal(e)
	}
	r := Result{Start: time.Now().Add(-time.Hour), ExitCode: 1, LaunchUnconfirmed: true}
	if e := WriteJSON(filepath.Join(dir, "result.json"), r); e != nil {
		t.Fatal(e)
	}
	reconcileLaunch(dir, &r)
	if !r.LaunchUnconfirmed || !r.End.IsZero() {
		t.Fatal("inspection resolved ambiguous command")
	}
	state, _ := ExecOutcome("j-unknown")
	if state != "unknown" {
		t.Fatal("ambiguous result not held", state)
	}
}
func TestJobRetentionKeepsLedgerAndUnknownLogs(t *testing.T) {
	jobHome(t)
	s := jobFixture(t)
	now := time.Now()
	state := JobState{Spec: s}
	for _, outcome := range []string{"succeeded", "unknown"} {
		name := "j-retain-" + outcome
		dir := filepath.Join(Root(), name)
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		if e := WriteJSON(filepath.Join(dir, "result.json"), Result{Start: now.Add(-16 * 24 * time.Hour), End: now.Add(-15 * 24 * time.Hour), ExitCode: 0}); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(dir, "log"), []byte("old log"), 0600); e != nil {
			t.Fatal(e)
		}
		state.History = append(state.History, JobOccurrence{ID: name, Run: name, State: outcome, End: now.Add(-15 * 24 * time.Hour)})
	}
	if e := cleanupJobLogs(state, now); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(Root(), "j-retain-succeeded", "log")); !os.IsNotExist(e) {
		t.Fatal("terminal log not expired")
	}
	if _, e := os.Stat(filepath.Join(Root(), "j-retain-unknown", "log")); e != nil {
		t.Fatal("unknown log removed")
	}
	if _, e := os.Stat(filepath.Join(Root(), "j-retain-succeeded", "result.json")); e != nil {
		t.Fatal("compact result removed")
	}
}

func TestJobsPausePreventsDeferredAdmissionAndAllowsCancellation(t *testing.T) {
	jobHome(t)
	f := newFakeJobs()
	s := applyFakeJob(t, f, "none")
	f.secretErr = errors.New("cache unavailable")
	f.now = f.now.Add(time.Minute)
	f.tick(t, false)
	id := mustJob(t, s.Name).Active
	if _, e := ControlJob(s.Name, "pause", f.now); e != nil {
		t.Fatal(e)
	}
	f.secretErr = nil
	f.now = f.now.Add(time.Second)
	f.tick(t, false)
	if len(f.starts) != 0 {
		t.Fatal("paused pending work was admitted")
	}
	if _, e := ReconcileJob(s.Name, id, "cancelled", "owner cancelled unadmitted work", f.now); e != nil {
		t.Fatal(e)
	}
	if _, e := ControlJob(s.Name, "rm", f.now); e != nil {
		t.Fatal("cancelled pending work blocked removal", e)
	}
}
