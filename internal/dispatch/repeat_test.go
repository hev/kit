package dispatch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRepeatCadence(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ elapsed, want time.Duration }{{0, time.Minute}, {30 * time.Second, time.Minute}, {time.Minute, 2 * time.Minute}, {150 * time.Second, 3 * time.Minute}} {
		if got := nextRound(start, start.Add(tc.elapsed), time.Minute); !got.Equal(start.Add(tc.want)) {
			t.Fatalf("%s: %s", tc.elapsed, got)
		}
	}
}
func TestRepeatOrder(t *testing.T) {
	base := "---\nname: repeat\nrepo: sample\nevery: 1m\nuntil: three rounds\n---\n## Acceptance\n- done\n"
	o, err := Parse(base)
	if err != nil || o.MaxLifetime != "24h" {
		t.Fatalf("%+v %v", o, err)
	}
	for _, value := range []string{"0s", "-1m", "bad"} {
		if _, err := Parse(strings.Replace(base, "every: 1m", "every: "+value, 1)); err == nil {
			t.Fatal(value)
		}
	}
}

func TestRepeatLifecycleFakeClock(t *testing.T) {
	for _, mode := range []string{"until", "lifetime", "stop", "amend-inbox", "overrun", "repeat removed"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			dir := filepath.Join(Root(), "repeat")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			m := Meta{Start: clock, Order: Order{Name: "repeat", Every: "1m", MaxLifetime: "3m", Until: "done"}}
			if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
				t.Fatal(err)
			}
			count := 0
			active := false
			sent := false
			notifications := 0
			round := func(string) error {
				if active {
					t.Fatal("overlap")
				}
				active = true
				defer func() { active = false }()
				count++
				if mode == "overrun" {
					clock = clock.Add(90 * time.Second)
				}
				if mode == "amend-inbox" && count == 2 {
					b, _ := os.ReadFile(filepath.Join(dir, "inbox.md"))
					if !strings.Contains(string(b), "between rounds") || m.Until != "revised" {
						t.Fatalf("lost amendment/inbox: %s %+v", b, m)
					}
				}
				if mode == "until" || mode == "amend-inbox" && count == 2 {
					return WriteJSON(filepath.Join(dir, "until-met.json"), map[string]any{"met": true, "reason": "done"})
				}
				return WriteJSON(filepath.Join(dir, "result.json"), Result{})
			}
			wait := func() {
				if notifications != 0 {
					t.Fatal("end notification while waiting")
				}
				if mode == "repeat removed" {
					m.Every = ""
					_ = WriteJSON(filepath.Join(dir, "meta.json"), m)
				}
				if mode == "stop" {
					_ = os.WriteFile(filepath.Join(dir, "stop"), nil, 0600)
				}
				if mode == "amend-inbox" && !sent {
					got := queuedRound(dir)
					if got != "queued for next round at 00:01" {
						t.Fatal(got)
					}
					if err := appendInbox(dir, "between rounds"); err != nil {
						t.Fatal(err)
					}
					m.Until = "revised"
					_ = WriteJSON(filepath.Join(dir, "meta.json"), m)
					sent = true
				}
				clock = clock.Add(time.Second)
			}
			if err := repeatRoundsNotify(dir, m, make(chan os.Signal), func() time.Time { return clock }, wait, round, func(_ Meta, r Result) error {
				notifications++
				wantReason := map[string]string{"lifetime": "max_lifetime", "stop": "stopped", "amend-inbox": "until", "overrun": "max_lifetime"}[mode]
				if wantReason == "" {
					wantReason = mode
				}
				if r.RepeatReason != wantReason {
					t.Fatalf("reason: %q want %q", r.RepeatReason, wantReason)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if notifications != 1 {
				t.Fatalf("notifications = %d", notifications)
			}
			s := loadSchedule(dir)
			want := map[string]int{"until": 1, "lifetime": 3, "stop": 1, "amend-inbox": 2, "overrun": 2, "repeat removed": 1}[mode]
			if count != want || s.Rounds != want || s.End.IsZero() || !s.Next.IsZero() {
				t.Fatalf("count %d schedule %+v", count, s)
			}
		})
	}
}

func TestRepeatWaitingAmendAndSend(t *testing.T) {
	dir, m := amendFixture(t, true)
	m.Every = "1m"
	m.MaxLifetime = "24h"
	m.Until = "done"
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := WriteJSON(filepath.Join(dir, "schedule.json"), Schedule{Rounds: 1, Last: clock, Next: clock.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"say", "peer-send"} {
		r, err := Handle(Request{Action: action, Name: "example", Note: "between rounds"})
		if err != nil {
			t.Fatal(err)
		}
		if r.Message != "queued for next round at 00:01" {
			t.Fatalf("%s: %+v", action, r)
		}
		if action == "peer-send" && r.DeliveryStatus != "queued for next round" {
			t.Fatal(r)
		}
	}
	revised := strings.Replace(good, "repo: kit", "repo: kit\nevery: 2m\nuntil: revised", 1)
	if _, err := Handle(Request{Action: "amend", Name: "example", Order: revised}); err != nil {
		t.Fatal(err)
	}
	if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
		t.Fatal(err)
	}
	if m.Every != "2m" || m.Until != "revised" || !supervisorLive(dir) {
		t.Fatalf("%+v", m)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "inbox.md"))
	if !strings.Contains(string(b), "between rounds") || !strings.Contains(string(b), "Order amended") {
		t.Fatal(string(b))
	}
}

func TestRepeatRestartKeepsRoundHistory(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := start
	m := Meta{Start: start, Order: Order{Name: "repeat", Every: "1m", MaxLifetime: "3m"}}
	save := func() {
		t.Helper()
		if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
			t.Fatal(err)
		}
	}
	save()
	interrupted := errors.New("supervisor interrupted")
	count := 0
	round := func(string) error {
		count++
		if err := os.WriteFile(filepath.Join(dir, "log"), []byte(fmt.Sprint(count)), 0600); err != nil {
			return err
		}
		return WriteJSON(filepath.Join(dir, "result.json"), Result{})
	}
	now := func() time.Time { return clock }
	noNotify := func(Meta, Result) error { return nil }
	// Interrupt after two completed rounds without ending the persisted run.
	err := repeatRoundsNotify(dir, m, make(chan os.Signal), now, func() {
		clock = clock.Add(time.Minute)
	}, func(name string) error {
		if count == 2 {
			return interrupted
		}
		return round(name)
	}, noNotify)
	if !errors.Is(err, interrupted) {
		t.Fatal(err)
	}
	// A relaunched attempt must use the original deadline even if metadata changes.
	m.Start = clock
	save()
	if err := repeatRoundsNotify(dir, m, make(chan os.Signal), now, func() { clock = clock.Add(time.Minute) }, round, noNotify); err != nil {
		t.Fatal(err)
	}
	s := loadSchedule(dir)
	if s.Rounds != 4 || count != 3 || s.Reason != "max_lifetime" || !s.End.Equal(start.Add(3*time.Minute)) {
		t.Fatalf("count=%d schedule=%+v", count, s)
	}
	b, err := os.ReadFile(filepath.Join(dir, "round-1.log"))
	if err != nil || string(b) != "1" {
		t.Fatalf("lost first log: %q %v", b, err)
	}
	// Explicit collisions preserve both the original and the newly archived evidence.
	if err := os.WriteFile(filepath.Join(dir, "log"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := archiveRoundLog(dir, 1); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(dir, "round-1.log"))
	extra, _ := filepath.Glob(filepath.Join(dir, "round-1-restart-*.log"))
	if string(b) != "1" || len(extra) != 1 {
		t.Fatal("archive replaced evidence")
	}
}

func TestRepeatAmendRestartDoesNotNotifyEnd(t *testing.T) {
	dir := t.TempDir()
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := Meta{Start: clock, Order: Order{Name: "repeat", Every: "1m", MaxLifetime: "24h"}}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	err := repeatRoundsNotify(dir, m, make(chan os.Signal), func() time.Time { return clock }, func() {
		if err := os.WriteFile(filepath.Join(dir, "stop"), []byte("amend"), 0600); err != nil {
			t.Fatal(err)
		}
	}, func(string) error { return nil }, func(Meta, Result) error { t.Fatal("amend sent final notification"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	s := loadSchedule(dir)
	if !s.End.IsZero() || s.Rounds != 1 || s.Next.IsZero() {
		t.Fatalf("lost active schedule: %+v", s)
	}
}
