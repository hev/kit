package main

import (
	"context"
	"debug/buildinfo"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/update"
	"github.com/hev/kit/internal/version"
	"github.com/spf13/cobra"
)

// updateSource is where builds come from: the official repository. Tests
// replace it with a local fake; nothing at run time can.
var updateSource = update.Official

func init() {
	rootCmd.AddCommand(newUpdateCommand())
	dispatch.UpdateHost = func(ctx context.Context, q update.Request, by string) (update.Report, error) {
		u, err := newHostUpdater(by, io.Discard)
		if err != nil {
			return update.Report{}, err
		}
		return runHostUpdate(ctx, u, q)
	}
	dispatch.HostChannel = func() string {
		c, err := update.LoadConfig(daemon.DefaultConfigPath())
		if err != nil {
			return ""
		}
		return c.EffectiveChannel()
	}
	daemon.AfterCycle = daemonAutoUpdate
	daemon.OnStart = daemonUpdateStart
}

func newUpdateCommand() *cobra.Command {
	var channel, to string
	var check, rollback, asJSON bool
	c := &cobra.Command{
		Use:   "update",
		Short: "Move this host to the newest build on its channel, or a named one",
		Long: `Move this host's hev to another published build of the official repository.

With no flags, the newest build on the host's channel ([update] channel in its
config; release unless set). --channel switches channel and records it.
--to names a version (X.Y.Z or X.Y.Z-edge.<sha>). --check reports only.
--rollback returns to the build the last update replaced.

The build's archive is verified against its published digest before anything
is replaced. It is installed beside the current build and switched in
atomically; runs in progress finish on the binary they started with. The
capture daemon (and anything else kit installed as a service) restarts onto
it, bundled skills are refreshed, and [update] post_update commands run. A
failed restart switches back.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if rollback && (to != "" || channel != "") {
				return fmt.Errorf("--rollback takes no --to or --channel")
			}
			u, err := newHostUpdater("cli", cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			q := update.Request{Channel: channel, To: to, Check: check, Rollback: rollback}
			if channel != "" && !check {
				q.SaveChannel = channel
			}
			r, err := runHostUpdate(cmd.Context(), u, q)
			if asJSON {
				out := struct {
					update.Report
					Error string `json:"error,omitempty"`
				}{Report: r}
				if err != nil {
					out.Error = err.Error()
				}
				if e := json.NewEncoder(cmd.OutOrStdout()).Encode(out); e != nil {
					return e
				}
				if err != nil {
					cmd.SilenceErrors, cmd.SilenceUsage = true, true
				}
				return err
			}
			for _, s := range r.Steps {
				fmt.Fprintf(cmd.OutOrStdout(), "  • %s\n", s)
			}
			if err != nil {
				cmd.SilenceUsage = true
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), r.Message)
			return nil
		},
	}
	c.Flags().StringVar(&channel, "channel", "", "release or edge; recorded as this host's channel")
	c.Flags().StringVar(&to, "to", "", "Move to this version (X.Y.Z or X.Y.Z-edge.<sha>)")
	c.Flags().BoolVar(&check, "check", false, "Report what would change; change nothing")
	c.Flags().BoolVar(&rollback, "rollback", false, "Return to the build the last update replaced")
	c.Flags().BoolVar(&asJSON, "json", false, "Print the report as JSON")
	return c
}

// newHostUpdater wires the updater to this host: its home, its config, the
// running binary, kit's services on this OS, the skills sync and the host's
// own post-update hooks.
func newHostUpdater(by string, out io.Writer) (*update.Updater, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	cfgPath := daemon.DefaultConfigPath()
	cfg, err := update.LoadConfig(cfgPath)
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	u := &update.Updater{
		Home: home, Source: updateSource, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		Current: version.String(), Invoked: update.Invoked(), Exe: exe,
		Extra: serviceBinaries(home), Config: cfg, ConfigPath: cfgPath,
		Services: hostServices(home), Smoke: update.DefaultSmoke,
		SyncSkills: syncSkillsFrom(home), RunHook: update.DefaultRunHook,
		By: by, Out: out,
	}
	adjustUpdater(u)
	return u, nil
}

// adjustUpdater lets tests point the updater at a temporary install and fake
// services instead of the test binary and this machine's service managers.
var adjustUpdater = func(*update.Updater) {}

// runHostUpdate runs one request and adds what the updater cannot do itself:
// a daemon running outside any service manager is named, not killed.
func runHostUpdate(ctx context.Context, u *update.Updater, q update.Request) (update.Report, error) {
	r, err := u.Run(ctx, q)
	if err == nil && r.Changed && !q.Self {
		present := false
		for _, s := range u.Services {
			present = present || s.Present()
		}
		if pids := findDaemonPIDs(); !present && len(pids) > 0 {
			r.Steps = append(r.Steps, fmt.Sprintf("hevd is running outside a service manager (pid %v); restart it to move it to %s", pids, r.After))
		}
	}
	return r, err
}

// serviceBinaries are the paths kit's own services run hev from that hold a
// kit binary: hev up installs ~/.local/bin/hev and names it in the service.
func serviceBinaries(home string) []string {
	p := filepath.Join(home, ".local", "bin", update.BinaryName(runtime.GOOS))
	if info, err := buildinfo.ReadFile(p); err == nil && info.Path == "github.com/hev/kit/cmd/hev" {
		return []string{p}
	}
	return nil
}

// syncSkillsFrom refreshes bundled skills with the new binary, so the skills
// it embeds are the ones installed, and only where a harness has skills.
func syncSkillsFrom(home string) func(context.Context, string) error {
	return func(ctx context.Context, bin string) error {
		present := false
		for _, d := range []string{".claude/skills", ".codex/skills", ".agents/skills"} {
			if info, err := os.Stat(filepath.Join(home, d)); err == nil && info.IsDir() {
				present = true
			}
		}
		if !present {
			return nil
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		out, err := exec.CommandContext(ctx, bin, "skills", "sync").CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
}

// hostServices are the services kit installs on this OS, in restart order.
func hostServices(home string) []update.Service {
	switch hostOS {
	case "darwin":
		return []update.Service{launchdService{home: home}}
	case "linux":
		return []update.Service{systemdService{home: home}}
	case "windows":
		return windowsUpdateServices(home)
	}
	return nil
}

// launchdService is hevd's LaunchAgent, when hev up installed it.
type launchdService struct{ home string }

func (s launchdService) Name() string { return "hevd (launchd " + launchdLabel() + ")" }
func (s launchdService) Present() bool {
	_, err := os.Stat(daemonJob("", s.home, "").plistPath(s.home))
	return err == nil
}
func (s launchdService) Restart(ctx context.Context) error {
	job := daemonJob("", s.home, "")
	if out, err := exec.CommandContext(ctx, "launchctl", "kickstart", "-k", job.target()).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl kickstart: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return confirmRunning(ctx, func() (string, bool) {
		out, err := exec.CommandContext(ctx, "launchctl", "print", job.target()).Output()
		if err != nil {
			return "", false
		}
		for _, l := range strings.Split(string(out), "\n") {
			if f := strings.Fields(l); len(f) == 3 && f[0] == "pid" && f[1] == "=" {
				return f[2], strings.Contains(string(out), "state = running")
			}
		}
		return "", false
	})
}

// systemdService is the hevd user unit, when hev up installed it.
type systemdService struct{ home string }

func (s systemdService) Name() string { return "hevd (systemd --user " + daemonUnit + ")" }
func (s systemdService) Present() bool {
	if _, err := os.Stat(systemdPath(s.home)); err != nil {
		return false
	}
	return hostOS == "linux" && systemdOwnership(s.home) == nil
}
func (s systemdService) Restart(ctx context.Context) error {
	// The unit is installed, so a session that cannot reach the user manager
	// (an SSH session without XDG_RUNTIME_DIR) must not skip it silently.
	if !linuxService.Available() {
		return fmt.Errorf("the systemd user manager is not reachable from this session; enable lingering (loginctl enable-linger) or run hev update in a login session")
	}
	if err := systemdRun("restart", daemonUnit); err != nil {
		return err
	}
	return confirmRunning(ctx, func() (string, bool) {
		out, _ := linuxService.Run("show", "--property=MainPID,ActiveState", daemonUnit)
		pid, active := "", false
		for _, l := range strings.Split(string(out), "\n") {
			k, v, _ := strings.Cut(strings.TrimSpace(l), "=")
			switch k {
			case "MainPID":
				pid = v
			case "ActiveState":
				active = v == "active"
			}
		}
		return pid, active && pid != "" && pid != "0"
	})
}

// confirmRunning waits for a service to report a running process and then
// for that same process to still be running a few seconds later: a binary
// that exits at start is restarted by the manager under a new pid.
func confirmRunning(ctx context.Context, probe func() (pid string, running bool)) error {
	deadline := time.Now().Add(20 * time.Second)
	for {
		pid, ok := probe()
		if ok {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(3 * time.Second):
			}
			again, still := probe()
			if still && again == pid {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("not running 20s after restart")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// daemonAutoUpdate runs after each daemon cycle: it confirms a self-update
// the daemon is now running cleanly on, then, when the host's config turns
// automatic updates on, checks on its schedule and updates by the same path
// as `hev update`. True re-executes the daemon onto the new build.
func daemonAutoUpdate(ctx context.Context, logger *slog.Logger) bool {
	u, err := newHostUpdater("daemon", io.Discard)
	if err != nil {
		logger.Warn("update: config", "err", err)
		return false
	}
	if err := u.Confirm(); err != nil {
		logger.Warn("update: confirm", "err", err)
	}
	res := u.Auto(ctx)
	if !res.Ran {
		return false
	}
	if res.Err != nil {
		logger.Warn("update: scheduled check failed", "channel", u.Config.Channel, "err", res.Err, "steps", res.Report.Steps)
		return false
	}
	logger.Info("update: scheduled check", "channel", u.Config.Channel, "before", res.Report.Before, "after", res.Report.After, "message", res.Report.Message, "steps", res.Report.Steps)
	return res.Restart
}

// daemonUpdateStart rolls back a self-update the daemon has repeatedly failed
// to start on.
func daemonUpdateStart(ctx context.Context, logger *slog.Logger) bool {
	u, err := newHostUpdater("daemon", io.Discard)
	if err != nil {
		return false
	}
	rolled, err := u.StartCheck(ctx)
	if err != nil {
		logger.Warn("update: start check", "err", err)
	}
	if rolled {
		logger.Warn("update: rolled back a build the daemon could not start on")
	}
	return rolled
}
