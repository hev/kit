package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/feed"
	registry "github.com/hev/kit/internal/harness"
	"github.com/hev/kit/internal/hooks"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/peers/compat"
	"github.com/hev/kit/internal/steer"
	"github.com/spf13/cobra"
)

// hookWatchdog bounds a whole hook process past its handler budget. Reading
// stdin, the event log and a wedged handler cannot hold an agent longer.
const hookWatchdog = 3 * time.Second

// hookExit ends the process when the watchdog fires. Tests that run runHook
// inside the test binary replace it so a late hook cannot end the test run.
var hookExit = os.Exit

// isHookInvocation reports whether argv is `hev hook ...`, which main runs
// before building the command tree so a hook does as little as possible.
func isHookInvocation(args []string) bool { return len(args) > 1 && args[1] == "hook" }

// runHook is `hev hook EVENT --harness H`. It always returns 0: any problem
// is logged and answered with "continue" (no output).
func runHook(args []string, in io.Reader, out io.Writer) (code int) {
	home, _ := os.UserHomeDir()
	log := hooks.Log{Dir: filepath.Join(home, ".hev", "hooks")}
	ev := hooks.Event{Schema: hooks.Schema}
	defer func() {
		if p := recover(); p != nil {
			_ = log.Warn(ev, fmt.Sprintf("hev hook panicked: %v; continuing", p))
			code = 0
		}
	}()
	// The kill switch comes before any other work, stdin included.
	if hooks.Disabled(os.Getenv("HEV_HOOKS")) {
		return 0
	}
	event, harness, installedBy := "", "", ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--harness" && i+1 < len(args):
			harness = args[i+1]
			i++
		case strings.HasPrefix(a, "--harness="):
			harness = strings.TrimPrefix(a, "--harness=")
		case a == "--installed-by" && i+1 < len(args):
			installedBy = args[i+1]
			i++
		case a == "-h" || a == "--help":
			fmt.Fprint(out, hookHelp)
			return 0
		case event == "" && !strings.HasPrefix(a, "-"):
			event = a
		}
	}
	ev.Event, ev.Harness = event, harness
	// The deadline starts before any file I/O: a config file or warnings log
	// that blocks (a FIFO, a stalled home filesystem) cannot hold the agent.
	began := time.Now()
	early := ev
	watchdog := armWatchdog(hooks.DefaultTimeout+hookWatchdog, func(msg string) { _ = log.Warn(early, msg) }, hookExit)
	defer watchdog.Stop()
	cfg, cfgWarn := hooks.LoadConfig(layerenv.ConfigPath())
	if !cfg.Enabled {
		return 0
	}
	// A run launched with invocation hooks gets every event from them; the
	// globally installed copy steps aside so nothing is handled twice.
	if installedBy != "" && os.Getenv("HEV_HOOK_INVOCATION") == "1" {
		return 0
	}
	// The configured budget replaces the default one, still counted from
	// the start of the process.
	watchdog.Reset(time.Until(began.Add(cfg.Timeout + hookWatchdog)))
	if cfgWarn != "" {
		_ = log.Warn(ev, cfgWarn)
	}
	raw, err := io.ReadAll(io.LimitReader(in, 16<<20))
	if err != nil {
		_ = log.Warn(ev, "read hook input: "+err.Error())
		return 0
	}
	start := time.Now()
	ev, err = hooks.Normalize(harness, event, raw, start)
	if err != nil {
		_ = log.Warn(ev, err.Error()+"; continuing")
		return 0
	}
	resp := hooks.Run(context.Background(), ev, hookHandlers(&ev, home), cfg.Timeout, func(msg string) { _ = log.Warn(ev, msg) })
	if b := hooks.Render(harness, event, resp); len(b) > 0 {
		_, _ = out.Write(b)
	}
	_ = log.Event(ev, resp, time.Since(start))
	return 0
}

// watchdogLogGrace is how long a firing watchdog waits for its warning to
// be written before it exits anyway.
const watchdogLogGrace = 200 * time.Millisecond

// watchdogGrace starts the wait for a firing watchdog's warning. Tests
// replace it to decide when the grace is over.
var watchdogGrace = func() <-chan time.Time { return time.After(watchdogLogGrace) }

// armWatchdog exits the process with 0 (continue) after d. The warning is
// written on another goroutine and given watchdogLogGrace: a blocked log
// cannot keep the process alive.
func armWatchdog(d time.Duration, warn func(string), exit func(int)) *time.Timer {
	return time.AfterFunc(d, func() {
		logged := make(chan struct{})
		go func() {
			warn("hev hook exceeded its deadline; exiting with continue")
			close(logged)
		}()
		select {
		case <-logged:
		case <-watchdogGrace():
		}
		exit(0)
	})
}

const hookHelp = `Usage: hev hook EVENT --harness claude|codex|copilot

Run kit's hook layer for one harness lifecycle event. The harness passes its
hook JSON on stdin. Events: session-start, prompt-submit, pre-tool, post-tool,
stop, pre-compact. Kit installs these through hev hooks install and wires them
into supported dispatch runs.

Always exits 0. HEV_HOOKS=off or [hooks] enabled = false makes it a no-op.
`

// hookHandlers are the built-in handlers followed by hev-hook-NAME
// executables on PATH. It sets ev.Target when a steering target resolves.
func hookHandlers(ev *hooks.Event, home string) []hooks.Handler {
	var hs []hooks.Handler
	if ev.Event == "prompt-submit" && os.Getenv("PEERS_INBOX_TOKEN") != "" && os.Getenv("PEERS_INBOX_HOOKED") == "" {
		hs = append(hs, hooks.Handler{Name: "peers-inbox", Run: func(ctx context.Context, _ hooks.Event) (hooks.Response, error) {
			text, system, err := compat.InboxContext(ctx)
			return hooks.Response{Context: text, SystemMessage: system}, err
		}})
	}
	cwd := ev.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if store, ok := steer.Resolve(os.Getenv, cwd, home, dispatch.Root(), tmuxSessionName); ok {
		ev.Target = store.Target
		ack := steerAckCommand()
		snapshot := *ev
		hs = append(hs, hooks.Handler{Name: "steer", Run: func(_ context.Context, _ hooks.Event) (hooks.Response, error) {
			return store.Handle(snapshot, ack, time.Now())
		}})
	}
	hs = append(hs, hooks.Handler{Name: "feed", Run: func(_ context.Context, ev hooks.Event) (hooks.Response, error) {
		// Spool only: the sender summarizes and posts outside the budget.
		if b, err := json.Marshal(ev); err == nil {
			feed.Observer{DispatchRoot: dispatch.Root()}.ObserveHook(b)
		}
		return hooks.Response{}, nil
	}})
	return append(hs, hooks.UserHandlers(os.Getenv("PATH"))...)
}

// steerAckCommand is what an agent runs to acknowledge: this binary, so an
// older hev earlier on PATH cannot answer for it.
func steerAckCommand() string {
	exe, err := os.Executable()
	if err != nil {
		return "hev steer ack"
	}
	return dispatch.Quote(exe) + " steer ack"
}

func tmuxSessionName() string {
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	b, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", pane, "#S").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func init() {
	hook := &cobra.Command{
		Use:                "hook EVENT --harness claude|codex|copilot",
		Short:              "Run kit's hook layer for one harness event (installed by hev hooks)",
		Long:               hookHelp,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			runHook(args, cmd.InOrStdin(), cmd.OutOrStdout())
			return nil
		},
	}
	rootCmd.AddCommand(hook)
	rootCmd.AddCommand(newHooksCommand())
}

func hookHarnessArgs(harness string) ([]string, error) {
	if harness == "" || harness == "all" {
		return hooks.Harnesses, nil
	}
	if harness == "pi" {
		return nil, fmt.Errorf("pi: not supported: kit has no verified pi hook (pi is not installed on our hosts), and dispatch and orchestrators do not run pi; only a note reaches a pi peers session, through peers send")
	}
	if !hooks.ValidHarness(harness) {
		return nil, fmt.Errorf("unknown harness %q (want claude, codex or copilot)", harness)
	}
	return []string{harness}, nil
}

// hookExecutable is the hev the installed hooks run: the installed binary
// when this is it, else this executable.
func hookExecutable(home string) string {
	exe, err := os.Executable()
	if err != nil {
		exe = "hev"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	installed := filepath.Join(home, ".local", "bin", "hev")
	if r, err := filepath.EvalSymlinks(installed); err == nil && r == exe {
		return installed
	}
	return exe
}

func newHooksCommand() *cobra.Command {
	c := &cobra.Command{Use: "hooks", Short: "Install, check and remove kit's harness hooks"}
	var harness string
	add := func(use, short string, run func(cmd *cobra.Command, home string, harnesses []string) error) {
		sub := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			hs, err := hookHarnessArgs(harness)
			if err != nil {
				return err
			}
			return run(cmd, home, hs)
		}}
		sub.Flags().StringVar(&harness, "harness", "", "claude, codex or copilot (default: every harness whose home exists)")
		c.AddCommand(sub)
	}
	add("install", "Merge kit's hooks into Claude Code and Codex settings", func(cmd *cobra.Command, home string, hs []string) error {
		return installHooks(cmd.OutOrStdout(), home, hs, harness != "")
	})
	add("check", "Report whether kit's hooks are installed and current", func(cmd *cobra.Command, home string, hs []string) error {
		return checkHooks(cmd.OutOrStdout(), home, hs, harness != "")
	})
	add("uninstall", "Remove only kit's hook entries", func(cmd *cobra.Command, home string, hs []string) error {
		for _, h := range hs {
			n, err := hooks.Uninstall(home, h)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: removed %d kit hook entries from %s\n", h, n, hooks.SettingsPath(home, h))
		}
		return nil
	})
	return c
}

func installHooks(out io.Writer, home string, hs []string, explicit bool) error {
	exe := hookExecutable(home)
	installed := 0
	for _, h := range hs {
		st, _ := hooks.Inspect(home, h, exe)
		if !st.Present && !explicit {
			fmt.Fprintf(out, "%s: %s not found; skipped\n", h, hooks.HarnessHome(home, h))
			continue
		}
		changed, err := hooks.Install(home, h, exe)
		if err != nil {
			return fmt.Errorf("%s: %w", h, err)
		}
		installed++
		verb := "already current"
		if changed {
			verb = "installed"
		}
		fmt.Fprintf(out, "%s: %s %d events in %s\n", h, verb, len(hooks.Events), hooks.SettingsPath(home, h))
		if registry.HooksFor(h).TrustRequired && !hooks.CodexTrusted(home, hooks.SettingsPath(home, h)) {
			fmt.Fprintln(out, "codex: Codex runs global hooks only after a person trusts them: open codex and review them with /hooks. Dispatch runs do not need this; their supervisor wires hooks per launch.")
		}
	}
	if installed == 0 {
		return fmt.Errorf("no harness home found under %s; start Claude Code, Codex or GitHub Copilot CLI once, or pass --harness", home)
	}
	return nil
}

func checkHooks(out io.Writer, home string, hs []string, explicit bool) error {
	exe := hookExecutable(home)
	bad := false
	for _, h := range hs {
		st, err := hooks.Inspect(home, h, exe)
		if err != nil {
			fmt.Fprintf(out, "%s: %v\n", h, err)
			bad = true
			continue
		}
		if !st.Present && !explicit {
			fmt.Fprintf(out, "%s: not set up here\n", h)
			continue
		}
		switch {
		case st.Current():
			fmt.Fprintf(out, "%s: current (%d events, %d other hooks kept) in %s\n", h, len(st.Kit), st.Foreign, st.Path)
		case len(st.Kit)+len(st.Stale) == 0:
			fmt.Fprintf(out, "%s: not installed in %s (hev hooks install --harness %s)\n", h, st.Path, h)
			bad = true
		default:
			fmt.Fprintf(out, "%s: needs repair: missing %v, stale %v in %s (hev hooks install --harness %s)\n", h, st.Missing, st.Stale, st.Path, h)
			bad = true
		}
		if registry.HooksFor(h).TrustRequired && len(st.Kit) > 0 && !hooks.CodexTrusted(home, st.Path) {
			fmt.Fprintln(out, "codex: global hooks not yet trusted; review them in codex with /hooks")
		}
	}
	if bad {
		return fmt.Errorf("kit hooks need attention")
	}
	return nil
}

// refreshInstalledHooks repairs kit's hooks where they are already installed
// (hev skills sync, hev update). It never installs hooks nobody asked for.
func refreshInstalledHooks(out io.Writer, home string) {
	exe := hookExecutable(home)
	for _, h := range hooks.Harnesses {
		st, err := hooks.Inspect(home, h, exe)
		if err != nil || len(st.Kit)+len(st.Stale) == 0 || st.Current() {
			continue
		}
		if _, err := hooks.Install(home, h, exe); err != nil {
			fmt.Fprintf(out, "hooks: %s: %v\n", h, err)
			continue
		}
		fmt.Fprintf(out, "hooks: %s: repaired kit's hook entries\n", h)
	}
}
