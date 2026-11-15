package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/secrets"
)

type JobOccurrence struct {
	ID         string    `json:"id"`
	Run        string    `json:"run"`
	Revision   string    `json:"revision"`
	Slot       time.Time `json:"slot"`
	Reserved   time.Time `json:"reserved"`
	Deadline   time.Time `json:"deadline"`
	End        time.Time `json:"end,omitempty"`
	State      string    `json:"state"`
	Reason     string    `json:"reason,omitempty"`
	Missed     int64     `json:"missed,omitempty"`
	MissedFrom time.Time `json:"missed_from,omitempty"`
	Manual     bool      `json:"manual,omitempty"`
	Spec       JobSpec   `json:"spec"`
	Result     *Result   `json:"result,omitempty"`
}
type JobState struct {
	BudgetUsage []secrets.SummaryRow `json:"budget_usage,omitempty"`
	BudgetError string               `json:"budget_error,omitempty"`
	Schema      int                  `json:"schema"`
	Spec        JobSpec              `json:"spec"`
	Revision    string               `json:"revision"`
	Generation  int64                `json:"generation"`
	Anchor      time.Time            `json:"anchor"`
	Cursor      int64                `json:"cursor"`
	Paused      bool                 `json:"paused"`
	Held        bool                 `json:"held"`
	Removed     bool                 `json:"removed"`
	Active      string               `json:"active,omitempty"`
	Sequence    int64                `json:"sequence"`
	Incident    string               `json:"incident,omitempty"`
	Failures    int64                `json:"failures"`
	History     []JobOccurrence      `json:"history"`
	Notices     []OrchestratorNotice `json:"notices,omitempty"`
	Next        []time.Time          `json:"next"`
}

func JobsRoot() string           { h, _ := os.UserHomeDir(); return filepath.Join(h, ".hev", "jobs") }
func jobPath(name string) string { return filepath.Join(JobsRoot(), name+".json") }
func lockJobs() (func(), error) {
	if e := platformops.MkdirAll(JobsRoot(), 0700); e != nil {
		return nil, e
	}
	return platformops.LockPath(filepath.Join(JobsRoot(), "state.lock"), false)
}
func saveJob(s JobState) error {
	s.Next = nil
	s.Notices = nil
	s.BudgetUsage = nil
	s.BudgetError = ""
	b, e := json.MarshalIndent(s, "", "  ")
	if e != nil {
		return e
	}
	return platformops.WriteFileAtomic(jobPath(s.Spec.Name), append(b, '\n'), 0600)
}
func readJob(name string) (JobState, error) {
	var s JobState
	if !jobNameRE.MatchString(name) {
		return s, fmt.Errorf("invalid job name")
	}
	e := readJSON(jobPath(name), &s)
	if e == nil && s.Schema != 1 {
		s.Held = true
	}
	return s, e
}
func LocalJobHost(host string) bool {
	n, _ := os.Hostname()
	if host == n {
		return true
	}
	cfg, e := targetPolicy()
	if e == nil {
		if h, ok := cfg.Hosts[host]; ok && h.Local {
			return true
		}
	}
	return false
}
func ApplyJob(spec JobSpec, now time.Time, paused, dry bool) (JobState, error) {
	// Revalidate even library callers; host placement is receiver-owned.
	if spec.Secrets == nil {
		spec.Secrets = map[string]string{}
	}
	b, _ := json.Marshal(spec)
	// JSON keys match YAML keys; YAML accepts JSON frontmatter.
	parsed, e := ParseJob(append(append([]byte("---\n"), b...), []byte("\n---\n")...))
	if e != nil {
		return JobState{}, e
	}
	spec = parsed
	if !LocalJobHost(spec.Host) {
		return JobState{}, fmt.Errorf("job host is not local; remote jobs are not supported")
	}
	if dry {
		return JobState{Schema: 1, Spec: spec, Revision: jobDigest(spec)}, nil
	}
	unlock, e := lockJobs()
	if e != nil {
		return JobState{}, e
	}
	defer unlock()
	s, e := readJob(spec.Name)
	if e != nil && !os.IsNotExist(e) {
		return s, e
	}
	if e == nil && s.Schema != 1 {
		return s, fmt.Errorf("unsupported job schema: upgrade or restore a compatible build")
	}
	fresh := os.IsNotExist(e) || s.Removed
	reset := fresh || s.Spec.Schedule != spec.Schedule
	if fresh {
		s.Paused = paused
		s.Removed = false
		s.Held = false
	} else if paused {
		s.Paused = true
	}
	if reset {
		s.Anchor = now.UTC()
		s.Cursor = 0
		s.Generation++
	}
	s.Schema = 1
	s.Spec = spec
	s.Revision = jobDigest(spec)
	return s, saveJob(s)
}
func Job(name string) (JobState, error) {
	unlock, e := lockJobs()
	if e != nil {
		return JobState{}, e
	}
	defer unlock()
	s, e := readJob(name)
	if e != nil {
		return s, e
	}
	every, _ := JobDuration(s.Spec.Schedule.Every)
	for i := int64(1); every > 0 && i <= 5; i++ {
		s.Next = append(s.Next, s.Anchor.Add(time.Duration(s.Cursor+i)*every))
	}
	notices, e := loadOrchestratorNotices(jobNoticeName(name))
	if e != nil {
		return s, e
	}
	for _, n := range notices {
		s.Notices = append(s.Notices, n.n)
	}
	usage, usageErr := secrets.Summary(time.Now())
	if usageErr != nil {
		s.BudgetError = "host usage ledger unavailable"
	} else {
		for _, row := range usage {
			if row.Caller == "job:"+name {
				s.BudgetUsage = append(s.BudgetUsage, row)
			}
		}
	}
	return s, nil
}
func Jobs() ([]JobState, error) {
	entries, e := os.ReadDir(JobsRoot())
	if os.IsNotExist(e) {
		return []JobState{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []JobState{}
	for _, f := range entries {
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		name := f.Name()[:len(f.Name())-5]
		s, e := Job(name)
		if e != nil {
			return nil, e
		}
		if !s.Removed {
			out = append(out, s)
		}
	}
	return out, nil
}
func ControlJob(name, action string, now time.Time) (JobState, error) {
	unlock, e := lockJobs()
	if e != nil {
		return JobState{}, e
	}
	defer unlock()
	s, e := readJob(name)
	if e != nil {
		return s, e
	}
	if s.Removed {
		return s, fmt.Errorf("job removed")
	}
	if s.Schema != 1 {
		return s, fmt.Errorf("unsupported job schema")
	}
	switch action {
	case "pause":
		s.Paused = true
	case "resume":
		if s.Held {
			return s, fmt.Errorf("unknown work must be reconciled")
		}
		s.Paused = false
		every, _ := JobDuration(s.Spec.Schedule.Every)
		s.Cursor = max(s.Cursor, int64(now.Sub(s.Anchor)/every))
	case "rm":
		if !s.Paused || s.Active != "" || s.Held {
			return s, fmt.Errorf("remove requires paused job without active or unresolved work")
		}
		s.Removed = true
	case "run":
		if s.Paused || s.Held || s.Active != "" {
			return s, fmt.Errorf("job paused, held or busy")
		}
		reserveJob(&s, now, now, true, 0, time.Time{})
	default:
		return s, fmt.Errorf("unknown control")
	}
	return s, saveJob(s)
}
func reserveJob(s *JobState, slot, now time.Time, manual bool, missed int64, from time.Time) {
	s.Sequence++
	// A short hash keeps dispatch's 63-character name limit even for long jobs.
	id := fmt.Sprintf("j-%s-%d-%d", s.Revision[:12], s.Generation, s.Sequence)
	timeout, _ := JobDuration(s.Spec.Timeout)
	o := JobOccurrence{ID: id, Run: id, Revision: s.Revision, Slot: slot.UTC(), Reserved: now.UTC(), Deadline: now.Add(timeout).UTC(), State: "queued", Manual: manual, Missed: missed, MissedFrom: from, Spec: s.Spec}
	s.History = append(s.History, o)
	s.Active = id
}
func skipJob(s *JobState, slot, now time.Time, reason string, count int64, from time.Time) {
	s.Sequence++
	s.History = append(s.History, JobOccurrence{ID: fmt.Sprintf("skip-%d-%d", s.Generation, s.Sequence), Revision: s.Revision, Slot: slot.UTC(), Reserved: now.UTC(), End: now.UTC(), State: "skipped", Reason: reason, Missed: count, MissedFrom: from})
}

// JobDriver isolates time, cache and admission for deterministic restart tests.
// Production resolution is always cache-only and has no direct-read fallback.
type JobDriver struct {
	Now     func() time.Time
	Resolve func(context.Context, string, []secrets.Ref) (map[string]string, error)
	Start   func(string, string, ExecSpec, []string) error
	Outcome func(string) (string, *Result)
	Notify  OrchestratorNotifier
}

func (d JobDriver) defaults() JobDriver {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Resolve == nil {
		d.Resolve = func(ctx context.Context, caller string, refs []secrets.Ref) (map[string]string, error) {
			return secrets.ResolveWith(ctx, refs, secrets.Options{Caller: caller, CacheOnly: true})
		}
	}
	if d.Start == nil {
		d.Start = StartExec
	}
	if d.Outcome == nil {
		d.Outcome = ExecOutcome
	}
	if d.Notify == nil {
		d.Notify = orchestratorNoticeSender
	}
	return d
}

// TickJobs persists the slot cursor and reservation before any launch. recovery
// is true only on the first observation of a new scheduler process.
func TickJobs(ctx context.Context, driver JobDriver, recovery bool) error {
	d := driver.defaults()
	now := d.Now().UTC()
	unlock, e := lockJobs()
	if e != nil {
		return e
	}
	entries, e := os.ReadDir(JobsRoot())
	if e != nil {
		unlock()
		return e
	}
	var flush []OrchestratorSpec
	defer func() {
		unlock()
		for _, s := range flush {
			flushOrchestratorNotices(s, d.Notify)
		}
	}()
	for _, f := range entries {
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		s, e := readJob(f.Name()[:len(f.Name())-5])
		if e != nil {
			return e
		}
		if e = cleanupJobLogs(s, now); e != nil {
			return e
		}
		if s.Removed {
			continue
		}
		if s.Schema != 1 {
			spec := jobNoticeSpec(s.Spec)
			if e = platformops.MkdirAll(OrchestratorDir(spec.Name), 0700); e != nil {
				return e
			}
			if e = queueOrchestratorNotice(spec, "failure", "schema", spec.Reply, "job "+s.Spec.Name+": unsupported schema; scheduling held; upgrade or restore compatible build"); e != nil {
				return e
			}
			flush = append(flush, spec)
			continue
		}
		var busyUntil time.Time

		if s.Active != "" && !s.Held {
			for i := range s.History {
				if s.History[i].ID != s.Active {
					continue
				}
				o := &s.History[i]
				state, r := d.Outcome(o.Run)
				switch state {
				case "done", "failed", "timeout", "unknown", "died":
					if state == "done" {
						state = "succeeded"
					}
					if state == "died" {
						state = "unknown"
					}
					o.State = state
					o.Result = r
					o.End = now
					if r != nil && !r.End.IsZero() {
						o.End = r.End
						busyUntil = r.End
						if r.Stopped {
							o.State = "cancelled"
						}
					}
					if state == "unknown" {
						s.Held = true
						o.Reason = "uncertain dispatch admission; reconcile before resume"
					} else {
						s.Active = ""
					}
					if e = jobTransition(&s, *o); e != nil {
						return e
					}
				case "running", "queued":
					o.State = state
				case "absent":
					// No dispatch reservation exists. Secret/capacity waits retain this
					// occurrence and its original finite deadline.
					if !now.Before(o.Deadline) {
						o.State = "failed"
						o.Reason = "secrets or admission deadline expired"
						o.End = now
						s.Active = ""
						if e = jobTransition(&s, *o); e != nil {
							return e
						}
					}
				}
				break
			}
		}
		every, _ := JobDuration(s.Spec.Schedule.Every)
		last := int64(now.Sub(s.Anchor) / every)
		if last > s.Cursor {
			count := last - s.Cursor
			from := s.Anchor.Add(time.Duration(s.Cursor+1) * every)
			slot := s.Anchor.Add(time.Duration(last) * every)
			switch {
			case s.Paused || s.Held:
				skipJob(&s, slot, now, "paused or held", count, from)
			case s.Active != "" || !busyUntil.IsZero() && !busyUntil.Before(slot):
				skipJob(&s, slot, now, "overlap", count, from)
			case (recovery || count > 1) && s.Spec.Schedule.CatchUp == "none":
				skipJob(&s, slot, now, "catch_up: none", count, from)
			default:
				reserveJob(&s, slot, now, false, count-1, from)
			}
			s.Cursor = last
		}
		// Flush durable reservations before touching cache or dispatch.
		if e = saveJob(s); e != nil {
			return e
		}
		if s.Active != "" && !s.Held && !s.Paused {
			for i := range s.History {
				if s.History[i].ID != s.Active {
					continue
				}
				o := &s.History[i]
				state, _ := d.Outcome(o.Run)
				if state != "absent" {
					break
				}
				if !now.Before(o.Deadline) {
					break
				}
				refs := []secrets.Ref{}
				keys := []string{}
				for k := range o.Spec.Secrets {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					refs = append(refs, secrets.Ref{Name: k, URI: o.Spec.Secrets[k]})
				}
				values := map[string]string{}
				if len(refs) > 0 {
					waitCtx, cancel := context.WithDeadline(ctx, minTime(o.Deadline, time.Now().Add(2*time.Second)))
					values, e = d.Resolve(waitCtx, "job:"+s.Spec.Name, refs)
					cancel()
					if e != nil {
						o.Reason = "waiting for host secret cache"
						break
					}
				}
				env := MinimalExecEnvironment()
				complete := true
				for _, k := range keys {
					v, ok := values[k]
					if !ok {
						complete = false
						break
					}
					env = append(env, k+"="+v)
				}
				if !complete {
					o.Reason = "waiting for complete cache resolution"
					break
				}
				env = append(env, "HEV_JOB_NAME="+s.Spec.Name, "HEV_JOB_OCCURRENCE="+o.ID)
				spec := ExecSpec{Argv: o.Spec.Exec.Argv, Cwd: o.Spec.Exec.Cwd, Deadline: o.Deadline, Job: s.Spec.Name, Occurrence: o.ID}
				if e = d.Start(o.Run, o.Spec.Host, spec, env); e != nil {
					o.Reason = "waiting for dispatch admission"
				} else {
					o.State = "running"
					o.Reason = ""
				}
				break
			}
			if e = saveJob(s); e != nil {
				return e
			}
		}
		flush = append(flush, jobNoticeSpec(s.Spec))
	}
	return nil
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// The dot-prefixed internal name cannot collide with an owner orchestrator.
// Jobs reuse its durable queue, leases, bounded backoff and inbox transport.
func jobNoticeName(name string) string { return ".job-" + name }
func jobNoticeSpec(s JobSpec) OrchestratorSpec {
	return OrchestratorSpec{Name: jobNoticeName(s.Name), Host: s.Host, Reply: s.Reply}
}
func jobTransition(s *JobState, o JobOccurrence) error {
	if o.State == "cancelled" {
		return nil
	}
	spec := jobNoticeSpec(o.Spec)
	kind := "failure"
	key := o.ID
	if o.State == "succeeded" {
		if s.Incident == "" {
			return nil
		}
		kind = "recovery"
		key = s.Incident
	} else {
		if s.Incident != "" {
			s.Failures++
			return nil
		}
	}
	message := fmt.Sprintf("job %s on %s: %s; occurrence=%s run=%s; hev jobs show %s; hev jobs logs %s", s.Spec.Name, s.Spec.Host, o.State, o.ID, o.Run, s.Spec.Name, s.Spec.Name)
	if e := platformops.MkdirAll(OrchestratorDir(spec.Name), 0700); e != nil {
		return e
	}
	if e := queueOrchestratorNotice(spec, kind, key, spec.Reply, message); e != nil {
		return e
	}
	if kind == "failure" {
		s.Incident = o.ID
		s.Failures = 1
	} else {
		s.Incident = ""
		s.Failures = 0
	}
	return nil
}
func RunJobs(ctx context.Context, driver JobDriver) error {
	if e := platformops.MkdirAll(JobsRoot(), 0700); e != nil {
		return e
	}
	release, e := platformops.LockPath(filepath.Join(JobsRoot(), "scheduler.lock"), true)
	if e != nil {
		return fmt.Errorf("jobs scheduler already running: %w", e)
	}
	defer release()
	recovery := true
	// Use elapsed monotonic time while alive. A forward wall jump (including
	// waking from sleep on systems whose monotonic clock pauses) is recovery;
	// wall rollback cannot repeat accepted slots.
	boot := time.Now()
	wallAnchor := boot.UTC()
	customClock := driver.Now != nil

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if !customClock {
			observed := time.Now()
			logical := wallAnchor.Add(observed.Sub(boot))
			if observed.After(logical.Add(2 * time.Second)) {
				recovery = true
				boot = observed
				wallAnchor = observed.UTC()
				logical = wallAnchor
			}
			driver.Now = func() time.Time { return logical }
		}
		if e = TickJobs(ctx, driver, recovery); e != nil {
			return e
		}
		recovery = false
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// ReconcileJob records an owner's evidence for an uncertain occurrence. It
// cannot erase a live command's reservation or make it eligible for replay.
func ReconcileJob(name, id, outcome, evidence string, now time.Time) (JobState, error) {
	unlock, e := lockJobs()
	if e != nil {
		return JobState{}, e
	}
	defer unlock()
	s, e := readJob(name)
	if e != nil {
		return s, e
	}
	pendingCancel := false
	if s.Paused && outcome == "cancelled" && s.Active == id {
		for _, o := range s.History {
			if o.ID == id && o.State == "queued" {
				state, _ := ExecOutcome(o.Run)
				pendingCancel = state == "absent"
			}
		}
	}
	if (!s.Held && !pendingCancel) || s.Active != id || evidence == "" {
		return s, fmt.Errorf("reconcile requires held work or paused unadmitted work, and evidence")
	}
	if outcome != "succeeded" && outcome != "failed" && outcome != "cancelled" {
		return s, fmt.Errorf("invalid reconciliation outcome")
	}
	dir := filepath.Join(Root(), id)
	if supervisorLive(dir) || processRecordLive(filepath.Join(dir, "harness.json")) {
		return s, fmt.Errorf("run is still live")
	}
	for i := range s.History {
		if s.History[i].ID == id {
			o := &s.History[i]
			o.State = outcome
			o.Reason = evidence
			o.End = now.UTC()
			if e = jobTransition(&s, *o); e != nil {
				return s, e
			}
			s.Held = false
			s.Paused = true
			s.Active = ""
			return s, saveJob(s)
		}
	}
	return s, fmt.Errorf("occurrence not found")
}

// Only terminal command logs are eligible. Cwd is the owner's source directory
// and is never scratch to delete; compact slot, meta and result records remain.
func cleanupJobLogs(s JobState, now time.Time) error {
	for _, o := range s.History {
		if o.Run == "" || o.End.IsZero() || now.Sub(o.End) < 14*24*time.Hour {
			continue
		}
		if o.State != "succeeded" && o.State != "failed" && o.State != "timeout" && o.State != "cancelled" {
			continue
		}
		state, _ := ExecOutcome(o.Run)
		if state != "done" && state != "failed" && state != "timeout" {
			continue
		}
		e := os.Remove(filepath.Join(Root(), o.Run, "log"))
		if e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	return nil
}
