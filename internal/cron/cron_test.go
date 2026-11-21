package cron

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/secrets"
)

func TestJobConfigRules(t *testing.T) {
	load := func(body string) ([]Job, error) {
		p := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return Load(p)
	}
	base := "[[cron]]\nname='index'\ncmd=['bin','']\n"
	valid := base + "every='10s'\n" +
		"[[cron]]\nname='daily'\nat='09:30'\ntz='America/New_York'\ncmd=['bin']\nenv_refs={ TOKEN='op://vault/item/field' }\n"
	jobs, err := load(valid)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 3, 7, 15, 0, 2, 0, time.UTC)
	if next := jobs[0].Next(start); !next.Equal(start.Add(8 * time.Second)) {
		t.Fatalf("interval tick = %v", next)
	}
	if next := jobs[1].Next(start); !next.Equal(time.Date(2026, 3, 8, 13, 30, 0, 0, time.UTC)) {
		t.Fatalf("wall-clock tick = %v", next)
	}
	for name, tc := range map[string]struct{ config, message string }{
		"duplicate": {base + "every='10s'\n" + base + "every='1m'", "duplicate name"},
		"bad name":  {strings.Replace(base, "'index'", "'../private'", 1) + "every='10s'", "name must match"},
		"empty":     {strings.Replace(base, "['bin','']", "[]", 1) + "every='10s'", "nonempty executable"},
		"blank":     {strings.Replace(base, "['bin','']", "['  ']", 1) + "every='10s'", "nonempty executable"},
		"both":      {base + "every='10s'\nat='00:00'", "exactly one"},
		"neither":   {base, "exactly one"},
		"short":     {base + "every='9s'", "at least 10s"},
		"negative":  {base + "every='10s'\ntimeout='-1s'", "positive Go duration"},
		"too long":  {base + "every='10s'\ntimeout='25h'", "up to 24h"},
		"bad clock": {base + "at='24:00'", "HH:MM"},
		"bad zone":  {base + "at='10:00'\ntz='Nowhere/Private'", "IANA time zone"},
		"unknown":   {base + "every='10s'\npassphrase='private value'", "unknown field"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := load(tc.config)
			if err == nil || !strings.Contains(err.Error(), tc.message) ||
				strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "Private") {
				t.Fatalf("unsafe or unclear validation error: %v", err)
			}
		})
	}
}

func TestJobWallClockDST(t *testing.T) {
	j := Job{Name: "daily", At: "02:30", TZ: "America/New_York", Cmd: []string{"bin"}}
	if err := j.validate(); err != nil {
		t.Fatal(err)
	}
	utc := func(month time.Month, day, h, m int) time.Time {
		return time.Date(2026, month, day, h, m, 0, 0, time.UTC)
	}
	if next := j.Next(utc(time.March, 8, 0, 0)); !next.Equal(utc(time.March, 9, 6, 30)) {
		t.Fatalf("gap tick = %v", next)
	}
	j.At = "01:30"
	if err := j.validate(); err != nil {
		t.Fatal(err)
	}
	first := utc(time.November, 1, 5, 30)
	if next := j.Next(first.Add(-time.Minute)); !next.Equal(first) {
		t.Fatalf("first repeat tick = %v", next)
	}
	if next := j.Next(first); !next.Equal(utc(time.November, 2, 6, 30)) {
		t.Fatalf("repeat must only run once, next tick = %v", next)
	}
}

const fixtureSecret = "fixture has spaces and a\nsecond line"
const fixtureGit = "ghp_1234567890abcdefghijklmnopqrstuvwxyz"

func TestJobHelperProcess(t *testing.T) {
	i := slices.Index(os.Args, "--job-fixture")
	if i < 0 {
		return
	}
	args := os.Args[i+1:]
	switch args[0] {
	case "output":
		home, _ := os.Getwd()
		fmt.Fprintln(os.Stdout, os.Getenv("JOB_VALUE"))
		fmt.Fprintln(os.Stderr, fixtureGit)
		fmt.Fprintln(os.Stdout, home)
	case "once":
		fmt.Fprint(os.Stdout, args[1])
	case "large":
		fmt.Fprint(os.Stdout, strings.Repeat("a", outputLimit+1)+fixtureSecret)
	case "fail":
		os.Exit(3)
	case "tree":
		child := exec.Command(os.Args[0], "-test.run=^TestJobHelperProcess$", "--", "--job-fixture", "child", args[1])
		if err := child.Start(); err != nil {
			os.Exit(5)
		}
		time.Sleep(8 * time.Second)
	case "child":
		_ = os.WriteFile(args[1]+".ready", nil, 0600)
		time.Sleep(3 * time.Second)
		_ = os.WriteFile(args[1], nil, 0600)
	}
	os.Exit(0)
}

func fixtureJob(t *testing.T, name, mode string, args ...string) Job {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	j := Job{Name: name, Every: "10s", Cmd: append([]string{exe, "-test.run=^TestJobHelperProcess$", "--", "--job-fixture", mode}, args...)}
	if err := j.validate(); err != nil {
		t.Fatal(err)
	}
	return j
}

func TestJobCaptureHistoryAndNotices(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	ctx := t.Context()
	var notices []attention.Notice
	r := Runner{Home: t.TempDir(), Membership: Membership{Self: "test"},
		Resolve: func(_ context.Context, refs []secrets.Ref) (map[string]string, error) {
			if len(refs) != 1 || refs[0].Name != "JOB_VALUE" || refs[0].TTL <= 0 {
				t.Fatal("expected a cacheable environment reference")
			}
			return map[string]string{"JOB_VALUE": fixtureSecret}, nil
		},
		Notice: func(_ context.Context, n attention.Notice) error {
			notices = append(notices, n)
			return nil
		},
	}
	j := fixtureJob(t, "test", "output")
	j.EnvRefs = map[string]string{"JOB_VALUE": "op://vault/item/field"}
	run := func(j Job, status string) Run {
		t.Helper()
		got, err := r.Run(ctx, j)
		if err != nil || got.Status != status {
			t.Fatalf("expected %s, got status %s, error %v", status, got.Status, err)
		}
		return got
	}
	res := run(j, "ok")
	if strings.Contains(res.Output, fixtureSecret) || strings.Contains(res.Output, fixtureGit) ||
		strings.Contains(res.Output, r.Home) || !strings.Contains(res.Output, "[REDACTED]") ||
		!strings.Contains(res.Output, "[REDACTED:github-pat#") {
		t.Fatal("output must scrub resolved values, known paths and gitleaks matches")
	}
	for _, file := range []string{"state.json", "runs/00000000000000000001.json"} {
		data, err := os.ReadFile(filepath.Join(r.dir(j), filepath.FromSlash(file)))
		if err != nil || bytes.Contains(data, []byte(fixtureGit)) || bytes.Contains(data, []byte("fixture has spaces")) ||
			bytes.Contains(data, []byte(r.Home)) {
			t.Fatalf("unexpected stored output in %s", file)
		}
	}
	if literal := "$(echo expanded); *"; run(fixtureJob(t, "literal", "once", literal), "ok").Output != literal {
		t.Fatal("job arguments must pass through without shell expansion")
	}
	res = run(fixtureJob(t, "test", "large"), "ok")
	if res.Output != "" || res.Message == "" {
		t.Fatal("oversize output must be omitted with an explanation")
	}
	for _, want := range []string{"failed", "failed", "failed", "ok", "failed", "failed"} {
		mode := "fail"
		if want == "ok" {
			mode = "once"
		}
		run(fixtureJob(t, "test", mode, "recovered"), want)
	}
	if len(notices) != 2 || notices[0].Event != "done" || notices[0].State != "failed" ||
		notices[0].Thread != "cron:test/test" || notices[0].ID == notices[1].ID {
		t.Fatal("one deduplicated notice is required on the second failure in each streak")
	}
	for i := 0; i < 25; i++ {
		j := fixtureJob(t, "test", "once", fmt.Sprint(i))
		run(j, "ok")
	}
	entries, err := os.ReadDir(filepath.Join(r.dir(j), "runs"))
	if err != nil || len(entries) != historyLimit {
		t.Fatalf("want %d history files, got %d, err %v", historyLimit, len(entries), err)
	}
	d, err := r.Inspect(ctx, j, true)
	if err != nil || len(d.Runs) != historyLimit || d.Runs[0].Output != "24" || d.Runs[19].Output != "5" ||
		d.State.ConsecutiveFailures != 0 || d.State.ExitCode == nil || *d.State.ExitCode != 0 {
		t.Fatal("state and retained runs must describe the newest successful executions")
	}
}

func await(t *testing.T, limit time.Duration, f func() bool) {
	t.Helper()
	timer := time.NewTimer(limit)
	tick := time.NewTicker(20 * time.Millisecond)
	defer timer.Stop()
	defer tick.Stop()
	for !f() {
		select {
		case <-timer.C:
			t.Fatal("condition was not reached before deadline")
		case <-tick.C:
		}
	}
}

func TestJobTimeoutAndNonOverlap(t *testing.T) {
	r := Runner{Home: t.TempDir()}
	marker := filepath.Join(r.Home, "escaped")
	j := fixtureJob(t, "timeout", "tree", marker)
	j.timeout = 2 * time.Second
	ctx := t.Context()
	type outcome struct {
		result Run
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := r.Run(ctx, j)
		done <- outcome{result, err}
	}()
	await(t, time.Second, func() bool {
		_, err := os.Stat(marker + ".ready")
		return err == nil
	})
	// A different Runner, as used by a CLI process, shares the same native lock.
	other := Runner{Home: r.Home}
	busy, err := other.Run(ctx, j)
	if err != nil || busy.Status != "skipped_busy" {
		t.Fatalf("overlap status=%s error=%v", busy.Status, err)
	}
	select {
	case got := <-done:
		if got.err != nil || got.result.Status != "timeout" || got.result.Duration > 3*time.Second {
			t.Fatalf("timeout status=%s duration=%v error=%v", got.result.Status, got.result.Duration, got.err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("timed-out process did not return")
	}
	if err := r.Pause(ctx, j, true); err != nil {
		t.Fatal(err)
	}
	if paused, err := other.Run(ctx, j); err != nil || paused.Status != "skipped_paused" {
		t.Fatalf("persistent pause status=%s error=%v", paused.Status, err)
	}
	if err := other.Pause(ctx, j, false); err != nil {
		t.Fatal(err)
	}
	j.cooldown = time.Hour
	if cooled, err := other.Run(ctx, j); err != nil || cooled.Status != "skipped_cooldown" {
		t.Fatalf("persistent cooldown status=%s error=%v", cooled.Status, err)
	}
	time.Sleep(2 * time.Second)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("descendant survived the process timeout")
	}
	d, err := r.Inspect(ctx, j, true)
	if err != nil || len(d.Runs) != 2 || d.Runs[0].Status != "timeout" || d.Runs[1].Status != "skipped_busy" {
		t.Fatal("history must keep the timeout and the busy tick")
	}
}

func TestCronSWIMFailover(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	key := make([]byte, 32)
	node := func(name string) *cluster.Node {
		n, err := cluster.Start(cluster.Config{Bind: "127.0.0.1:0", Name: name, KeyRef: "secret://fixture/key"}, key, cluster.LocalFacts(name, false, 0), logger)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = n.Close(false) })
		return n
	}
	a, b := node("node-a"), node("node-b")
	if err := b.Join(a.Address()); err != nil {
		t.Fatal(err)
	}
	j := fixtureJob(t, "placed", "once", "ran")
	j.Placement = []string{"node-a", "node-b"}
	ctx := t.Context()
	makeRunner := func(name string, node *cluster.Node) Runner {
		return Runner{Home: t.TempDir(), Membership: Membership{Self: name, Enabled: true,
			Members: func(context.Context) []cluster.Member { return node.Members() },
			Reserve: func(_ context.Context, key string, until time.Time) (bool, error) {
				return node.ReserveCronCooldown(key, until)
			},
		}}
	}
	ar, br, outsider := makeRunner("node-a", a), makeRunner("node-b", b), makeRunner("other", a)
	await(t, 5*time.Second, func() bool { return len(a.Members()) == 2 && len(b.Members()) == 2 })
	winner, _ := ar.Membership.winner(ctx, j)
	otherWinner, _ := br.Membership.winner(ctx, j)
	if winner == "" || winner != otherWinner {
		t.Fatalf("members disagree: %q and %q", winner, otherWinner)
	}
	active, standby, activeNode := ar, br, a
	if winner == "node-b" {
		active, standby, activeNode = br, ar, b
	}
	noCluster := Runner{Home: t.TempDir(), Membership: Membership{Self: winner}}
	if res, err := noCluster.Run(ctx, j); err != nil || res.Status != "skipped_standby" ||
		!strings.Contains(res.Message, "requires an active cluster") {
		t.Fatal("multi-host placement must fail closed without SWIM")
	}

	stopActive := active.Start(ctx, []Job{j})
	defer stopActive()
	defer standby.Start(ctx, []Job{j})()
	defer outsider.Start(ctx, []Job{j})()
	runs := func(r Runner, j Job) int {
		d, err := r.Inspect(ctx, j, true)
		if err != nil {
			t.Fatal(err)
		}
		return len(d.Runs)
	}
	await(t, 12*time.Second, func() bool { return runs(active, j) > 0 })
	await(t, 2*time.Second, func() bool {
		stand, err := standby.Inspect(ctx, j, false)
		return err == nil && stand.State.LastPlacementWinner == winner
	})
	if runs(standby, j) != 0 || runs(outsider, j) != 0 {
		t.Fatal("only the placement winner may run with both members alive")
	}
	cooled := j
	cooled.cooldown = time.Minute
	for i := range 32 {
		cooled.Name = fmt.Sprintf("placed-cooldown-%d", i)
		if w, _ := active.Membership.winner(ctx, cooled); w == winner {
			break
		}
	}
	if res, err := active.Run(ctx, cooled); err != nil || res.Status != "ok" {
		t.Fatalf("winner could not reserve cluster cooldown: %s, %v", res.Status, err)
	}
	// Wait for the real SWIM delivery before checking the survivor's cooldown.
	await(t, 5*time.Second, func() bool {
		lookup := cooled
		lookup.cooldown = time.Nanosecond
		cooling, err := standby.Membership.cooling(ctx, lookup, time.Now().Add(-time.Second))
		return err == nil && cooling
	})
	stopActive()
	if err := activeNode.Leave(); err != nil {
		t.Fatal(err)
	}
	await(t, 5*time.Second, func() bool {
		_, reason := standby.Membership.winner(ctx, j)
		return reason == ""
	})
	if res, err := standby.Run(ctx, cooled); err != nil || res.Status != "skipped_cooldown" {
		t.Fatalf("new winner ignored gossiped cooldown: %s, %v", res.Status, err)
	}
	await(t, 12*time.Second, func() bool { return runs(standby, j) > 0 })
	if runs(outsider, j) != 0 {
		t.Fatal("a node outside placement must stay ineligible after leave")
	}
}

func TestJobSchedulerCancelsRunningTreeOnShutdown(t *testing.T) {
	r := Runner{Home: t.TempDir()}
	marker := filepath.Join(r.Home, "shutdown-descendant")
	j := fixtureJob(t, "shutdown", "tree", marker)
	// Shorten only the scheduler's test clock to observe a busy tick.
	j.interval = 250 * time.Millisecond
	stop := r.Start(t.Context(), []Job{j})
	defer stop()
	await(t, 4*time.Second, func() bool {
		d, err := r.Inspect(t.Context(), j, true)
		if err != nil {
			t.Fatal(err)
		}
		_, ready := os.Stat(marker + ".ready")
		return ready == nil && len(d.Runs) > 0 && d.Runs[0].Status == "skipped_busy"
	})
	start := time.Now()
	stop()
	if time.Since(start) > 2*time.Second {
		t.Fatal("shutdown did not cancel and wait promptly")
	}
	d, err := r.Inspect(t.Context(), j, true)
	if err != nil || d.Status == "running" || len(d.Runs) == 0 || d.Runs[0].Status != "canceled" {
		t.Fatal("shutdown must save the cancellation and release the execution lock")
	}
	time.Sleep(3 * time.Second)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("a descendant survived scheduler shutdown")
	}
}
