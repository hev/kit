package dispatch

import (
	"context"
	"errors"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

// Schedule persists the supervisor's round boundaries for inventory and senders.
type Schedule struct {
	Start     time.Time `json:"start,omitempty"`
	Completed time.Time `json:"completed,omitempty"`
	Rounds    int       `json:"rounds"`
	Last      time.Time `json:"last_round"`
	Next      time.Time `json:"next_round"`
	End       time.Time `json:"end,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

func loadSchedule(dir string) (s Schedule) {
	_ = readJSON(filepath.Join(dir, "schedule.json"), &s)
	return
}
func queuedRound(dir string) string {
	s := loadSchedule(dir)
	if !s.Next.IsZero() && s.End.IsZero() {
		return "queued for next round at " + s.Next.UTC().Format("15:04")
	}
	return ""
}

// nextRound skips missed ticks; a slow round never creates concurrent work.
func nextRound(last, now time.Time, every time.Duration) time.Time {
	return last.Add((now.Sub(last)/every + 1) * every)
}

func Supervise(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid name")
	}
	dir := filepath.Join(Root(), name)
	var m Meta
	if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
		return err
	}
	if err := writePID(dir); err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(filepath.Join(dir, "supervisor.json"))
		// The exiting supervisor must release its slot before draining.
		_ = SweepQueue()
	}()
	if m.Every == "" && loadSchedule(dir).Rounds == 0 {
		return superviseRound(name)
	}
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, terminationSignals()...)
	defer signal.Stop(signals)
	return repeatRounds(dir, m, signals, time.Now, func() { time.Sleep(100 * time.Millisecond) }, superviseRound)
}

func repeatRounds(dir string, m Meta, signals <-chan os.Signal, now func() time.Time, wait func(), round func(string) error) error {
	return repeatRoundsNotify(dir, m, signals, now, wait, round, func(m Meta, r Result) error {
		cfg, err := Load(m.ConfigPath)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return Notify(ctx, cfg.Notify, m, r)
	})
}

func repeatRoundsNotify(dir string, m Meta, signals <-chan os.Signal, now func() time.Time, wait func(), round func(string) error, notify func(Meta, Result) error) error {
	name := m.Name
	s := loadSchedule(dir)
	if s.Start.IsZero() {
		s.Start = m.Start
	}
	s.End = time.Time{}
	s.Reason = ""
	completed := s.Completed
	if completed.IsZero() {
		completed = s.Last
		if !s.Next.IsZero() {
			completed = s.Next.Add(-time.Nanosecond)
		}
	}

	finish := func(reason string) error {
		// An amend terminates this supervisor, but the repeating run continues.
		if b, _ := os.ReadFile(filepath.Join(dir, "stop")); reason == "stopped" && string(b) == "amend" {
			return nil
		}
		s.Next = time.Time{}
		s.End = now().UTC()
		s.Reason = reason
		if err := WriteJSON(filepath.Join(dir, "schedule.json"), s); err != nil {
			return err
		}
		var r Result
		_ = readJSON(filepath.Join(dir, "result.json"), &r)
		r.End = s.End
		if reason == "stopped" {
			r.Stopped = true
			r.ExitCode = 143
		}
		r.RepeatReason = reason
		if err := WriteJSON(filepath.Join(dir, "result.json"), r); err != nil {
			return err
		}
		if err := expediteHandoff(dir, m, reason); err != nil {
			return setNotifyError(dir, err)
		}
		if err := notify(m, r); err != nil {
			return setNotifyError(dir, err)
		}
		return nil
	}
	for {
		m = Meta{} // Omitted JSON fields must clear values removed by amend.
		if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
			return err
		}
		if m.Every == "" {
			return finish("repeat removed")
		}
		lifetime, err := time.ParseDuration(m.MaxLifetime)
		if err != nil {
			return err
		}
		if !now().Before(s.Start.Add(lifetime)) {
			return finish("max_lifetime")
		}
		if _, err := os.Stat(filepath.Join(dir, "stop")); err == nil {
			return finish("stopped")
		}
		if !s.Next.IsZero() {
			every, err := time.ParseDuration(m.Every)
			if err != nil {
				return err
			}
			next := nextRound(s.Last, completed, every)
			if !next.Equal(s.Next) {
				s.Next = next
				if err := WriteJSON(filepath.Join(dir, "schedule.json"), s); err != nil {
					return err
				}
			}
		}
		if !s.Next.IsZero() && now().Before(s.Next) {
			select {
			case <-signals:
				return finish("stopped")
			default:
				wait()
			}
			continue
		}
		s.Rounds++
		s.Last = now().UTC()
		s.Next = time.Time{}
		if err := WriteJSON(filepath.Join(dir, "schedule.json"), s); err != nil {
			return err
		}
		_ = os.Remove(filepath.Join(dir, "until-met.json"))
		if s.Rounds > 1 {
			if err := archiveRoundLog(dir, s.Rounds-1); err != nil {
				return err
			}
		}
		if err := round(name); err != nil {
			return err
		}
		_ = os.Remove(filepath.Join(dir, "harness.json"))
		var r Result
		_ = readJSON(filepath.Join(dir, "result.json"), &r)
		if r.Stopped {
			return finish("stopped")
		}
		var met struct {
			Met    bool   `json:"met"`
			Reason string `json:"reason"`
		}
		if m.Until != "" && readJSON(filepath.Join(dir, "until-met.json"), &met) == nil && met.Met && met.Reason != "" {
			return finish("until")
		}
		if m.Every == "" {
			return finish("repeat removed")
		}
		every, err := time.ParseDuration(m.Every)
		if err != nil {
			return err
		}
		completed = now()
		s.Completed = completed
		s.Next = nextRound(s.Last, completed, every)
		if err := WriteJSON(filepath.Join(dir, "schedule.json"), s); err != nil {
			return err
		}
	}
}

// archiveRoundLog never replaces evidence, including after an interrupted restart.
func archiveRoundLog(dir string, n int) error {
	src, err := os.Open(filepath.Join(dir, "log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer src.Close()
	path := filepath.Join(dir, fmt.Sprintf("round-%d.log", n))
	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		dst, err = os.CreateTemp(dir, fmt.Sprintf("round-%d-restart-*.log", n))
	}
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Remove(filepath.Join(dir, "log"))
}

// SuperviseOrchestrator uses the repeating-run scheduler's missed-tick policy.
// It is opt-in, serial, and persists every boundary; no daemon is installed.
func SuperviseOrchestrator(ctx context.Context, name string, d OrchestratorDriver) error {
	if parent := currentOrchestratorIdentity(); parent != "" {
		if e := admitOrchestratorChild(parent, name); e != nil {
			return e
		}
	}
	s, err := LoadOrchestrator(name)
	if err != nil {
		return err
	}
	dir := OrchestratorDir(name)
	// Expedite's declared preset is once. Its compatibility adapter retains
	// the legacy bounded repeat cadence from the saved dispatch snapshot.
	repeat := s.Mode == "loop"
	if raw, e := os.ReadFile(filepath.Join(dir, "order.md")); e == nil {
		legacy, e := Parse(string(raw))
		if e != nil {
			return e
		}
		if legacy.Until == "all followed runs merged or abandoned" {
			repeat = true
			s.Every = legacy.Every
			s.MaxLifetime = legacy.MaxLifetime
		}
	}

	if exists(filepath.Join(dir, "scope-owner.json")) {
		return fmt.Errorf("scope-only registration cannot supervise")
	}
	// A short root lock admits exactly one supervisor, including concurrent wakes.
	unlock, err := lockRoot()
	if err != nil {
		return err
	}
	if processRecordLive(filepath.Join(dir, "supervisor.json")) {
		unlock()
		return fmt.Errorf("orchestrator already running")
	}
	if err = writePID(dir); err != nil {
		unlock()
		return err
	}
	unlock()
	defer os.Remove(filepath.Join(dir, "supervisor.json"))
	st, err := orchestratorState(name)
	if err != nil {
		return err
	}
	// The supervisor holds no lock while it delivers; rounds and controls only
	// queue under their locks.
	notice := func(event, key string, to ...string) {
		_ = orchestratorLifecycle(s, st, event, key, to...)
		flushOrchestratorNotices(s, noticeSender(d))
	}
	flushOrchestratorNotices(s, noticeSender(d))
	if st.State == "complete" || st.State == "handed-off" {
		return nil
	}
	persist := func() error {
		line := fmt.Sprintf("%s: %s; next wake: %s; error: %s\n", name, st.State, st.Next.Format(time.RFC3339), st.Error)
		fmt.Print(line)
		if e := WriteJSON(filepath.Join(dir, "state.json"), st); e != nil {
			return e
		}
		f, e := os.OpenFile(filepath.Join(dir, "supervisor.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if e != nil {
			return e
		}
		_, e = f.WriteString(line)
		closeErr := f.Close()
		if e != nil {
			return e
		}
		return closeErr
	}

	if d.Wait == nil {
		watcher, e := fsnotify.NewWatcher()
		if e != nil {
			return e
		}
		defer watcher.Close()
		if e = watcher.Add(dir); e != nil {
			return e
		}
		// Watch an external pause_file so its removal starts a round promptly.
		pausePath := filepath.Clean(sourcePausePath(s))
		if pausePath != "." && filepath.Dir(pausePath) != filepath.Clean(dir) {
			if e = watcher.Add(filepath.Dir(pausePath)); e != nil {
				fmt.Fprintf(os.Stderr, "%s: cannot watch pause_file directory: %v\n", name, e)
			}
		}
		d.Wait = func(ctx context.Context) error {
			deadline := st.Next
			if exists(filepath.Join(dir, "pause")) {
				deadline = time.Time{}
			}
			if deadline.IsZero() {
				deadline = d.Now().Add(24 * time.Hour)
			}
			if s.MaxLifetime != "" {
				life, _ := OrchestratorDuration(s.MaxLifetime)
				if end := st.Start.Add(life); end.Before(deadline) {
					deadline = end
				}
			}
			if end, e := time.Parse("2006-01-02", s.Until); e == nil && end.Before(deadline) {
				deadline = end
			}
			timer := time.NewTimer(max(time.Millisecond, deadline.Sub(d.Now())))
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-timer.C:
					return nil
				case e, ok := <-watcher.Errors:
					if !ok {
						return fmt.Errorf("control watcher closed")
					}
					return e
				case event, ok := <-watcher.Events:
					if !ok {
						return fmt.Errorf("control watcher closed")
					}
					if pausePath != "." && filepath.Clean(event.Name) == pausePath {
						if !exists(pausePath) {
							_ = os.WriteFile(filepath.Join(dir, "wake"), []byte("pause_file removed"), 0600)
						}
						return nil
					}
					if filepath.Dir(filepath.Clean(event.Name)) != filepath.Clean(dir) {
						continue
					}
					switch filepath.Base(event.Name) {
					case "wake", "stop", "pause", "handoff.json":
						return nil
					}
				}
			}
		}
	}
	if repeat && s.Mode == "once" {
		defer func() {
			if st.State != "stopped" && st.State != "complete" && st.State != "handed-off" {
				return
			}
			raw, e := os.ReadFile(filepath.Join(dir, "order.md"))
			if e != nil {
				return
			}
			legacy, e := Parse(string(raw))
			if e != nil {
				return
			}
			if notes, e := os.ReadFile(filepath.Join(dir, "checkpoint.md")); e == nil {
				_ = os.WriteFile(filepath.Join(dir, "round-notes.md"), notes, 0600)
			}
			if e := expediteHandoff(dir, Meta{Order: legacy}, st.State); e != nil {
				st.Error = e.Error()
				_ = persist()
			}
		}()
	}
	if st.Start.IsZero() {
		st.Start = d.Now()
	}
	for {
		if ctx.Err() != nil {
			notice("stopped", "stopped")
			st.State = "stopped"
			st.Error = ctx.Err().Error()
			_ = persist()
			return ctx.Err()
		}
		if exists(filepath.Join(dir, "stop")) {
			notice("stopped", "stopped")
			st.State = "stopped"
			st.Next = time.Time{}
			return persist()
		}
		var h struct {
			To string `json:"to"`
		}
		if readJSON(filepath.Join(dir, "handoff.json"), &h) == nil {
			b, _ := os.ReadFile(filepath.Join(dir, "checkpoint.md"))
			b = append(b, []byte("\n\nHandoff to "+h.To+". This goal is now owned by that session.\n")...)

			st.Rounds++
			round := filepath.Join(dir, "rounds", fmt.Sprintf("%03d", st.Rounds))
			if err = os.MkdirAll(round, 0700); err != nil {
				return err
			}
			if err = WriteJSON(filepath.Join(round, "packet.json"), map[string]any{"orchestrator": name, "handoff_to": h.To}); err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(round, "checkpoint.md"), b, 0600); err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(round, "trace"), []byte("orchestrator="+name+" handoff to "+h.To+"\n"), 0600); err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(dir, "checkpoint.md"), b, 0600); err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(dir, "handoff.md"), b, 0600); err != nil {
				return err
			}
			notice("handoff to "+h.To, "handoff", s.Reply, h.To)
			st.State = "handed-off"
			st.Next = time.Time{}
			return persist()
		}
		flushOrchestratorNotices(s, noticeSender(d))
		now := d.Now()
		expired := false
		if s.MaxLifetime != "" {
			life, _ := OrchestratorDuration(s.MaxLifetime)
			expired = !now.Before(st.Start.Add(life))
		}
		if date, e := time.Parse("2006-01-02", s.Until); e == nil && !now.Before(date) {
			expired = true
		}
		if expired {
			notice("budget exhausted (lifetime/deadline)", "budget-lifetime")
			st.State = "complete"
			st.Next = time.Time{}
			return persist()
		}
		if exists(filepath.Join(dir, "pause")) {
			if st.State != "paused" {
				st.State = "paused"
				if err = persist(); err != nil {
					return err
				}
			}
			if err = d.Wait(ctx); err != nil {
				return err
			}
			continue
		}
		wake := exists(filepath.Join(dir, "wake"))
		if !wake && !st.Next.IsZero() && now.Before(st.Next) {
			if err = d.Wait(ctx); err != nil {
				return err
			}
			continue
		}
		_ = os.Remove(filepath.Join(dir, "wake"))
		timeout := 30 * time.Minute
		if s.RoundTimeout != "" {
			timeout, _ = OrchestratorDuration(s.RoundTimeout)
		}
		roundCtx, cancel := context.WithTimeout(ctx, timeout)
		if s.MaxLifetime != "" {
			life, _ := OrchestratorDuration(s.MaxLifetime)
			cancel()
			roundCtx, cancel = context.WithTimeout(ctx, min(timeout, st.Start.Add(life).Sub(now)))
		}
		err = RunOrchestratorRound(roundCtx, s, &st, d)
		cancel()
		if errors.Is(err, ErrSourceDeferred) {
			err = nil // Deferral keeps state waiting and the cursor in place.
		}
		if exists(filepath.Join(dir, "stop")) {
			notice("stopped", "stopped")
			st.State = "stopped"
			st.Next = time.Time{}
			return persist()
		}
		if err != nil {
			st.ConsecutiveFailures++
			if st.ConsecutiveFailures == 2 {
				notice("round failure or timeout after 2 consecutive failures: "+err.Error(), "failures:"+d.Now().Format(time.RFC3339Nano))
			}
			st.State = "failed"
			st.Error = err.Error()
		}
		if err == nil {
			st.ConsecutiveFailures = 0
		}
		if !repeat || st.State == "complete" {
			// A once round with deferred work stays waiting until woken again.
			if st.State == "waiting" && st.Deferred == "" {
				st.State = "complete"
			}
			if st.State == "complete" {
				notice("goal met (complete)", "complete")
			}
			st.Next = time.Time{}
			if e := persist(); e != nil {
				return e
			}
			return err
		}
		every, _ := OrchestratorDuration(s.Every)
		st.Next = nextRound(now, d.Now(), every)
		if e := persist(); e != nil {
			return e
		}
	}
}
