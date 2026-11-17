package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFollowOrchestratorFakeClockStopConditions(t *testing.T) {
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
			m := Meta{Order: Order{Name: "follow-orchestrator", Every: "1m", MaxLifetime: "3m", Until: "all followed runs merged or abandoned"}, Start: clock}
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

func TestFollowOrchestratorOwnerHandoff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "absent.toml"))
	t.Setenv("FACTORY_BOARD_URL", "")
	m := Meta{Order: Order{Name: "example", Host: "worker", Until: "all followed runs merged or abandoned"}}
	if err := os.WriteFile(filepath.Join(home, "order.md"), []byte("snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	old := followBoardPost
	t.Cleanup(func() { followBoardPost = old })
	posts := 0
	followBoardPost = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		posts++
		if !strings.Contains(strings.Join(args, " "), "--board floor") {
			t.Fatal(args)
		}
		return []byte("posted"), nil
	}
	if err := followHandoff(home, m, "stopped"); err != nil || posts != 0 {
		t.Fatal(err, posts)
	}
	t.Setenv("FACTORY_BOARD_URL", "http://example.invalid")
	if err := followHandoff(home, m, "stopped"); err != nil || posts != 1 {
		t.Fatal(err, posts)
	}
	if err := followHandoff(home, m, "stopped"); err != nil || posts != 1 {
		t.Fatal("duplicate handoff", err, posts)
	}
	b, err := os.ReadFile(filepath.Join(home, "handoff.md"))
	if err != nil || !strings.Contains(string(b), "snapshot") {
		t.Fatal(string(b), err)
	}
}

func TestFollowOrchestratorHandoffBoundsBoardBody(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "absent.toml"))
	t.Setenv("FACTORY_BOARD_URL", "https://board.example")
	if e := os.WriteFile(filepath.Join(home, "order.md"), []byte(strings.Repeat("snapshot", 2000)), 0600); e != nil {
		t.Fatal(e)
	}
	old := followBoardPost
	t.Cleanup(func() { followBoardPost = old })
	followBoardPost = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		body, e := os.ReadFile(args[len(args)-1])
		if e != nil || len(body) > 4096 || !strings.Contains(string(body), "full snapshot and notes") {
			t.Fatal(len(body), e)
		}
		return []byte("thread 123"), nil
	}
	if e := followHandoff(home, Meta{Order: Order{Name: "test", Until: "all followed runs merged or abandoned"}}, "stopped"); e != nil {
		t.Fatal(e)
	}
	full, e := os.ReadFile(filepath.Join(home, "handoff.md"))
	if e != nil || len(full) < 16000 {
		t.Fatal(len(full), e)
	}
}

// Handoffs trim the snapshot out of both the current and the v0.7 order
// marker, so stopping an orchestrator an older build started still posts it.
func TestFollowHandoffReadsLegacyPlanMarker(t *testing.T) {
	for _, marker := range []string{FollowPlanMarker, LegacyFollowPlanMarker} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("HEV_CONFIG", filepath.Join(home, "absent.toml"))
		t.Setenv("FACTORY_BOARD_URL", "https://board.example")
		order := "---\nname: x\n---\n" + strings.Repeat("preamble ", 500) + marker + `{"name":"expedite-x"}` + "\n-->\n"
		if e := os.WriteFile(filepath.Join(home, "order.md"), []byte(order), 0600); e != nil {
			t.Fatal(e)
		}
		old := followBoardPost
		t.Cleanup(func() { followBoardPost = old })
		var body, args string
		followBoardPost = func(_ context.Context, _ string, a ...string) ([]byte, error) {
			b, _ := os.ReadFile(a[len(a)-1])
			body, args = string(b), strings.Join(a, " ")
			return []byte("ok"), nil
		}
		if e := followHandoff(home, Meta{Order: Order{Name: "expedite-x", Until: "all followed runs merged or abandoned"}}, "stopped"); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(body, "preamble") || !strings.HasSuffix(strings.TrimSpace(body), `{"name":"expedite-x"}`) || !strings.Contains(args, "--tags orchestrator,follow,handoff") {
			t.Fatal(marker, body, args)
		}
	}
	if raw, ok := FollowPlanJSON("no marker"); ok || raw != "" {
		t.Fatal(raw, ok)
	}
	if !IsFollowName("expedite-20261117-180405") || !IsFollowName("follow-20261117-180405") || IsFollowName("release-train") {
		t.Fatal("follow names")
	}
}
