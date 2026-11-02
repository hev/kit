package dispatch

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

// Schedule persists the supervisor's round boundaries for inventory and senders.
type Schedule struct {
	Rounds int       `json:"rounds"`
	Last   time.Time `json:"last_round"`
	Next   time.Time `json:"next_round"`
	End    time.Time `json:"end,omitempty"`
	Reason string    `json:"reason,omitempty"`
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
	if m.Every == "" {
		return superviseRound(name)
	}
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, terminationSignals()...)
	defer signal.Stop(signals)
	return repeatRounds(dir, m, signals, time.Now, func() { time.Sleep(100 * time.Millisecond) }, superviseRound)
}

func repeatRounds(dir string, m Meta, signals <-chan os.Signal, now func() time.Time, wait func(), round func(string) error) error {
	name := m.Name
	s := Schedule{}
	var completed time.Time
	finish := func(reason string) error {
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
		return WriteJSON(filepath.Join(dir, "result.json"), r)
	}
	for {
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
		if !now().Before(m.Start.Add(lifetime)) {
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
			_ = os.Rename(filepath.Join(dir, "log"), filepath.Join(dir, fmt.Sprintf("round-%d.log", s.Rounds-1)))
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
		s.Next = nextRound(s.Last, completed, every)
		if err := WriteJSON(filepath.Join(dir, "schedule.json"), s); err != nil {
			return err
		}
	}
}
