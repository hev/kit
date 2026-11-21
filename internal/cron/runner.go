package cron

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/secrets"
)

type Runner struct {
	Home       string
	Membership Membership
	Resolve    func(context.Context, []secrets.Ref) (map[string]string, error)
	Notice     func(context.Context, attention.Notice) error
	Logger     *slog.Logger
}

type Summary struct {
	Name     string     `json:"name"`
	Schedule string     `json:"schedule"`
	Status   string     `json:"status"`
	Winner   string     `json:"winner,omitempty"`
	Reason   string     `json:"reason,omitempty"`
	Next     *time.Time `json:"next_run,omitempty"`
	State    State      `json:"state"`
}

type Details struct {
	Summary
	Runs []Run `json:"runs"`
}

func (r Runner) dir(j Job) string {
	return filepath.Join(r.Home, ".hev", "cron", j.Name)
}

func (r Runner) Pause(ctx context.Context, j Job, paused bool) error {
	dir := r.dir(j)
	release, err := lockState(ctx, dir)
	if err != nil {
		return err
	}
	defer release()
	s, err := readState(dir)
	if err != nil {
		return err
	}
	s.Paused = paused
	return writeJSON(filepath.Join(dir, "state.json"), s)
}

func (r Runner) Inspect(ctx context.Context, j Job, history bool) (Details, error) {
	winner, reason := r.Membership.winner(ctx, j)
	d := Details{Summary: Summary{Name: j.Name, Schedule: j.Schedule(), Status: "ok", Winner: winner, Reason: reason}}
	if history {
		d.Runs = []Run{}
	}
	dir := r.dir(j)
	// Reading an unused job should not create its state directory.
	if _, err := os.Stat(dir); err == nil {
		unlock, err := lockState(ctx, dir)
		if err != nil {
			return d, err
		}
		defer unlock()
		if d.State, err = readState(dir); err != nil {
			return d, err
		}
		if history {
			if d.Runs, err = readRuns(dir); err != nil {
				return d, err
			}
		}
		release, err := platformops.LockPath(filepath.Join(dir, "run.lock"), true)
		if platformops.IsLockBusy(err) {
			d.Status = "running"
		} else if err != nil {
			return d, errStorage
		} else {
			release()
		}
	} else if !os.IsNotExist(err) {
		return d, errStorage
	}

	switch {
	case d.Status == "running":
	case !j.enabled():
		d.Status, d.Reason = "paused", "disabled in configuration"
	case d.State.Paused:
		d.Status, d.Reason = "paused", "paused locally"
	case reason != "":
		d.Status = "standby"
	case d.State.ConsecutiveFailures > 0:
		d.Status = "failing"
	}
	if j.enabled() && !d.State.Paused && reason == "" {
		next := j.Next(time.Now())
		d.Next = &next
	}
	return d, nil
}

type attempt struct {
	start   time.Time
	release func()
}

func (r Runner) admit(ctx context.Context, j Job) (attempt, Run, error) {
	dir := r.dir(j)
	releaseState, err := lockState(ctx, dir)
	if err != nil {
		return attempt{}, Run{}, err
	}
	defer releaseState()
	if err := ctx.Err(); err != nil {
		return attempt{}, Run{}, err
	}
	s, err := readState(dir)
	if err != nil {
		return attempt{}, Run{}, err
	}
	winner, reason := r.Membership.winner(ctx, j)
	now := time.Now().UTC()
	save := func() error { return writeJSON(filepath.Join(dir, "state.json"), s) }
	if winner != s.LastPlacementWinner {
		s.LastPlacementWinner = winner
		if err := save(); err != nil {
			return attempt{}, Run{}, err
		}
	}
	switch {
	case !j.enabled():
		return attempt{}, skip(now, "skipped_disabled", "disabled in configuration"), nil
	case s.Paused:
		return attempt{}, skip(now, "skipped_paused", "paused locally"), nil
	case reason != "":
		return attempt{}, skip(now, "skipped_standby", reason), nil
	}

	releaseRun, err := platformops.LockPath(filepath.Join(dir, "run.lock"), true)
	if platformops.IsLockBusy(err) {
		busy := skip(now, "skipped_busy", "job is already running")
		return attempt{}, busy, appendRun(dir, busy)
	}
	if err != nil {
		return attempt{}, Run{}, errStorage
	}
	keep := false
	defer func() {
		if !keep {
			releaseRun()
		}
	}()
	if s.LastStart != nil && j.cooldown > 0 && now.Before(s.LastStart.Add(j.cooldown)) {
		return attempt{}, skip(now, "skipped_cooldown", "local cooldown is active"), nil
	}
	cooling, err := r.Membership.cooling(ctx, j, now)
	if err != nil {
		return attempt{}, skip(now, "skipped_standby", "cluster cooldown state unavailable"), nil
	}
	if cooling {
		return attempt{}, skip(now, "skipped_cooldown", "cluster cooldown is active"), nil
	}
	s.LastStart = &now
	if err := save(); err != nil {
		return attempt{}, Run{}, err
	}
	keep = true
	return attempt{start: now, release: releaseRun}, Run{}, nil
}

func skip(now time.Time, status, message string) Run {
	return Run{Start: now, End: now, Status: status, Message: message}
}

func (r Runner) Run(ctx context.Context, j Job) (Run, error) {
	a, skipped, err := r.admit(ctx, j)
	if err != nil || a.release == nil {
		return skipped, err
	}
	return r.run(ctx, j, a)
}

func (r Runner) process(ctx context.Context, j Job, start time.Time) Run {
	ctx, cancel := context.WithDeadline(ctx, start.Add(j.timeout))
	defer cancel()
	result := Run{Start: start}
	var values map[string]string
	var err error
	if len(j.EnvRefs) > 0 {
		refs := make([]secrets.Ref, 0, len(j.EnvRefs))
		for name, uri := range j.EnvRefs {
			refs = append(refs, secrets.Ref{Name: name, URI: uri, TTL: 5 * time.Minute})
		}
		if r.Resolve == nil {
			err = errors.New("secret resolver unavailable")
		} else {
			values, err = r.Resolve(ctx, refs)
		}
	}
	switch {
	case ctx.Err() != nil:
		setContextResult(ctx, &result)
	case err != nil:
		result.Status, result.Message = "failed", "could not resolve job secret references"
	default:
		execute(ctx, j, r.Home, values, &result)
	}
	result.End = time.Now().UTC()
	result.Duration = result.End.Sub(start)
	return result
}

func (r Runner) run(ctx context.Context, j Job, a attempt) (Run, error) {
	result := r.process(ctx, j, a.start)
	secondFailure, err := r.finish(j, result)
	a.release()
	if secondFailure && r.Notice != nil {
		notice := attention.New(attention.KindDone, attention.SourceCron, r.Membership.Self, j.Name,
			result.Start.Format(time.RFC3339Nano), "failed", "Job failed twice consecutively",
			attention.Links{Run: "hev cron show " + j.Name})
		notifyCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		if err := r.Notice(notifyCtx, notice); err != nil && r.Logger != nil {
			r.Logger.Warn("could not emit job attention notice", "job", j.Name)
		}
	}
	return result, err
}

func (r Runner) finish(j Job, result Run) (bool, error) {
	dir := r.dir(j)
	// Persist final state even when cancellation came from daemon shutdown.
	release, err := lockState(context.Background(), dir)
	if err != nil {
		return false, err
	}
	defer release()
	s, err := readState(dir)
	if err != nil {
		return false, err
	}
	s.LastEnd, s.ExitCode, s.Duration = &result.End, result.ExitCode, result.Duration
	secondFailure := false
	switch result.Status {
	case "ok":
		s.ConsecutiveFailures = 0
	case "failed", "timeout":
		s.ConsecutiveFailures++
		secondFailure = s.ConsecutiveFailures == 2
	}
	if err := writeJSON(filepath.Join(dir, "state.json"), s); err != nil {
		return false, err
	}
	return secondFailure, appendRun(dir, result)
}

// Start supervises scheduled jobs until the returned function cancels and waits.
// Each job has a separate scheduler so busy ticks still reach admission.
func (r Runner) Start(ctx context.Context, jobs []Job) func() {
	ctx, cancel := context.WithCancel(ctx)
	var schedulers sync.WaitGroup
	for _, j := range jobs {
		schedulers.Go(func() { r.schedule(ctx, j) })
	}
	return func() {
		cancel()
		schedulers.Wait()
	}
}

func (r Runner) schedule(ctx context.Context, j Job) {
	var runs sync.WaitGroup
	defer runs.Wait()
	for ctx.Err() == nil {
		now := time.Now()
		timer := time.NewTimer(j.Next(now).Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		a, _, err := r.admit(ctx, j)
		if err != nil {
			if ctx.Err() == nil && r.Logger != nil {
				r.Logger.Warn("could not admit scheduled job", "job", j.Name)
			}
			continue
		}
		if a.release != nil {
			runs.Go(func() {
				if _, err := r.run(ctx, j, a); err != nil && r.Logger != nil {
					r.Logger.Warn("could not store scheduled job result", "job", j.Name)
				}
			})
		}
	}
}
