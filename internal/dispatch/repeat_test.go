package dispatch

import (
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
	for _, mode := range []string{"until", "lifetime", "stop", "amend-inbox", "overrun"} {
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
			if err := repeatRounds(dir, m, make(chan os.Signal), func() time.Time { return clock }, wait, round); err != nil {
				t.Fatal(err)
			}
			s := loadSchedule(dir)
			want := map[string]int{"until": 1, "lifetime": 3, "stop": 1, "amend-inbox": 2, "overrun": 2}[mode]
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
