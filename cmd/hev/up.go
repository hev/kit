package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/local"
	"github.com/hev/kit/internal/storagemode"
	"github.com/spf13/cobra"
)

var (
	upNoDashboard bool
	upStore       string
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start capture into local files, or into hev layer with a key",
	Long: `Start everything kit needs on this machine and leave the archive filling.

  hev up
  hev traces search "why did the preflight fail"

With no Layer API key, up writes the config, installs the capture daemon
under launchd (macOS) or systemd (Linux) and the bundled agent skills for
Claude Code and Codex, and starts nothing in Docker. Traces are kept in files
on this machine (~/.hev/archive, or [archive] dir / HEV_ARCHIVE_DIR), after
the same redaction Layer gets. Search there is lexical: it matches words, not
meaning. ` + "`hev serve`" + ` shows the dashboard over the same files.

Hosted hev layer is the shared archive for capture and the dashboard:

  export HEVLAYER_API_KEY=<your-hev-layer-key>
  hev up

HEVLAYER_ENDPOINT optionally selects another hev layer gateway. The hosted
lane installs capture without starting a local gateway. Use ` + "`hev open`" + `
to sign in to the public hosted dashboard; custom gateways provide their own.
HEVLAYER_NAMESPACE selects your archive. Saved targets are reused unless
--store or HEVLAYER_STORE explicitly chooses another lane.

Advanced: bring your own Turbopuffer account with an explicit store choice:
Set HEVLAYER_API_KEY and run ` + "`hev up --store turbopuffer`" + `, or choose it in ` + "`hev init`" + `.
That lane runs a local gateway and dashboard in Docker in front of your account.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runUp(cmd.Context(), cmd.OutOrStdout(), upNoDashboard)
	},
}

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Unload the daemon, and stop the containers on the Turbopuffer lane",
	Long: `Unload the capture daemon, and on the Turbopuffer lane stop the gateway and
dashboard containers.

The archive is left exactly as it is: in files on this machine with no key,
in hev layer on the hosted lane, or in your own account on the advanced BYO
lane. ` + "`hev up`" + ` picks it up again.`,
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
	upCmd.Flags().BoolVar(&upNoDashboard, "no-dashboard", false, "Skip the local dashboard or hosted dashboard instructions")
	upCmd.Flags().StringVar(&upStore, "store", "", "Choose hosted hev layer or turbopuffer (advanced BYO); with no key, traces are kept in files")
	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
}

// hostOS is runtime.GOOS, held in a variable so the launchd half of up and
// down is decided in one place and can be tested from any platform.
var hostOS = runtime.GOOS

// portFree is local.PortFree, replaceable where a test stands a stub in for
// the gateway on the port `up` is about to check.
var portFree = local.PortFree

func hasLaunchd() bool { return hostOS == "darwin" }

func tick(out io.Writer, format string, args ...any) {
	fmt.Fprintf(out, "  ✓ "+format+"\n", args...)
}

func localStack(cfg *daemon.Config, home, apiKey string) local.Stack {
	namespace := cfg.LayerNamespace
	if namespace == "" {
		namespace = "hev-traces"
	}
	return local.Stack{
		Image: cfg.Local.Image, Port: cfg.Local.Port, Project: cfg.Local.Project,
		KitImage: cfg.Local.KitImage, ServePort: cfg.Local.ServePort,
		Namespace: namespace, APIKey: apiKey,
		Dir: filepath.Join(home, ".hev", "local"),
	}
}

// errNoKey is the whole of what `hev up` prints when the Turbopuffer lane is
// chosen and there is no key for it.
var errNoKey = fmt.Errorf("the Turbopuffer lane needs a Turbopuffer API key: create one at https://turbopuffer.com/dashboard, `export HEVLAYER_API_KEY=<provider-key>`, and run `hev up --store turbopuffer` again, or run `hev up` with no key to keep traces in files")

// upFiles is the lane with no Layer API key: traces in files, no Docker.
const upFiles = "files"

// errRetiredStore answers an explicit choice of the retired local store.
var errRetiredStore = fmt.Errorf("--store %s: the local Docker lane is retired; run `hev up` with no key to keep traces in files on this machine, or set HEVLAYER_API_KEY for hev layer", daemon.RetiredStore)

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
	if store == daemon.RetiredStore {
		return "", "", errRetiredStore
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
	case layer.StoreTurbopuffer:
		if apiKey == "" {
			return "", "", errNoKey
		}
		return store, apiKey, nil
	case upFiles:
		return store, "", nil
	}
	return "", "", fmt.Errorf("--store %s: expected hosted or %s (with no key, `hev up` keeps traces in files)", store, layer.StoreTurbopuffer)
}

// runUp performs the steps of RFC 0006 in order. Each is a precondition of the
// next, and each first asks whether it is already done.
func runUp(ctx context.Context, out io.Writer, noDashboard bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	// 0. A config that names a hosted Layer is refused, and a Turbopuffer lane
	// with no key is reported, before anything is started, pulled or written —
	// and before the config is loaded in full, because loading shells out
	// (`gh`, for the capture identity) and what another tool writes under HOME
	// is still a write on the refusal path.
	// Hosted selection precedes all local Docker checks.
	layerenv.WarnDeprecated(out)
	store, apiKey, err := upLane(upStore)
	if err != nil {
		return err
	}
	if store == "hosted" {
		cfg, err := daemon.LoadConfig()
		if err != nil {
			return err
		}
		return runHostedUp(ctx, out, home, cfg, apiKey, noDashboard)
	}
	if store == upFiles {
		return runFilesUp(out, home)
	}
	explicitMove := upStore != "" || layerenv.Get("HEVLAYER_STORE") != ""
	if err := daemon.CheckLocalConfig(explicitMove); err != nil {
		return err
	}
	cfg, err := daemon.LoadConfig()
	if err != nil {
		return err
	}
	stack := localStack(cfg, home, apiKey)
	if strings.EqualFold(cfg.LayerStore, daemon.RetiredStore) {
		// Moving off the retired lane: say so, and leave its containers.
		fmt.Fprintf(out, "%s\n\n", daemon.RetiredMessage(cfg))
		stack.KeepOrphans = true
	}

	// 1. Docker.
	if err := local.Preflight(ctx); err != nil {
		return err
	}
	tick(out, "docker running")

	// An older kit served the dashboard from launchd on the same port. On a
	// config that kit wrote, that job is retired before the port is checked.
	if cfg.Local.Managed && hasLaunchd() {
		legacy := launchdJob{Label: serveLabel()}
		if wasLoaded, err := legacy.bootout(); err != nil {
			return err
		} else if wasLoaded {
			os.Remove(legacy.plistPath(home))
			tick(out, "retired the launchd read side (%s); the dashboard runs in Docker now", legacy.Label)
		}
	}

	// 2. The gateway and the dashboard, pinned, healthy before anything
	// depends on them.
	services := stack.Services()
	if noDashboard {
		services = services[:len(services)-1]
	}
	before := stack.Containers(ctx)
	if before["gateway"] == "" && !portFree(stack.Port) {
		return fmt.Errorf("port %d is in use: set HEV_LOCAL_PORT=<free port> (or [local] port in %s) and run `hev up` again", stack.Port, cfg.ConfigPath)
	}
	if !noDashboard && before["dashboard"] == "" && !portFree(stack.ServePort) {
		return fmt.Errorf("port %d is in use: set HEV_LOCAL_SERVE_PORT=<free port> (or [local] serve_port in %s) and run `hev up` again", stack.ServePort, cfg.ConfigPath)
	}
	if err := stack.Up(ctx, services...); err != nil {
		return err
	}
	after := stack.Containers(ctx)
	gateway := layer.New(stack.Endpoint(), apiKey, "", "")
	if err := waitFor(ctx, 60*time.Second, func() error { return gatewayHealthy(gateway) }); err != nil {
		return fmt.Errorf("gateway on :%d: %w", stack.Port, err)
	}
	health, err := gateway.Health()
	if err != nil {
		return err
	}
	if err := local.CheckPin(stack.Image, health.Version); err != nil {
		return err
	}
	tick(out, "%s %s on :%d", local.ShortImage(stack.Image), state(before, after, "gateway"), stack.Port)
	edition := upEdition(out, gateway)

	// 3. The store: the key, through the gateway, so that a typo fails here
	// and not in the daemon's log five minutes from now.
	if err := checkKey(ctx, stack.Endpoint(), apiKey, stack.Namespace); err != nil {
		return err
	}
	tick(out, "turbopuffer key accepted, archiving to namespace %s", stack.Namespace)

	// 4. The config file, which is what the capture service will read.
	unlock := func() {}
	if !localEndpoint(cfg.LayerEndpoint) && cfg.LayerEndpoint != "" {
		unlock, err = stopArchiveWriter(ctx, home)
		if err != nil {
			return err
		}
	}
	w, err := daemon.WriteLocalConfig(cfg.Local, store, apiKey, explicitMove)
	if err == nil && w.Migrated && cfg.LayerEndpoint != "" && !localEndpoint(cfg.LayerEndpoint) {
		err = index.ResetState()
	}
	unlock()
	if err != nil {
		return err
	}
	configPath, err := filepath.Abs(w.Path)
	if err != nil {
		return err
	}
	for env, saved := range map[string]string{"HEVLAYER_ENDPOINT": stack.Endpoint(), "HEVLAYER_API_KEY": apiKey, "HEVLAYER_NAMESPACE": stack.Namespace, "HEVLAYER_STORE": store} {
		if value := layerenv.Get(env); value != "" && value != saved {
			fmt.Fprintf(out, "  ! %s is exported in this shell and overrides %s for commands run here\n", env, configPath)
		}
	}

	// 5. The daemon, under the user service manager so capture continues after
	// this terminal closes. It is installed only now: its first scan fires at
	// load.
	daemonReady, err := upCapture(out, home, configPath, w)
	if err != nil {
		return err
	}

	// 6. The agent skills, so Claude Code and Codex search the archive
	// instead of their own history.
	if harnesses, err := installSkills(home); err != nil {
		return fmt.Errorf("install agent skills: %w", err)
	} else if len(harnesses) > 0 {
		tick(out, "bundled skills checked for %s", strings.Join(harnesses, ", "))
	}

	// 7. The dashboard, reachable from the host.
	lead := hevdLine{value: "hevd is up: sending traces and taking dispatched work on this machine."}
	if !daemonReady {
		lead.value = "the stack is up. run `hev d` to capture."
	}
	search := hevdLine{"search", `hev traces search "why did the preflight fail"`}
	dispatch := hevdLine{"dispatch", "hev dispatch ls"}
	archive := archiveLine(stack.Endpoint(), stack.Namespace)
	stop := hevdLine{"stop", "hev down"}
	if noDashboard {
		printHevd(out, lead, search, dispatch, archive, edition, stop)
		return nil
	}
	url := stack.DashboardURL()
	if err := waitFor(ctx, 30*time.Second, func() error { return reachable(url) }); err != nil {
		return fmt.Errorf("dashboard did not answer on %s (see `docker compose -p %s logs dashboard`): %w", url, stack.Project, err)
	}
	tick(out, "dashboard %s %s", state(before, after, "dashboard"), url)
	printHevd(out, lead, hevdLine{"dashboard", url}, search, dispatch, archive, edition, stop)
	return nil
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
// capture daemon keeps traces in files, and nothing runs in Docker. A config
// the retired local Docker lane wrote gets the retirement message and is
// moved to files; its containers and volume are never touched.
func runFilesUp(out io.Writer, home string) error {
	cfg, err := daemon.LoadConfig()
	if err != nil {
		return err
	}
	if strings.EqualFold(cfg.LayerStore, daemon.RetiredStore) {
		fmt.Fprintf(out, "%s\n\n", daemon.RetiredMessage(cfg))
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
	tick(out, "no Layer API key: traces are kept in files under %s", target.Files.Dir())
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
	lead := hevdLine{value: "hevd is up: capturing traces into files and taking dispatched work on this machine."}
	if !daemonReady {
		lead.value = "the config is written. run `hev d` to capture."
	}
	printHevd(out, lead,
		hevdLine{"search", `hev traces search "why did the preflight fail"`},
		hevdLine{"dashboard", "hev serve"},
		hevdLine{"dispatch", "hev dispatch ls"},
		hevdLine{"archive", target.Files.Dir()},
		hevdLine{"stop", "hev down"})
	return nil
}

// upEdition reads the gateway's license for the summary. A license about to
// lapse is a line of its own, printed whether or not out is a terminal; an
// edition the gateway will not state leaves the summary line unknown rather
// than failing `up`.
func upEdition(out io.Writer, gateway *layer.Client) hevdLine {
	lic, err := gateway.License()
	if err != nil {
		return hevdLine{"edition", "unknown"}
	}
	if w := lic.Warning(); w != "" {
		fmt.Fprintf(out, "  ! %s\n", w)
	}
	if lic.Community() {
		return hevdLine{"edition", lic.Edition() + " · pro: hev pro"}
	}
	return hevdLine{"edition", lic.Edition()}
}

// state words a service's line by whether `up` created its container.
func state(before, after map[string]string, service string) string {
	if before[service] != "" && before[service] == after[service] {
		return "already running"
	}
	return "running"
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

// checkKey asks the gateway for a namespace's metadata with the key. A
// namespace that does not exist yet is an answer from Turbopuffer, and so an
// accepted key; only an authorization failure is a rejected one.
func checkKey(ctx context.Context, endpoint, key, namespace string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v2/namespaces/"+namespace+"/query",
		bytes.NewReader([]byte(`{"rank_by":["id","asc"],"top_k":1}`)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("check the Turbopuffer key: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode < 300, resp.StatusCode == http.StatusNotFound:
		return nil
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("Turbopuffer rejected the key (HTTP %d): check HEVLAYER_API_KEY and run `hev up` again", resp.StatusCode)
	}
	var body struct {
		Error string `json:"error"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if json.Unmarshal(raw, &body) != nil || body.Error == "" {
		body.Error = strings.TrimSpace(string(raw))
	}
	return fmt.Errorf("check the Turbopuffer key through the gateway: HTTP %d: %s", resp.StatusCode, body.Error)
}

// runDown is up's mirror. On the Turbopuffer lane the containers stop first
// and the jobs are unloaded second; the daemon is KeepAlive, so it is unloaded
// rather than signalled. Nothing is deleted: the archive is in files, in hev
// layer or in Turbopuffer, and the index state that describes it stays true.
// Only the Turbopuffer lane runs containers, so only it touches Docker; a
// config the retired local lane wrote gets the retirement message and its
// containers and volume are left alone.
func runDown(ctx context.Context, out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfg, err := daemon.LoadConfig()
	if err != nil {
		return err
	}
	stack := localStack(cfg, home, "")
	// Only a lane with a key runs containers. With no key the archive is
	// files; a config the retired local lane wrote is told so, and its
	// containers and volume are left alone.
	retired := strings.EqualFold(cfg.LayerStore, daemon.RetiredStore)
	keyed := true
	if mode, err := storagemode.Resolve(storagemode.Options{ConfigKey: true, ConfigPath: cfg.ConfigPath, Peek: true}); err == nil && mode.Mode == storagemode.Filesystem {
		keyed = false
	}
	docker := cfg.LayerStore != "hosted" && !retired && (keyed || cfg.LayerStore == layer.StoreTurbopuffer)
	if retired {
		fmt.Fprintf(out, "%s\n\n", daemon.RetiredMessage(cfg))
	}

	// A config `up` never wrote: the jobs on this machine belong to something
	// else — a hosted daemon, a deploy's read side — and are not down's to
	// touch. Containers are addressed by project name, so stopping those is
	// still safe.
	if !cfg.Local.Managed && cfg.LayerStore != "hosted" && !docker {
		fmt.Fprintf(out, "  • %s has no [local] block: not written by `hev up`, so launchd jobs are left alone\n", cfg.ConfigPath)
		return nil
	}
	if !cfg.Local.Managed && cfg.LayerStore != "hosted" {
		if err := local.Preflight(ctx); err != nil {
			return err
		}
		if err := stack.Down(ctx); err != nil {
			return err
		}
		tick(out, "containers stopped")
		fmt.Fprintf(out, "  • %s has no [local] block: not written by `hev up`, so launchd jobs are left alone\n", cfg.ConfigPath)
		return nil
	}

	// Containers first, jobs second. Without Docker the containers cannot be
	// stopped, but the jobs up installed still can: do that, and fail saying
	// what is left.
	var dockerErr error
	if docker {
		dockerErr = local.Preflight(ctx)
	}
	if dockerErr == nil && docker {
		if err := stack.Down(ctx); err != nil {
			return err
		}
		tick(out, "containers stopped")
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
	} else if !hasLaunchd() {
		fmt.Fprintf(out, "  • no launchd on %s: no jobs to unload; stop `hev d` yourself\n", hostOS)
	} else if err := unloadJobs(out, home); err != nil {
		return err
	}
	if dockerErr != nil {
		hint := strings.ReplaceAll(dockerErr.Error(), "`hev up`", "`hev down`")
		return fmt.Errorf("jobs unloaded, but the %s containers were left as they are — %s", stack.Project, hint)
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

func gatewayHealthy(c *layer.Client) error {
	_, err := c.Health()
	return err
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
