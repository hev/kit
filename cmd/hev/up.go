package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/hev/kit/internal/layerenv"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/grep"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/local"
	"github.com/spf13/cobra"
)

var (
	upNoDashboard    bool
	upStore          string
	upNonInteractive bool
	upNoPicker       bool
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start capture into local files, or into hev layer with a key",
	Long: `Start everything kit needs on this machine and leave the archive filling.

  hev up
  hev traces search "preflight failed 401"

With no hev layer key (Starter Kit), up writes the config and installs the
capture daemon and the dashboard (` + "`hev serve`" + `) under launchd (macOS) or
systemd (Linux), plus the bundled agent skills for Claude Code and Codex.
Nothing runs in Docker. Traces are kept in files on this machine
(~/.hev/traces, or [archive] dir / HEV_ARCHIVE_DIR), after the same
redaction hev layer gets. Search there is grep: it matches the words you type,
not their meaning. The dashboard listens on 127.0.0.1:[local] serve_port
(8099 by default).

A hev layer key moves traces to the shared archive, with search by meaning:

  export HEVLAYER_API_KEY=<your-hev-layer-key>
  hev up

HEVLAYER_ENDPOINT optionally selects another hev layer gateway. The hosted
lane installs capture without starting a local gateway. Use ` + "`hev open`" + `
to sign in to the public hosted dashboard; custom gateways provide their own.
HEVLAYER_NAMESPACE selects your archive. Saved targets are reused unless
--store or HEVLAYER_STORE explicitly chooses another lane.

A config from the retired local Docker lane (Layer CE on Postgres) is moved
to files: up exports its rows when its containers start, otherwise it
re-indexes the transcripts still on disk and says how many sessions could not
be recovered. The containers and the Docker volume are left alone.

Bring-your-own Turbopuffer is hosted-only now: there is no local gateway lane.

Data lives under ~/.hev/<surface>/ (traces, board, orders, peers, hooks,
evidence). A machine that already has ~/.hev/archive, ~/dispatch or
~/.local/state/peers-v2 keeps using them; see https://hevkit.com/docs/traces. With no
dispatch config, up writes one naming this machine as the local host "here".

On a terminal, the first ` + "`hev up`" + ` on an unconfigured machine runs ` + "`hev onboard`" + `
first. Without a terminal, or with --non-interactive, up never asks anything.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !upNonInteractive && interactive(cmd.InOrStdin(), cmd.OutOrStdout()) && unconfigured() {
			return runOnboard(cmd.Context(), onboardOptions{In: cmd.InOrStdin(), Out: cmd.OutOrStdout(), NoPicker: upNoPicker, NoDashboard: upNoDashboard})
		}
		return runUp(cmd.Context(), cmd.OutOrStdout(), upNoDashboard)
	},
}

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the capture daemon and the local dashboard",
	Long: `Unload the capture daemon and, with no key, the dashboard service.

The archive is left exactly as it is: in files on this machine with no key,
or in hev layer. ` + "`hev up`" + ` picks it up again. kit runs nothing in Docker,
so down touches no container.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runDown(cmd.Context(), cmd.OutOrStdout())
	},
}

func init() {
	// Their errors are one line that says what to do next; a usage dump under
	// that line, and the line again, bury it.
	for _, c := range []*cobra.Command{upCmd, downCmd} {
		c.SilenceUsage, c.SilenceErrors = true, true
	}
	upCmd.Flags().BoolVar(&upNoDashboard, "no-dashboard", false, "Skip the local dashboard service or hosted dashboard instructions")
	upCmd.Flags().BoolVar(&upNonInteractive, "non-interactive", false, "Never prompt, even on a terminal (CI and scripts; up never prompts without a terminal)")
	upCmd.Flags().BoolVar(&upNoPicker, "no-picker", false, "When up runs first-run onboarding, do not launch the Picker afterwards")
	upCmd.Flags().StringVar(&upStore, "store", "", "hosted (hev layer, needs HEVLAYER_API_KEY) or files (no key; the default without one)")
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
}

// hostOS is runtime.GOOS, held in a variable so the launchd half of up and
// down is decided in one place and can be tested from any platform.
var hostOS = runtime.GOOS

// portFree is local.PortFree, replaceable where a test stands a stub on the
// port `up` is about to check.
var portFree = local.PortFree

func hasLaunchd() bool { return hostOS == "darwin" }

func tick(out io.Writer, format string, args ...any) {
	fmt.Fprintf(out, "  ✓ "+format+"\n", args...)
}

// upFiles is the lane with no Layer API key: traces in files, no Docker.
const upFiles = "files"

// errRetiredStore answers an explicit choice of the retired local store.
var errRetiredStore = fmt.Errorf("--store %s: the local Docker lane is retired; run `hev up` with no key to keep traces in files on this machine, or set HEVLAYER_API_KEY for hev layer", daemon.RetiredStore)

// errTurbopufferHostedOnly answers a choice of the removed BYO Turbopuffer
// lane, which ran a gateway in Docker in front of the user's account.
var errTurbopufferHostedOnly = fmt.Errorf("--store %s: bring-your-own Turbopuffer is hosted-only now (kit no longer runs a local gateway in Docker); set HEVLAYER_API_KEY to your hev layer key and run `hev up --store hosted`, or run `hev up` with no key to keep traces in files", layer.StoreTurbopuffer)

// errTurbopufferLane answers `hev up` on a config the removed BYO Turbopuffer
// lane wrote: kit will not guess which way to move it.
func errTurbopufferLane(configPath string) error {
	return fmt.Errorf("%s names the BYO Turbopuffer lane, which ran a local gateway in Docker and is gone in this release: BYO Turbopuffer is hosted-only now. Run `hev up --store files` to keep traces in files on this machine (the old containers are left alone; stop them with `docker compose -p <project> down`), or set HEVLAYER_API_KEY to your hev layer key and run `hev up --store hosted`", configPath)
}

// upLane preserves a saved archive unless a store choice explicitly moves it.
// Environment keys provide defaults only when the saved target has no key.
func upLane(flag string) (store, apiKey string, err error) {
	layerenv.WarnDeprecated(os.Stderr)
	store = strings.ToLower(strings.TrimSpace(flag))
	if store == "" {
		store = strings.ToLower(strings.TrimSpace(layerenv.Get("HEVLAYER_STORE")))
	}
	file, err := loadInitConfig(daemon.DefaultConfigPath())
	if err != nil {
		return "", "", err
	}
	switch store {
	case daemon.RetiredStore:
		return "", "", errRetiredStore
	case layer.StoreTurbopuffer:
		return "", "", errTurbopufferHostedOnly
	}
	gatewayKey := strings.TrimSpace(layerenv.Get("HEVLAYER_API_KEY"))
	if gatewayKey == layerenv.PlaceholderKey {
		gatewayKey = ""
	}
	saved := file.Layer.Store
	if saved == daemon.RetiredStore {
		// A config the retired lane wrote: decided afresh, as if it named
		// no store. runUp says what happened to the lane.
		saved = ""
	}
	if saved == layer.StoreTurbopuffer {
		if file.Layer.Endpoint == "" || localEndpoint(file.Layer.Endpoint) {
			// The removed local-gateway lane.
			if store == "" {
				return "", "", errTurbopufferLane(daemon.DefaultConfigPath())
			}
		} else {
			// Your own hev layer gateway in front of Turbopuffer (`hev
			// init`): a Layer target like any other.
			saved = "hosted"
		}
	}

	if store == "" {
		store = saved
		if store == "" && file.Layer.Endpoint != "" && !localEndpoint(file.Layer.Endpoint) {
			store = "hosted"
		}
		if store == "" {
			switch {
			case gatewayKey != "":
				store = "hosted"
			default:
				store = upFiles
			}
		}
	}
	apiKey = file.Layer.APIKey
	if apiKey == layerenv.PlaceholderKey || (saved != "" && store != saved) {
		apiKey = ""
	}
	if apiKey == "" {
		apiKey = gatewayKey
	}
	switch store {
	case "hosted":
		if apiKey == "" {
			if file.Layer.Endpoint != "" && saved == "" {
				return "", "", fmt.Errorf("will not repoint a hosted config: configure its gateway key or choose --store explicitly; with no key, remove [layer] endpoint from the config to keep traces in files")
			}
			return "", "", fmt.Errorf("hosted hev layer requires HEVLAYER_API_KEY")
		}
		return store, apiKey, nil
	case upFiles:
		return store, "", nil
	}
	return "", "", fmt.Errorf("--store %s: expected hosted or files (with no key, `hev up` keeps traces in files)", store)
}

// runUp performs the steps of RFC 0006 in order. Each is a precondition of the
// next, and each first asks whether it is already done.
func runUp(ctx context.Context, out io.Writer, noDashboard bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	// A config that names a hosted Layer is refused, and a removed lane is
	// reported, before anything is started or written — and before the
	// config is loaded in full, because loading shells out (`gh`, for the
	// capture identity) and what another tool writes under HOME is still a
	// write on the refusal path.
	layerenv.WarnDeprecated(out)
	store, apiKey, err := upLane(upStore)
	if err != nil {
		return err
	}
	if err := bootstrapDispatch(out, upWorkspace); err != nil {
		return err
	}
	if store == "hosted" {
		cfg, err := daemon.LoadConfig()
		if err != nil {
			return err
		}
		return runHostedUp(ctx, out, home, cfg, apiKey, noDashboard)
	}
	return runFilesUp(ctx, out, home, noDashboard)
}

// upCapture installs hevd under launchd or systemd, or says how to run it
// where there is neither. daemonReady reports that it is running.
func upCapture(out io.Writer, home, configPath string, w daemon.LocalWrite) (bool, error) {
	daemonReady := hasLaunchd()
	if err := systemdOwnership(home); hostOS == "linux" && err != nil {
		fmt.Fprintf(out, "  • %s; run `hev d` to capture\n", err)
	} else if hasSystemd() {
		if err := upSystemd(home, configPath, w.Changed); err != nil {
			if !errors.Is(err, errUnmanaged) {
				return false, err
			}
			fmt.Fprintf(out, "  • %s; run `hev d` to capture\n", err)
		} else {
			daemonReady = true
			tick(out, "hevd running under systemd")
		}
	} else if !hasLaunchd() {
		fmt.Fprintf(out, "  • no launchd on %s: run `hev d` to capture\n", hostOS)
	} else if err := upDaemon(out, home, configPath, w); err != nil {
		return false, err
	}
	return daemonReady, nil
}

// runFilesUp is `hev up` with no Layer API key: the config names no key, the
// capture daemon keeps traces in files, the dashboard is `hev serve` under the
// user's service manager, and nothing runs in Docker. A config the retired
// local Docker lane wrote is migrated first (migrateRetired); its containers
// and volume are never removed.
func runFilesUp(ctx context.Context, out io.Writer, home string, noDashboard bool) error {
	cfg, err := daemon.LoadConfig()
	if err != nil {
		return err
	}
	retired := strings.EqualFold(cfg.LayerStore, daemon.RetiredStore)
	if retired {
		if err := migrateRetired(ctx, out, cfg); err != nil {
			return err
		}
	}
	w, err := daemon.WriteFilesConfig(cfg.Local)
	if err != nil {
		return err
	}
	if cfg, err = daemon.LoadConfig(); err != nil {
		return err
	}
	target, err := daemon.OpenTarget(cfg, "")
	if err != nil {
		return err
	}
	if target.Files == nil {
		return fmt.Errorf("a Layer API key is configured: run `hev up` to capture into hev layer")
	}
	tick(out, "no hev layer key: traces are kept in files under %s, searched with %s", target.Files.Dir(), grepEngine())
	if err := ensureSurfaces(out); err != nil {
		return err
	}
	configPath, err := filepath.Abs(w.Path)
	if err != nil {
		return err
	}
	daemonReady, err := upCapture(out, home, configPath, w)
	if err != nil {
		return err
	}
	if harnesses, err := installSkills(home); err != nil {
		return fmt.Errorf("install agent skills: %w", err)
	} else if len(harnesses) > 0 {
		tick(out, "bundled skills checked for %s", strings.Join(harnesses, ", "))
	}
	dashboard := hevdLine{"dashboard", "hev serve"}
	if !noDashboard {
		url, err := upServe(ctx, out, home, configPath, cfg.Local.ServePort, w.Changed)
		if err != nil {
			return err
		}
		if url != "" {
			dashboard.value = url
		}
	}
	lead := hevdLine{value: "hevd is up: capturing traces into files and taking dispatched work on this machine."}
	if !daemonReady {
		lead.value = "the config is written. run `hev d` to capture."
	}
	printHevd(out, lead,
		hevdLine{"search", `hev traces search "preflight failed 401"`},
		dashboard,
		hevdLine{"dispatch", "hev dispatch ls"},
		hevdLine{"archive", target.Files.Dir()},
		hevdLine{"stop", "hev down"})
	return nil
}

// grepEngine names what runs search on this machine.
func grepEngine() string {
	if grep.RgPath() != "" {
		return "ripgrep"
	}
	return "kit's built-in grep (install ripgrep for speed)"
}

// upDaemon installs hevd, restarting it when the binary or the config it reads
// has changed. A config moved to another archive also forgets its index
// state, with the daemon stopped: those units were indexed into an archive the
// config no longer names.
func upDaemon(out io.Writer, home, configPath string, w daemon.LocalWrite) error {
	if err := os.MkdirAll(filepath.Join(home, ".hev"), 0o755); err != nil {
		return err
	}
	bin, updated, err := installSelf(home)
	if err != nil {
		return err
	}
	job := daemonJob(bin, home, configPath)
	if w.Migrated {
		if _, err := job.bootout(); err != nil {
			return err
		}
		if err := index.ResetState(); err != nil {
			return err
		}
		tick(out, "moved to a new archive: every transcript will be indexed again")
	}
	changed, err := job.ensure(home, updated || w.Changed)
	if err != nil {
		return err
	}
	if changed {
		tick(out, "hevd installed, first scan started")
	} else {
		tick(out, "hevd already running")
	}
	return nil
}

// runDown is up's mirror: the daemon and the dashboard service are unloaded
// (KeepAlive jobs are unloaded rather than signalled). Nothing is deleted:
// the archive is in files or in hev layer, and the index state that
// describes it stays true. kit runs nothing in Docker, so no container is
// touched; a config the retired local lane wrote is told where its
// containers are.
func runDown(ctx context.Context, out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfg, err := daemon.LoadConfig()
	if err != nil {
		return err
	}
	if strings.EqualFold(cfg.LayerStore, daemon.RetiredStore) || cfg.LayerStore == layer.StoreTurbopuffer {
		project := cfg.Local.Project
		if project == "" {
			project = daemon.DefaultLocalProject
		}
		fmt.Fprintf(out, "  • %s names a removed local Docker lane; its containers are left as they are (`docker compose -p %s down` stops them)\n", cfg.ConfigPath, project)
	}

	// A config `up` never wrote: the jobs on this machine belong to something
	// else — a hosted daemon, a deploy's read side — and are not down's to
	// touch.
	if !cfg.Local.Managed && cfg.LayerStore != "hosted" {
		fmt.Fprintf(out, "  • %s has no [local] block: not written by `hev up`, so services are left alone\n", cfg.ConfigPath)
		return nil
	}
	if err := systemdOwnership(home); hostOS == "linux" && err != nil {
		fmt.Fprintf(out, "  • %s; stop `hev d` yourself\n", err)
	} else if hasSystemd() {
		if err := downSystemd(home); err != nil {
			if !errors.Is(err, errUnmanaged) && !errors.Is(err, errNoSystemdUnit) {
				return err
			}
			fmt.Fprintf(out, "  • %s; stop `hev d` yourself\n", err)
		} else {
			tick(out, "hevd stopped")
		}
		if err := downServe(out, home); err != nil {
			return err
		}
	} else if !hasLaunchd() {
		fmt.Fprintf(out, "  • no launchd on %s: no jobs to unload; stop `hev d` yourself\n", hostOS)
	} else if err := unloadJobs(out, home); err != nil {
		return err
	}
	return nil
}

// unloadJobs boots out the daemon, and the read-side job an older `up`
// installed, and removes their plists, so that neither launchd now nor the
// next login brings them back. A job that was never there is not reported.
func unloadJobs(out io.Writer, home string) error {
	for _, job := range []launchdJob{{Label: launchdLabel()}, {Label: serveLabel()}} {
		wasLoaded, err := job.bootout()
		if err != nil {
			return err
		}
		if err := os.Remove(job.plistPath(home)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove plist: %w", err)
		}
		if wasLoaded {
			tick(out, "%s unloaded", job.Label)
		} else if job.Label == launchdLabel() {
			tick(out, "%s not loaded", job.Label)
		}
	}
	return nil
}

func reachable(url string) error {
	probe := &http.Client{Timeout: 2 * time.Second}
	resp, err := probe.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func waitFor(ctx context.Context, timeout time.Duration, check func() error) error {
	deadline := time.Now().Add(timeout)
	for {
		err := check()
		if err == nil || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
