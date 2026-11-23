package services

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/secrets"
)

const (
	fixtureSecret      = "service-secret-value-alpha-98765"
	fixtureMultiSecret = "multi-secret-first-line\nmulti-secret-second-line"
	fixtureGitPAT      = "ghp_1234567890abcdefghijklmnopqrstuvwxyz"
)

func TestServiceHelperProcess(t *testing.T) {
	i := slices.Index(os.Args, "--service-fixture")
	if i < 0 {
		return
	}
	args := os.Args[i+1:]
	switch args[0] {
	case "serve":
		// Writes PID to marker file and stays alive until SIGTERM/SIGINT.
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		if len(args) > 1 && args[1] != "" {
			_ = os.WriteFile(args[1], []byte(strconv.Itoa(os.Getpid())), 0600)
		}
		if len(args) > 2 && args[2] != "" {
			cwd, _ := os.Getwd()
			fmt.Fprintln(os.Stdout, "literal="+args[2])
			fmt.Fprintln(os.Stdout, "cwd="+cwd)
		}
		<-sig
		if len(args) > 1 && args[1] != "" {
			_ = os.WriteFile(args[1]+".stopped", []byte("graceful"), 0600)
		}
		os.Exit(0)

	case "exit":
		code, _ := strconv.Atoi(args[1])
		os.Exit(code)

	case "scripted":
		// Reads current counter from state file, increments it, and decides behavior:
		// counts 1..4 -> exit(2) immediately (crash loop)
		// count 5 -> stay alive until healthy signal file exists, then exit(0) or wait for signal
		// counts 6..8 -> exit(3) immediately (second crash loop after healthy reset)
		// count >= 9 -> stay alive until SIGTERM
		counterFile := args[1]
		raw, _ := os.ReadFile(counterFile)
		n, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
		n++
		_ = os.WriteFile(counterFile, []byte(strconv.Itoa(n)), 0600)
		switch {
		case n <= 4:
			os.Exit(2)
		case n == 5:
			// Stay healthy until caller creates counterFile+".crash-again", then exit(3).
			for {
				if _, err := os.Stat(counterFile + ".crash-again"); err == nil {
					os.Exit(3)
				}
				time.Sleep(15 * time.Millisecond)
			}
		case n <= 7:
			// Together with n==5's exit after healthy reset? Wait: n==5 was healthy (>HealthyUptime),
			// so n==6, 7, 8 are 3 rapid crashes!
			os.Exit(3)
		case n == 8:
			os.Exit(3)
		default:
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
			<-sig
			os.Exit(0)
		}

	case "on-failure-seq":
		counterFile := args[1]
		raw, _ := os.ReadFile(counterFile)
		n, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
		n++
		_ = os.WriteFile(counterFile, []byte(strconv.Itoa(n)), 0600)
		if n == 1 {
			os.Exit(1)
		}
		os.Exit(0)

	case "stubborn-tree":
		// Ignores SIGTERM/SIGINT and spawns a child that also ignores SIGTERM/SIGINT.
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		marker := args[1]
		child := exec.Command(os.Args[0], "-test.run=^TestServiceHelperProcess$", "--", "--service-fixture", "stubborn-child", marker)
		if err := child.Start(); err != nil {
			os.Exit(5)
		}
		time.Sleep(10 * time.Second)

	case "stubborn-child":
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		marker := args[1]
		_ = os.WriteFile(marker+".ready", []byte(strconv.Itoa(os.Getpid())), 0600)
		time.Sleep(600 * time.Millisecond)
		_ = os.WriteFile(marker+".escaped", []byte("orphan"), 0600)

	case "log-flood":
		cwd, _ := os.Getwd()
		fmt.Fprintln(os.Stdout, "single="+os.Getenv("SVC_SECRET"))
		// Write multiline secret across two separate writes to test boundary scrubbing.
		multi := os.Getenv("SVC_MULTI")
		parts := strings.SplitN(multi, "\n", 2)
		if len(parts) == 2 {
			_, _ = os.Stdout.Write([]byte("split=" + parts[0] + "\n"))
			_, _ = os.Stdout.Write([]byte(parts[1] + "\n"))
		}
		fmt.Fprintln(os.Stderr, "pat="+fixtureGitPAT)
		fmt.Fprintln(os.Stdout, "dir="+cwd)
		// Oversized single line without newline containing a secret.
		_, _ = os.Stdout.Write([]byte(strings.Repeat("x", maxLineBytes+512) + fixtureSecret + "\n"))
		// Write > 1.3 MiB of numbered lines via buffered chunks so ring compaction
		// runs without thousands of tiny unbuffered write syscalls.
		bw := bufio.NewWriterSize(os.Stdout, 32<<10)
		line := strings.Repeat("a", 1000)
		for idx := range 1350 {
			fmt.Fprintf(bw, "row-%05d %s\n", idx, line)
		}
		fmt.Fprintln(bw, "final-marker-line")
		_ = bw.Flush()
		os.Exit(0)
	}
	os.Exit(0)
}

func fixtureService(t *testing.T, name, mode string, args ...string) Service {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s := Service{
		Name: name,
		Cmd:  append([]string{exe, "-test.run=^TestServiceHelperProcess$", "--", "--service-fixture", mode}, args...),
	}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	return s
}

func await(t *testing.T, limit time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.NewTimer(limit)
	tick := time.NewTicker(20 * time.Millisecond)
	defer deadline.Stop()
	defer tick.Stop()
	for !cond() {
		select {
		case <-deadline.C:
			t.Fatal("condition was not reached before deadline")
		case <-tick.C:
		}
	}
}

// Rule 1: Valid [[service]] configs load cleanly with defaults; invalid configs
// (duplicate/bad names, empty cmd, invalid restart mode, negative/excessive
// stop_timeout, >64 services, unknown fields, raw secrets in env_refs) are
// refused without echoing secret values.
func TestServiceConfigRules(t *testing.T) {
	load := func(body string) ([]Service, error) {
		p := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return Load(p)
	}

	if svcs, err := Load(filepath.Join(t.TempDir(), "missing.toml")); err != nil || len(svcs) != 0 {
		t.Fatalf("missing file must load empty: %v %v", svcs, err)
	}

	valid := `
[[service]]
name = "board"
cmd = ["hev", "board", "serve", ""]

[[service]]
name = "intranet-1"
cmd = ["intranet"]
workdir = "~/workspace/intranet"
restart = "on-failure"
stop_timeout = "5m"
placement = ["node-a", "node-b"]
enabled = false
env_refs = { API_TOKEN = "secret://api-token", OP_KEY = "op://vault/item/field", KV_KEY = "azkv://vault/secret" }
`
	svcs, err := load(valid)
	if err != nil || len(svcs) != 2 {
		t.Fatalf("valid config failed: %v", err)
	}
	if svcs[0].RestartMode() != RestartAlways || svcs[0].StopTimeoutDuration() != 15*time.Second || !svcs[0].enabled() {
		t.Fatalf("unexpected defaults: %+v", svcs[0])
	}
	if svcs[1].RestartMode() != RestartOnFailure || svcs[1].StopTimeoutDuration() != 5*time.Minute || svcs[1].enabled() {
		t.Fatalf("unexpected explicit settings: %+v", svcs[1])
	}

	// 64 services must be accepted; 65 must be refused.
	var maxBuilder strings.Builder
	for i := range MaxServices {
		fmt.Fprintf(&maxBuilder, "[[service]]\nname = 'svc-%d'\ncmd = ['bin']\n", i)
	}
	if list, err := load(maxBuilder.String()); err != nil || len(list) != MaxServices {
		t.Fatalf("64 services should succeed: len=%d err=%v", len(list), err)
	}
	fmt.Fprintf(&maxBuilder, "[[service]]\nname = 'svc-extra'\ncmd = ['bin']\n")
	if _, err := load(maxBuilder.String()); err == nil || !strings.Contains(err.Error(), "at most 64 services") {
		t.Fatalf("expected >64 refusal, got %v", err)
	}

	base := "[[service]]\nname='web'\ncmd=['bin']\n"
	const leaked = "privateSecretLeakToken"
	cases := map[string]struct {
		config  string
		message string
	}{
		"duplicate name":   {base + base, "duplicate name"},
		"empty name":       {"[[service]]\ncmd=['bin']\n", "name must match"},
		"leading hyphen":   {"[[service]]\nname='-bad'\ncmd=['bin']\n", "name must match"},
		"uppercase name":   {"[[service]]\nname='BadName'\ncmd=['bin']\n", "name must match"},
		"path traversal":   {"[[service]]\nname='../" + leaked + "'\ncmd=['bin']\n", "name must match"},
		"overlong name":    {"[[service]]\nname='" + strings.Repeat("a", 129) + "'\ncmd=['bin']\n", "name must match"},
		"empty cmd":        {"[[service]]\nname='web'\ncmd=[]\n", "nonempty executable"},
		"blank cmd":        {"[[service]]\nname='web'\ncmd=['   ']\n", "nonempty executable"},
		"bad restart":      {base + "restart='sometimes'\n", "restart must be"},
		"zero timeout":     {base + "stop_timeout='0s'\n", "positive Go duration up to 5m"},
		"negative timeout": {base + "stop_timeout='-5s'\n", "positive Go duration up to 5m"},
		"excess timeout":   {base + "stop_timeout='5m1s'\n", "positive Go duration up to 5m"},
		"bad duration":     {base + "stop_timeout='" + leaked + "'\n", "positive Go duration up to 5m"},
		"dup placement":    {base + "placement=['node-a','node-a']\n", "distinct member names"},
		"blank placement":  {base + "placement=['']\n", "distinct member names"},
		"padded placement": {base + "placement=[' node-a']\n", "distinct member names"},
		"bad env key":      {base + "env_refs={ '1BAD'='secret://tok' }\n", "distinct environment names"},
		"dup env case":     {base + "env_refs={ tok='secret://a', TOK='secret://b' }\n", "distinct environment names"},
		"literal secret":   {base + "env_refs={ TOKEN='" + leaked + "' }\n", "secret://, op://, or azkv://"},
		"unknown field":    {base + "secret='" + leaked + "'\n", "unknown field"},
		"parse error":      {"[[service]\nsecret='" + leaked + "'\n", "invalid service configuration at line"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := load(tc.config)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("want %q, got %v", tc.message, err)
			}
			if strings.Contains(err.Error(), leaked) {
				t.Fatalf("validation error leaked value: %v", err)
			}
		})
	}
}

// Rule 2a: Backoff starts at 2s, doubles on consecutive rapid failures, and caps at 5m.
func TestBackoffScheduleBounds(t *testing.T) {
	want := []struct {
		attempt int
		delay   time.Duration
	}{
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 32 * time.Second},
		{6, 64 * time.Second},
		{7, 128 * time.Second},
		{8, 256 * time.Second},
		{9, 5 * time.Minute},
		{50, 5 * time.Minute},
	}
	for _, tc := range want {
		got := BackoffDuration(tc.attempt, DefaultInitialBackoff, DefaultMaxBackoff)
		if got != tc.delay {
			t.Fatalf("attempt %d: got %v, want %v", tc.attempt, got, tc.delay)
		}
	}
}

// Rule 2b: Supervisor enforces restart modes (never, on-failure, always), passes
// literal argv without shell expansion, expands ~ workdir, emits one deduplicated
// done/failed attention notice on >= 3 rapid exits within 2m, and resets on healthy uptime.
func TestSupervisionRestartModesCrashLoopAndHealthyReset(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	home := t.TempDir()
	subDir := filepath.Join(home, "sub")
	if err := os.MkdirAll(subDir, 0700); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var notices []attention.Notice
	sup := Supervisor{
		Home:           home,
		Membership:     Membership{Self: "test-host"},
		InitialBackoff: 30 * time.Millisecond,
		MaxBackoff:     120 * time.Millisecond,
		CrashWindow:    10 * time.Second,
		HealthyUptime:  150 * time.Millisecond,
		PollInterval:   20 * time.Millisecond,
		Notice: func(_ context.Context, n attention.Notice) error {
			mu.Lock()
			notices = append(notices, n)
			mu.Unlock()
			return nil
		},
	}

	marker := filepath.Join(t.TempDir(), "serve.pid")
	literalArg := "$(echo expanded); *"
	serveSvc := fixtureService(t, "literal-svc", "serve", marker, literalArg)
	serveSvc.Workdir = "~/sub"

	neverFail := fixtureService(t, "never-fail", "exit", "4")
	neverFail.Restart = RestartNever

	onFailCounter := filepath.Join(t.TempDir(), "onfail.count")
	onFail := fixtureService(t, "on-fail", "on-failure-seq", onFailCounter)
	onFail.Restart = RestartOnFailure

	crashCounter := filepath.Join(t.TempDir(), "crash.count")
	crasher := fixtureService(t, "crasher", "scripted", crashCounter)

	ctx := t.Context()
	stop, err := sup.Start(ctx, []Service{serveSvc, neverFail, onFail, crasher})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	// Verify never-fail runs once and settles in failed with 0 restarts.
	await(t, 5*time.Second, func() bool {
		d, err := sup.Inspect(ctx, neverFail, 0)
		return err == nil && d.Status == StatusFailed && d.LastExit != nil && *d.LastExit == 4 && d.Restarts == 0
	})

	// Verify on-failure restarts once after exit 1, then stops on exit 0.
	await(t, 5*time.Second, func() bool {
		d, err := sup.Inspect(ctx, onFail, 0)
		return err == nil && d.Status == StatusStopped && d.LastExit != nil && *d.LastExit == 0 && d.Restarts == 1
	})

	// Verify literal-svc ran directly without shell expansion in ~/sub.
	await(t, 5*time.Second, func() bool {
		logs, err := sup.Logs(serveSvc, 0)
		return err == nil && strings.Contains(logs, "literal="+literalArg) && strings.Contains(logs, "cwd=[REDACTED]")
	})

	// Crasher crashes 4 times rapidly -> reaches healthy run #5.
	// Must have emitted exactly 1 deduplicated crash-loop notice across all 4 crashes,
	// and then reset CrashLoop once HealthyUptime (150ms) elapses on run #5.
	await(t, 8*time.Second, func() bool {
		raw, _ := os.ReadFile(crashCounter)
		n, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
		d, err := sup.Inspect(ctx, crasher, 0)
		return err == nil && n == 5 && d.Status == StatusRunning && !d.State.CrashLoop
	})
	mu.Lock()
	if len(notices) != 1 || notices[0].Event != attention.KindDone || notices[0].Source != attention.SourceService ||
		notices[0].State != StatusFailed || notices[0].Thread != "service:test-host/crasher" {
		t.Fatalf("expected 1 crash-loop notice, got %+v", notices)
	}
	firstNoticeID := notices[0].ID
	mu.Unlock()

	// Trigger a second crash-loop streak after healthy uptime reset -> must emit one new notice.
	if err := os.WriteFile(crashCounter+".crash-again", []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	await(t, 8*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(notices) == 2
	})
	mu.Lock()
	secondNoticeID := notices[1].ID
	mu.Unlock()
	if firstNoticeID == secondNoticeID {
		t.Fatal("new crash-loop streak after healthy uptime should produce a distinct notice ID")
	}

	stop()
	// Graceful shutdown stopped serveSvc via SIGTERM.
	if _, err := os.Stat(marker + ".stopped"); err != nil {
		t.Fatalf("expected graceful SIGTERM shutdown marker: %v", err)
	}
}

// Rule 2c: Stop and daemon shutdown terminate stubborn process trees after
// stop_timeout without leaving orphaned child processes.
func TestProcessTreeKillOnStopAndShutdown(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	home := t.TempDir()
	sup := Supervisor{
		Home:         home,
		Membership:   Membership{Self: "test-host"},
		PollInterval: 20 * time.Millisecond,
	}

	stopMarker := filepath.Join(t.TempDir(), "stop-tree")
	svcStop := fixtureService(t, "tree-stop", "stubborn-tree", stopMarker)
	svcStop.StopTimeout = "150ms"
	svcStop.stopTimeout = 150 * time.Millisecond

	shutMarker := filepath.Join(t.TempDir(), "shut-tree")
	svcShut := fixtureService(t, "tree-shut", "stubborn-tree", shutMarker)
	svcShut.StopTimeout = "150ms"
	svcShut.stopTimeout = 150 * time.Millisecond

	ctx := t.Context()
	stop, err := sup.Start(ctx, []Service{svcStop, svcShut})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	await(t, 5*time.Second, func() bool {
		_, err1 := os.Stat(stopMarker + ".ready")
		_, err2 := os.Stat(shutMarker + ".ready")
		return err1 == nil && err2 == nil
	})

	// Stop svcStop via Control("stop") -> escalates to SIGKILL / Job close after 150ms.
	st, err := sup.Control(ctx, svcStop, "stop")
	if err != nil || st.Status != StatusStopped || st.PID != 0 {
		t.Fatalf("stop failed: state=%+v err=%v", st, err)
	}

	// Shut down entire supervisor -> stops svcShut cleanly as well.
	startShut := time.Now()
	stop()
	if time.Since(startShut) > 3*time.Second {
		t.Fatal("supervisor shutdown did not terminate stubborn tree promptly")
	}

	// Wait past child's 600ms sleep to prove no orphaned grandchild survived either stop.
	time.Sleep(750 * time.Millisecond)
	for _, m := range []string{stopMarker, shutMarker} {
		if _, err := os.Stat(m + ".escaped"); !os.IsNotExist(err) {
			t.Fatalf("orphaned child process survived for %s", filepath.Base(m))
		}
	}
}

// Rule 2d: service.log is capped at <= 1 MiB and scrubs single-line and multi-line
// env_refs secrets, gitleaks credentials, and local paths from disk and CLI views.
func TestRingLogCapAndSecretRedaction(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	home := t.TempDir()
	sup := Supervisor{
		Home:         home,
		Membership:   Membership{Self: "test-host"},
		PollInterval: 20 * time.Millisecond,
		Resolve: func(_ context.Context, refs []secrets.Ref) (map[string]string, error) {
			if len(refs) != 2 {
				t.Fatalf("expected 2 refs, got %d", len(refs))
			}
			return map[string]string{
				"SVC_SECRET": fixtureSecret,
				"SVC_MULTI":  fixtureMultiSecret,
			}, nil
		},
	}

	svc := fixtureService(t, "flood", "log-flood")
	svc.Restart = RestartNever
	svc.EnvRefs = map[string]string{
		"SVC_SECRET": "secret://svc-secret",
		"SVC_MULTI":  "op://vault/item/multi",
	}

	ctx := t.Context()
	stop, err := sup.Start(ctx, []Service{svc})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	await(t, 30*time.Second, func() bool {
		d, err := sup.Inspect(ctx, svc, 5)
		if err == nil && d.Status == StatusFailed {
			t.Fatalf("flood unexpectedly failed: %+v", d)
		}
		return err == nil && d.Status == StatusStopped && d.LastExit != nil && strings.Contains(d.Logs, "final-marker-line")
	})
	stop()

	dir := sup.dir(svc)
	logPath := filepath.Join(dir, "service.log")
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > LogLimit || info.Size() < compactTarget/2 {
		t.Fatalf("service.log size %d outside expected ring bounds (max %d)", info.Size(), LogLimit)
	}

	for _, name := range []string{"service.log", "state.json"} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{fixtureSecret, "multi-secret-first-line", "multi-secret-second-line", fixtureGitPAT, home} {
			if bytes.Contains(raw, []byte(forbidden)) {
				t.Fatalf("%s leaked %q", name, forbidden)
			}
		}
	}

	// Verify line-count tailing on Logs.
	tail3, err := sup.Logs(svc, 3)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(tail3, "\n"), "\n")
	if len(lines) != 3 || lines[2] != "final-marker-line" {
		t.Fatalf("expected 3 tail lines ending with final-marker-line, got %q", lines)
	}

	// Also verify redaction on a small run where early secret lines remain in the ring.
	small := fixtureService(t, "redact-check", "serve", filepath.Join(t.TempDir(), "p"), "hello "+fixtureGitPAT+" "+fixtureSecret)
	small.EnvRefs = map[string]string{"SVC_SECRET": "secret://svc-secret", "SVC_MULTI": "op://vault/item/multi"}
	stop2, err := sup.Start(ctx, []Service{small})
	if err != nil {
		t.Fatal(err)
	}
	await(t, 5*time.Second, func() bool {
		logs, err := sup.Logs(small, 0)
		return err == nil && strings.Contains(logs, "[REDACTED:github-pat#") && strings.Contains(logs, "[REDACTED]")
	})
	stop2()
	rawSmall, err := os.ReadFile(filepath.Join(sup.dir(small), "service.log"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rawSmall, []byte(fixtureSecret)) || bytes.Contains(rawSmall, []byte(fixtureGitPAT)) || bytes.Contains(rawSmall, []byte(home)) {
		t.Fatal("small service.log leaked secret or local path")
	}
}

// Rule 3: In a 2-node loopback SWIM cluster with placement = ["node-a", "node-b"],
// only the HRW winner runs the service; when the winner leaves, the survivor starts
// it automatically; when a higher-ranked member joins, the non-winner stops cleanly;
// nodes outside placement and multi-host configs without SWIM never run it.
func TestServiceSWIMActiveStandbyFailover(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	key := make([]byte, 32)
	startNode := func(name string) *cluster.Node {
		n, err := cluster.Start(cluster.Config{Bind: "127.0.0.1:0", Name: name, KeyRef: "secret://fixture-key"}, key, cluster.LocalFacts(name, false, 0), logger)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = n.Close(false) })
		return n
	}

	a, b := startNode("node-a"), startNode("node-b")
	if err := b.Join(a.Address()); err != nil {
		t.Fatal(err)
	}
	await(t, 5*time.Second, func() bool { return len(a.Members()) == 2 && len(b.Members()) == 2 })

	ctx := t.Context()
	markerDir := t.TempDir()
	makeSvc := func(tag string) (Service, string) {
		m := filepath.Join(markerDir, tag+".pid")
		svc := fixtureService(t, "placed-svc", "serve", m)
		svc.Placement = []string{"node-a", "node-b"}
		return svc, m
	}

	makeSup := func(self string, n *cluster.Node) Supervisor {
		return Supervisor{
			Home:         t.TempDir(),
			PollInterval: 30 * time.Millisecond,
			Membership: Membership{
				Self:    self,
				Enabled: true,
				Members: func(context.Context) []cluster.Member { return n.Members() },
			},
		}
	}

	supA := makeSup("node-a", a)
	supB := makeSup("node-b", b)
	outsider := makeSup("node-c", a)
	noCluster := Supervisor{Home: t.TempDir(), PollInterval: 30 * time.Millisecond, Membership: Membership{Self: "node-a", Enabled: false}}

	svcA, markerA := makeSvc("a")
	svcB, markerB := makeSvc("b")
	svcOut, markerOut := makeSvc("out")
	svcNoCluster, markerNoCluster := makeSvc("nocluster")

	winnerA, _ := supA.Membership.winner(ctx, svcA)
	winnerB, _ := supB.Membership.winner(ctx, svcB)
	if winnerA == "" || winnerA != winnerB {
		t.Fatalf("SWIM members disagree on HRW winner: %q vs %q", winnerA, winnerB)
	}

	// Multi-host placement without an active cluster must fail closed in standby.
	stopNoCluster, err := noCluster.Start(ctx, []Service{svcNoCluster})
	if err != nil {
		t.Fatal(err)
	}
	defer stopNoCluster()
	await(t, 2*time.Second, func() bool {
		d, err := noCluster.Inspect(ctx, svcNoCluster, 0)
		return err == nil && d.Status == StatusStandby && strings.Contains(d.Reason, "requires an active cluster")
	})
	if _, err := os.Stat(markerNoCluster); !os.IsNotExist(err) {
		t.Fatal("multi-host placement started without an active cluster")
	}

	activeSup, standbySup := supA, supB
	activeSvc, standbySvc := svcA, svcB
	activeMarker, standbyMarker := markerA, markerB
	activeNode, survivorName := a, "node-b"
	if winnerA == "node-b" {
		activeSup, standbySup = supB, supA
		activeSvc, standbySvc = svcB, svcA
		activeMarker, standbyMarker = markerB, markerA
		activeNode, survivorName = b, "node-a"
	}

	stopActive, err := activeSup.Start(ctx, []Service{activeSvc})
	if err != nil {
		t.Fatal(err)
	}
	defer stopActive()
	stopStandby, err := standbySup.Start(ctx, []Service{standbySvc})
	if err != nil {
		t.Fatal(err)
	}
	defer stopStandby()
	stopOutsider, err := outsider.Start(ctx, []Service{svcOut})
	if err != nil {
		t.Fatal(err)
	}
	defer stopOutsider()

	// Winner runs the service; standby and outsider stay on standby and never spawn it.
	await(t, 5*time.Second, func() bool {
		da, err1 := activeSup.Inspect(ctx, activeSvc, 0)
		ds, err2 := standbySup.Inspect(ctx, standbySvc, 0)
		do, err3 := outsider.Inspect(ctx, svcOut, 0)
		_, started := os.Stat(activeMarker)
		return err1 == nil && err2 == nil && err3 == nil &&
			da.Status == StatusRunning && da.PID > 0 && started == nil &&
			ds.Status == StatusStandby && ds.PlacementWinner == winnerA &&
			do.Status == StatusStandby && do.PlacementWinner == winnerA
	})
	if _, err := os.Stat(standbyMarker); !os.IsNotExist(err) {
		t.Fatal("standby node started placed service while winner was alive")
	}
	if _, err := os.Stat(markerOut); !os.IsNotExist(err) {
		t.Fatal("node outside placement started placed service")
	}

	// Manual start on standby and outsider must also refuse to run the placed service.
	if _, err := standbySup.Control(ctx, standbySvc, "start"); err == nil || !strings.Contains(err.Error(), "standby") {
		t.Fatalf("expected standby refusal on manual start, got %v", err)
	}
	if _, err := outsider.Control(ctx, svcOut, "start"); err == nil || !strings.Contains(err.Error(), "standby") {
		t.Fatalf("expected outsider refusal on manual start, got %v", err)
	}

	// Stop and leave the active winner -> survivor must take over automatically!
	stopActive()
	if err := activeNode.Leave(); err != nil {
		t.Fatal(err)
	}
	await(t, 5*time.Second, func() bool {
		ds, err := standbySup.Inspect(ctx, standbySvc, 0)
		_, started := os.Stat(standbyMarker)
		return err == nil && ds.Status == StatusRunning && ds.PID > 0 && ds.PlacementWinner == survivorName && started == nil
	})
	if _, err := os.Stat(markerOut); !os.IsNotExist(err) {
		t.Fatal("outsider started placed service after winner left")
	}

	// Now verify step-down when a higher-ranked SWIM member joins!
	c1, c2 := startNode("step-a"), startNode("step-b")
	sup1 := makeSup("step-a", c1)
	probe := Service{Name: "stepdown", Placement: []string{"step-a", "step-b"}}
	// Find which of step-a / step-b has the HIGHER rendezvous score when both are alive.
	_ = c2.Join(c1.Address())
	await(t, 5*time.Second, func() bool { return len(c1.Members()) == 2 && len(c2.Members()) == 2 })
	topWinner, _ := sup1.Membership.winner(ctx, probe)

	// Start fresh isolated nodes where the lower-ranked node starts alone first, then joins the higher-ranked node.
	lowName, highName := "step-a", "step-b"
	if topWinner == "step-a" {
		lowName, highName = "step-b", "step-a"
	}
	lowNode, highNode := startNode(lowName), startNode(highName)
	lowSup := makeSup(lowName, lowNode)
	highSup := makeSup(highName, highNode)
	lowMarker := filepath.Join(markerDir, "low.pid")
	highMarker := filepath.Join(markerDir, "high.pid")
	lowSvc := fixtureService(t, "stepdown", "serve", lowMarker)
	lowSvc.Placement = []string{"step-a", "step-b"}
	highSvc := fixtureService(t, "stepdown", "serve", highMarker)
	highSvc.Placement = []string{"step-a", "step-b"}

	stopLow, err := lowSup.Start(ctx, []Service{lowSvc})
	if err != nil {
		t.Fatal(err)
	}
	defer stopLow()
	// The supervisor records Running at spawn. Wait for the child to install
	// its signal handler before the membership change can send SIGTERM.
	await(t, 5*time.Second, func() bool {
		d, err := lowSup.Inspect(ctx, lowSvc, 0)
		pid, markerErr := os.ReadFile(lowMarker)
		return err == nil && d.Status == StatusRunning && d.PID > 0 &&
			markerErr == nil && string(pid) == strconv.Itoa(d.PID)
	})

	stopHigh, err := highSup.Start(ctx, []Service{highSvc})
	if err != nil {
		t.Fatal(err)
	}
	defer stopHigh()
	if err := lowNode.Join(highNode.Address()); err != nil {
		t.Fatal(err)
	}
	// Lower-ranked node must cleanly stop its child process (writing .stopped) and transition to standby!
	await(t, 5*time.Second, func() bool {
		dl, err1 := lowSup.Inspect(ctx, lowSvc, 0)
		dh, err2 := highSup.Inspect(ctx, highSvc, 0)
		_, stoppedCleanly := os.Stat(lowMarker + ".stopped")
		return err1 == nil && err2 == nil &&
			dl.Status == StatusStandby && dl.PID == 0 && dl.PlacementWinner == highName && stoppedCleanly == nil &&
			dh.Status == StatusRunning && dh.PID > 0
	})
}
