package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExpediterFakeClockStopConditions(t *testing.T) {
	for _, mode := range []string{"all closed", "explicit stop", "lifetime"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("HEV_CONFIG", filepath.Join(home, "absent.toml"))
			t.Setenv("FACTORY_BOARD_URL", "")
			dir := filepath.Join(home, "run")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			m := Meta{Order: Order{Name: "expediter", Every: "1m", MaxLifetime: "3m", Until: "all followed runs merged or abandoned"}, Start: clock}
			if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
				t.Fatal(err)
			}
			rounds := 0
			err := repeatRoundsNotify(dir, m, make(chan os.Signal), func() time.Time { return clock }, func() {
				clock = clock.Add(time.Second)
				if mode == "explicit stop" {
					_ = os.WriteFile(filepath.Join(dir, "stop"), nil, 0600)
				}
			}, func(string) error {
				rounds++
				if mode == "all closed" {
					return WriteJSON(filepath.Join(dir, "until-met.json"), map[string]any{"met": true, "reason": "all snapshotted PRs merged"})
				}
				return nil
			}, func(Meta, Result) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			s := loadSchedule(dir)
			want := map[string]string{"all closed": "until", "explicit stop": "stopped", "lifetime": "max_lifetime"}[mode]
			if s.Reason != want || s.End.IsZero() || !s.Next.IsZero() {
				t.Fatalf("%+v", s)
			}
			if mode == "lifetime" && rounds != 3 {
				t.Fatal(rounds)
			}
		})
	}
}

func TestExpediterOwnerHandoff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "absent.toml"))
	t.Setenv("FACTORY_BOARD_URL", "")
	m := Meta{Order: Order{Name: "example", Host: "worker", Until: "all followed runs merged or abandoned"}}
	if err := os.WriteFile(filepath.Join(home, "order.md"), []byte("snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	old := expediteBoardPost
	t.Cleanup(func() { expediteBoardPost = old })
	posts := 0
	expediteBoardPost = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		posts++
		if !strings.Contains(strings.Join(args, " "), "--board floor") {
			t.Fatal(args)
		}
		return []byte("posted"), nil
	}
	if err := expediteHandoff(home, m, "stopped"); err != nil || posts != 0 {
		t.Fatal(err, posts)
	}
	t.Setenv("FACTORY_BOARD_URL", "http://example.invalid")
	if err := expediteHandoff(home, m, "stopped"); err != nil || posts != 1 {
		t.Fatal(err, posts)
	}
	if err := expediteHandoff(home, m, "stopped"); err != nil || posts != 1 {
		t.Fatal("duplicate handoff", err, posts)
	}
	b, err := os.ReadFile(filepath.Join(home, "handoff.md"))
	if err != nil || !strings.Contains(string(b), "snapshot") {
		t.Fatal(string(b), err)
	}
}
