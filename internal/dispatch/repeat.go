package dispatch

import (
	"context"
	"fmt"
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
	defer os.Remove(filepath.Join(dir, "supervisor.json"))
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
