package intake

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func run(args []string, out io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "run", "once", "check", "render", "reconcile":
			args = append([]string{"linear"}, args...)
		}
	}
	if len(args) > 0 {
		switch args[0] {
		case "install", "start", "stop", "status":
			return runService(args)
		}
	}
	if len(args) == 1 && args[0] == "--hev-describe" {
		fmt.Fprintln(out, "Poll Linear assignments and dispatch configured orders")
		return nil
	}
	if len(args) < 2 || args[0] != "linear" {
		return fmt.Errorf("usage: hev watch linear {run|once|status|reconcile|render|check} --config FILE")
	}
	action := args[1]
	if action == "render" || action == "check" {
		return runTemplate(args[1:], out, os.Stderr)
	}
	if action != "run" && action != "once" && action != "status" && action != "reconcile" {
		return fmt.Errorf("unknown subcommand %q", action)
	}
	name := ""
	if action == "reconcile" {
		if len(args) < 3 || !ValidName(args[2]) {
			return fmt.Errorf("reconcile NAME required")
		}
		name = args[2]
		args = append([]string{args[0], args[1]}, args[3:]...)
	}
	flags := flag.NewFlagSet(action, flag.ContinueOnError)
	admitted := flags.Bool("admitted", false, "record confirmed admission")
	notAdmitted := flags.Bool("not-admitted", false, "release lease for retry")
	path := flags.String("config", "", "TOML configuration")
	if e := flags.Parse(args[2:]); e != nil {
		return e
	}
	if *path == "" || flags.NArg() != 0 {
		return fmt.Errorf("--config FILE required")
	}
	if action == "reconcile" && *admitted == *notAdmitted {
		return fmt.Errorf("choose exactly one of --admitted or --not-admitted")
	}
	c, e := loadConfig(*path)
	if e != nil {
		return e
	}
	key := ""
	if action != "status" && action != "reconcile" {
		key = os.Getenv(c.Linear.APIKeyEnv)
		if key == "" {
			return fmt.Errorf("API key environment variable is empty")
		}
	}
	p, e := newPoller(c, key, out)
	if e != nil {
		return e
	}
	if action == "status" {
		paused, e := exists(c.PauseFile)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(map[string]any{"sources": p.state.Sources, "comment_runs": p.state.CommentRuns, "last_poll": p.state.LastPoll, "paused": paused, "today": p.day(), "backoff_until": p.state.BackoffUntil, "pending": p.state.Pending, "needs-reconcile": p.state.NeedsReconcile, "ticket_backoff": p.state.TicketFailures, "effects": p.state.Effects})
	}
	// A single writer owns state and the dispatch admission window.
	if e = os.MkdirAll(c.StateDir, 0700); e != nil {
		return e
	}
	unlock, e := lockState(filepath.Join(c.StateDir, "RUNNING"))
	if e != nil {
		return e
	}
	defer unlock()
	p, e = newPoller(c, key, out)
	if e != nil {
		return e
	}
	if e = p.cleanTemp(); e != nil {
		return e
	}
	if action == "reconcile" {
		return p.reconcile(name, *admitted)
	}
	if action == "run" {
		if e = p.start(); e != nil {
			return e
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if action == "once" {
		if e = p.poll(ctx); e != nil {
			_ = p.log(Decision{Decision: "error", Reason: e.Error()})
		}
		return e
	}
	interval, _ := time.ParseDuration(c.Interval)
	for {
		if e = p.poll(ctx); e != nil {
			if le := p.log(Decision{Decision: "error", Reason: e.Error()}); le != nil {
				return le
			}
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

type commandError struct {
	code int
	err  error
}

func (e *commandError) Error() string { return e.err.Error() }
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var e *commandError
	if errors.As(err, &e) {
		return e.code
	}
	return 1
}
func usageError(err error) error { return &commandError{2, err} }

// Run executes the deterministic intake CLI.
func Run(args []string, out io.Writer) error {
	if len(args) > 0 && (args[0] == "watch" || args[0] == "intake") {
		args = args[1:]
	}
	return run(args, out)
}

// ExitCode preserves the intake command error status for the unified CLI.
func ExitCode(err error) int { return exitCode(err) }
