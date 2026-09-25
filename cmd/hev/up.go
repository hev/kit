package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/spf13/cobra"
)

var upNoDashboard bool

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the gateway and dashboard in Docker, and the capture daemon",
	Long: `Start everything kit needs on this machine and leave the archive filling.

  export TURBOPUFFER_API_KEY=tpuf_...
  hev up
  hev query "why did the preflight fail"

up runs the hev layer gateway (community edition) and the kit dashboard in
Docker, in front of your Turbopuffer account, points the config at them, and
installs the capture daemon under launchd and the hev-query skill for
Claude Code and Codex. Your transcripts land in a
namespace in your own Turbopuffer account (hev-traces by default).

The key is read from TURBOPUFFER_API_KEY and kept in the config, so a second
` + "`hev up`" + ` needs nothing exported. Run again, it reports what is already
running and changes nothing. Image pins, host ports and the Compose project
name live in the [local] block of the config. ` + "`hev init`" + ` remains the way to
use a hosted Layer instead.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runUp(cmd.Context(), cmd.OutOrStdout(), upNoDashboard)
	},
}

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the containers and unload the daemon",
	Long: `Stop the gateway and dashboard containers and unload the capture daemon.

The archive is in your Turbopuffer account and is left exactly as it is.`,
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
	upCmd.Flags().BoolVar(&upNoDashboard, "no-dashboard", false, "Run the gateway and daemon only")
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
	return local.Stack{
		Image: cfg.Local.Image, Port: cfg.Local.Port, Project: cfg.Local.Project,
		KitImage: cfg.Local.KitImage, ServePort: cfg.Local.ServePort,
		Namespace: daemon.LocalNamespace(cfg.LayerNamespace, cfg.LayerStore),
		APIKey:    apiKey,
		Dir:       filepath.Join(home, ".hev", "local"),
	}
}

// errNoKey is the whole of what a first `hev up` without a key prints.
var errNoKey = fmt.Errorf("hev up needs a Turbopuffer API key: create one at https://turbopuffer.com/dashboard, `export TURBOPUFFER_API_KEY=tpuf_...`, and run `hev up` again")

// upAPIKey is the key the stack runs with: the one exported in this shell,
// which also replaces a stored key, else the one a previous `up` stored.
func upAPIKey() (string, error) {
	if v := strings.TrimSpace(os.Getenv("TURBOPUFFER_API_KEY")); v != "" {
		return v, nil
	}
	if v := daemon.LocalAPIKey(); v != "" {
		return v, nil
	}
	return "", errNoKey
}

// runUp performs the steps of RFC 0006 in order. Each is a precondition of the
// next, and each first asks whether it is already done.
func runUp(ctx context.Context, out io.Writer, noDashboard bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	// 0. A config that names a hosted Layer is refused, and a missing key is
	// reported, before anything is started, pulled or written — and before the
	// config is loaded in full, because loading shells out (`gh`, for the
	// capture identity) and what another tool writes under HOME is still a
	// write on the refusal path.
	if err := daemon.CheckLocalConfig(); err != nil {
		return err
	}
	apiKey, err := upAPIKey()
	if err != nil {
		return err
	}
	cfg, err := daemon.LoadConfig()
	if err != nil {
		return err
	}
	stack := localStack(cfg, home, apiKey)

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
	services := local.Services
	if noDashboard {
		services = []string{"gateway"}
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

	// 3. The key, through the gateway, so that a typo fails here and not in
	// the daemon's log five minutes from now.
	if err := checkKey(ctx, stack.Endpoint(), apiKey, stack.Namespace); err != nil {
		return err
	}
	tick(out, "turbopuffer key accepted, archiving to namespace %s", stack.Namespace)

	// 4. The config file, which is what launchd will read.
	w, err := daemon.WriteLocalConfig(cfg.Local, apiKey)
	if err != nil {
		return err
	}
	configPath, err := filepath.Abs(w.Path)
	if err != nil {
		return err
	}
	for _, env := range []string{"LAYER_ENDPOINT", "LAYER_API_KEY", "LAYER_NAMESPACE", "LAYER_STORE"} {
		if os.Getenv(env) != "" {
			fmt.Fprintf(out, "  ! %s is exported in this shell and overrides %s for commands run here\n", env, configPath)
		}
	}

	// 5. The daemon, under launchd so traces keep filling after this terminal
	// closes. It is installed only now: its first scan fires at load.
	if !hasLaunchd() {
		fmt.Fprintf(out, "  • no launchd on %s: run `hev d` to capture\n", hostOS)
	} else if err := upDaemon(out, home, configPath, w); err != nil {
		return err
	}

	// 6. The agent skills, so Claude Code and Codex search the archive
	// instead of their own history.
	if harnesses, err := installSkills(home); err != nil {
		return fmt.Errorf("install agent skills: %w", err)
	} else if len(harnesses) > 0 {
		tick(out, "hev-query skill installed for %s", strings.Join(harnesses, ", "))
	}

	// 7. The dashboard, reachable from the host.
	lead := hevdLine{value: "hevd is up, capturing sessions on this machine."}
	if !hasLaunchd() {
		lead.value = "the stack is up. run `hev d` to capture."
	}
	search := hevdLine{"search", `hev query "why did the preflight fail"`}
	archive := hevdLine{"archive", pufferMark + " turbopuffer · " + stack.Namespace}
	stop := hevdLine{"stop", "hev down"}
	if noDashboard {
		printHevd(out, lead, search, archive, edition, stop)
		return nil
	}
	url := stack.DashboardURL()
	if err := waitFor(ctx, 30*time.Second, func() error { return reachable(url) }); err != nil {
		return fmt.Errorf("dashboard did not answer on %s (see `docker compose -p %s logs dashboard`): %w", url, stack.Project, err)
	}
	tick(out, "dashboard %s %s", state(before, after, "dashboard"), url)
	printHevd(out, lead, hevdLine{"dashboard", url}, search, archive, edition, stop)
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
// has changed. A config moved off the Postgres-era local store also forgets
// its index state, with the daemon stopped: those units were indexed into an
// archive the config no longer names.
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
		tick(out, "moved off the local Postgres store: every transcript will be indexed again")
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
		return fmt.Errorf("Turbopuffer rejected the key (HTTP %d): check TURBOPUFFER_API_KEY and run `hev up` again", resp.StatusCode)
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

// runDown is up's mirror. The containers stop first and the jobs are unloaded
// second; the daemon is KeepAlive, so it is unloaded rather than signalled.
// Nothing is deleted: the archive is in Turbopuffer, and the index state that
// describes it stays true.
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

	// A config `up` never wrote: the jobs on this machine belong to something
	// else — a hosted daemon, a deploy's read side — and are not down's to
	// touch. Containers are addressed by project name, so stopping those is
	// still safe.
	if !cfg.Local.Managed {
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
	dockerErr := local.Preflight(ctx)
	if dockerErr == nil {
		if err := stack.Down(ctx); err != nil {
			return err
		}
		tick(out, "containers stopped")
	}
	if !hasLaunchd() {
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
