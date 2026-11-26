package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/cron"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/feed"
	"github.com/hev/kit/internal/harness"
	"github.com/hev/kit/internal/hygiene"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/kitcapture"
	"github.com/hev/kit/internal/license"
	"github.com/hev/kit/internal/metrics"
	"github.com/hev/kit/internal/peers"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/services"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/pkg/kitapi"
)

type Status struct {
	Redactions       redact.Counts  `json:"redactions,omitempty"`
	LastRun          time.Time      `json:"last_run"`
	UnitsIndexed     int            `json:"units_indexed"`
	MigrationPending bool           `json:"migration_pending,omitempty"`
	LastError        string         `json:"last_error,omitempty"`
	Targets          []TargetStatus `json:"targets,omitempty"`
	Notice           string         `json:"notice,omitempty"`
}

func StatusPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".hev-daemon-status.json"
	}
	return filepath.Join(home, ".hev", "daemon-status.json")
}

func ReadStatus() (Status, error) {
	var s Status
	b, err := os.ReadFile(StatusPath())
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	return s, err
}

func writeStatus(s Status) error {
	if err := platformops.MkdirAll(filepath.Dir(StatusPath()), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return platformops.WriteFileAtomic(StatusPath(), b, 0o600)
}

// AfterCycle runs between scan cycles, never during one: the host's
// scheduled self-update (cmd/hev sets it). True stops the daemon with
// ErrRestart so it can re-execute onto the build it switched to.
var AfterCycle func(ctx context.Context, logger *slog.Logger) (restart bool)

// OnStart runs before the first cycle. True stops the daemon with ErrRestart:
// a self-update that kept failing to start was rolled back.
var OnStart func(ctx context.Context, logger *slog.Logger) (restart bool)

// ErrRestart asks the caller to re-execute the daemon from its installed path.
var ErrRestart = errors.New("hevd: restarting onto another build")

// Run continuously indexes local transcripts until interrupted.
func Run(ctx context.Context) error {
	if err := platformops.RequireTraceWrites(); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	if err := platformops.MkdirAll(filepath.Join(home, ".hev"), 0o755); err != nil {
		return err
	}
	logFile, err := platformops.OpenFile(filepath.Join(home, ".hev", "hev.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()
	logger := slog.New(slog.NewJSONHandler(logFile, nil))
	if OnStart != nil && OnStart(ctx, logger) {
		return ErrRestart
	}

	configuredCron, err := cron.Load(DefaultConfigPath())
	if err != nil {
		return err
	}
	configuredHygiene, err := hygiene.Load(DefaultConfigPath())
	if err != nil {
		return err
	}
	configuredMetrics, err := metrics.Load(DefaultConfigPath())
	if err != nil {
		return err
	}
	configuredServices, err := services.Load(DefaultConfigPath())
	if err != nil {
		return err
	}
	cfg, err := loadConfig(true)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if cfg.ScanInterval <= 0 {
		return fmt.Errorf("scan interval must be positive")
	}
	// A local file lock prevents overlapping writers without opening a port.
	release, err := platformops.LockPath(filepath.Join(home, ".hev", "daemon.lock"), true)
	if err != nil {
		return fmt.Errorf("another daemon is already running: %w", err)
	}
	defer release()
	primary, primaryErr := openPrimary(cfg, logger)
	if primaryErr != nil && len(cfg.captureTargets) == 0 {
		return primaryErr
	}
	cache, clusterSvc, membership, stopSidecars := startSecretSidecars(ctx, logger)
	defer stopSidecars()
	licenseCtx, stopLicense := context.WithCancel(ctx)
	defer stopLicense()
	go license.Refresh(licenseCtx)

	// The primary keeps the storage record used by existing CLI readers.
	defer removeStorageRecord()
	if primary.Mode != "" {
		recordPrimary(primary, logger)
	}
	ctx, closeControl, err := platformops.DaemonContext(ctx)
	if err != nil {
		return err
	}
	defer closeControl()

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	defer startOptionalPeers(ctx, cfg.ConfigPath, logger)()
	cronRunner := cron.Runner{
		Home: home, Membership: membership, Logger: logger,
		Resolve: func(ctx context.Context, refs []secrets.Ref) (map[string]string, error) {
			return cache.ResolveAs(ctx, "hevd cron", refs)
		},
		Notice: func(ctx context.Context, notice attention.Notice) error {
			return attention.Default().Emit(ctx, notice, "")
		},
	}
	defer cronRunner.Start(ctx, configuredCron)()
	hygieneRunner := hygiene.Runner{Home: home, Logger: logger}
	defer hygieneRunner.Start(ctx, configuredHygiene)()
	serviceSupervisor := services.Supervisor{
		Home: home,
		Membership: services.Membership{
			Self:    membership.Self,
			Enabled: membership.Enabled,
			Members: membership.Members,
		},
		Logger: logger,
		Resolve: func(ctx context.Context, refs []secrets.Ref) (map[string]string, error) {
			return cache.ResolveAs(ctx, "hevd service", refs)
		},
		Notice: func(ctx context.Context, notice attention.Notice) error {
			return attention.Default().Emit(ctx, notice, "")
		},
	}
	stopServices, err := serviceSupervisor.Start(ctx, configuredServices)
	if err != nil {
		return err
	}
	defer stopServices()
	defer startMetrics(ctx, configuredMetrics, cache, clusterSvc, membership, cronRunner, configuredCron, logger)()
	return runCapture(ctx, cfg, home, primary, primaryErr, cache.ResolveAs, logger)
}

// firstScanGate is nil unless `hev up` manages the stack: a hosted daemon has
// no gateway of its own to wait for, and scans at once as it always has.
func firstScanGate(cfg *Config, healthy func() error) func(context.Context) error {
	if !cfg.Local.Managed {
		return nil
	}
	return func(ctx context.Context) error {
		return waitHealthy(ctx, healthy, time.Second, firstScanGateTimeout)
	}
}

// firstScanGateTimeout bounds how long a managed daemon holds its first scan
// for the gateway. Past it the scan runs anyway: a failed cycle is recorded
// and retried, which beats a daemon that never reports.
const firstScanGateTimeout = 2 * time.Minute

// scanLoop runs a cycle immediately and then on every tick. When kit owns the
// local stack the first cycle waits on gate: launchd starts the job at login,
// the containers may still be coming up, and a first scan that loses that race
// is not retried for a whole scan interval.
func scanLoop(ctx context.Context, gate func(context.Context) error, run func(), interval time.Duration, logger *slog.Logger) error {
	if gate != nil {
		if err := gate(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			logger.Warn("gateway not healthy; scanning anyway", "error", captureError("gateway health check", err))
		}
	}
	run()
	attention.Flush(ctx)
	feed.Wake()
	if AfterCycle != nil && AfterCycle(ctx, logger) {
		return ErrRestart
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// A tick and cancellation can be ready together, and select
			// picks either; a cancelled loop must not start another scan.
			if ctx.Err() != nil {
				return nil
			}
			run()
			// Retry Slack notices nothing else woke for (https://hevkit.com/docs/notify).
			attention.Flush(ctx)
			feed.Wake()
			if AfterCycle != nil && AfterCycle(ctx, logger) {
				return ErrRestart
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// waitHealthy polls healthy until it succeeds, ctx ends, or timeout passes.
func waitHealthy(ctx context.Context, healthy func() error, every, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		err := healthy()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("gateway not healthy after %s: %w", timeout, err)
		case <-time.After(every):
		}
	}
}

// kitNotice is what `hev d status` says about hosted capture.
func kitNotice(d *kitcapture.Drainer, o *kitcapture.Outbox) string {
	pending, _ := o.Pending()
	_, err := d.Last()
	var notes []string
	switch {
	case kitapi.ReadOnly(err):
		notes = append(notes, kitcapture.BillingNotice(len(pending)))
	case errors.Is(err, kitcapture.ErrTeamMismatch), errors.Is(err, kitcapture.ErrNoTeam):
		notes = append(notes, fmt.Sprintf("kit outbox not sent (%v): %d batches queued", err, len(pending)))
	case err != nil:
		notes = append(notes, fmt.Sprintf("hevmind.com service API unavailable (%v): %d batches queued in %s", err, len(pending), o.Dir()))
	case len(pending) > 0:
		notes = append(notes, fmt.Sprintf("capturing to the hevmind.com service API: %d batches queued", len(pending)))
	default:
		notes = append(notes, "capturing to the hevmind.com service API")
	}
	if n, at := o.Refused(); n > 0 {
		notes = append(notes, fmt.Sprintf("outbox full: %d batches not queued (last %s); their rows are in the local archive only", n, at.UTC().Format(time.RFC3339)))
	}
	if unbound, _ := o.Unbound(); len(unbound) > 0 {
		notes = append(notes, fmt.Sprintf("%d batches queued before outboxes were bound to a team are not sent (no record of their team); they are in the local archive", len(unbound)))
	}
	return strings.Join(notes, "; ")
}

// validateLayerTarget checks a Layer target has what it needs. The file
// archive (store "files", or no key at all) needs none of it.
func validateLayerTarget(cfg *Config) error {
	if strings.EqualFold(strings.TrimSpace(cfg.LayerStore), "files") {
		return nil
	}
	if cfg.LayerEndpoint == "" || cfg.LayerAPIKey == "" || cfg.LayerNamespace == "" {
		if len(cfg.Buckets) > 0 || cfg.ActiveBucket != "" {
			return fmt.Errorf("legacy S3-only config is no longer supported; run `hev init` to configure [layer] endpoint, api_key, and namespace (use `hev migrate` for an existing bucket archive)")
		}
		return fmt.Errorf("Layer target is not configured; run `hev init`")
	}
	return nil
}

// instructionWorkdirs lets instruction capture re-read only the sessions that
// changed since the last cycle instead of every session on every cycle.
var instructionWorkdirs = &trace.WorkdirCache{}

func runIndexCycle(client index.Archive, state *index.State, logger *slog.Logger) Status {
	return runIndexCycleImages(client, state, logger, nil)
}

// runIndexCycleImages is one scan with this host's image store; nil stores
// no image bytes.
func runIndexCycleImages(client index.Archive, state *index.State, logger *slog.Logger, images *trace.ImageCapture) Status {
	return indexCapture(client, state, logger, index.Options{Images: images}, instructionWorkdirs, nil)
}

func indexCapture(client index.Archive, state *index.State, logger *slog.Logger, opts index.Options, workdirs *trace.WorkdirCache, additionalSources []trace.Source) Status {
	s := Status{LastRun: time.Now().UTC(), Redactions: redact.Counts{}}
	// The same sources `hev index` walks.
	piRoot, openCodeRoot, agyRoot := trace.DefaultPiRoot(), trace.DefaultOpenCodeRoot(), trace.DefaultAgyRoot()
	cfg, cfgErr := LoadConfig()
	if cfgErr == nil {
		piRoot, openCodeRoot, agyRoot = cfg.PiSessionsDir, cfg.OpenCodeDataDir, cfg.AgyDataDir
	}
	sources := trace.Sources(map[string]string{harness.Pi: piRoot, harness.OpenCode: openCodeRoot, harness.Agy: agyRoot})
	sources = trace.MergeSources(sources, additionalSources)
	failures := 0
	add := func(rep *index.Report) {
		if rep == nil {
			return
		}
		s.Redactions.Add(rep.Redactions)
		s.UnitsIndexed += rep.UnitsIndexed
		failures += len(rep.Errors)
	}
	record := func(rep *index.Report, err error) {
		if err != nil {
			failures++
		}
		add(rep)
		if rep != nil && rep.MigrationPending {
			s.MigrationPending = true
		}
	}
	if _, hosted := client.(*kitcapture.Archive); hosted {
		record(index.RunNewest(sources, client, state, opts))
	} else {
		for _, src := range sources {
			if opts.Context != nil && opts.Context.Err() != nil {
				break
			}
			record(index.Run(src, client, state, opts))
		}
	}
	if s.MigrationPending {
		logger.Warn("historical redaction migration pending; review archive rebuild scope before running hev index")
	}

	// Scoped targets capture only transcripts. Host-wide dispatch and
	// instruction records have no equivalent transcript workspace boundary.
	if opts.FilterTurns == nil && (opts.Context == nil || opts.Context.Err() == nil) {
		if spans, ok := client.(index.SpanWriter); ok {
			rep, err := index.RunTraceSpans(dispatch.Root(), spans, state, false)
			if err != nil {
				failures++
			}
			add(rep)
		}
		if cfgErr != nil {
			failures++
		} else if cfg.CaptureInstructions && cfg.CaptureInstructionsExplicit {
			rep, err := index.RunInstructions(trace.InstructionSource{Sessions: sources, Workdirs: workdirs}, client)
			if err != nil {
				failures++
			}
			add(rep)
		}
	}
	if failures > 0 {
		s.LastError = fmt.Sprintf("indexing failed in %d operations or units", failures)
	}
	// Save successful unit signatures even when another unit failed.
	if err := state.Save(); err != nil {
		if s.LastError != "" {
			s.LastError += ". "
		}
		s.LastError += captureError("save index state", err)
	}
	logger.Info("index cycle", "last_run", s.LastRun, "units_indexed", s.UnitsIndexed, "last_error", s.LastError)
	return s
}

// startOptionalPeers never prevents capture when the optional message endpoint fails.
func startOptionalPeers(ctx context.Context, path string, logger *slog.Logger) func() {
	cfg, err := peers.LoadNetworkConfig(path)
	if err == nil {
		var close func()
		close, err = peers.StartListener(ctx, cfg)
		if err == nil {
			return close
		}
	}
	logger.Warn("peers endpoint disabled; capture continues", "listen", cfg.Listen, "err", err)
	return func() {}
}

// The optional secrets sidecar must never prevent trace capture from starting.
func startSecrets(ctx context.Context, logger *slog.Logger) func() {
	_, _, _, stop := startSecretSidecars(ctx, logger)
	return stop
}

func startSecretSidecars(ctx context.Context, logger *slog.Logger) (*secrets.Cache, *cluster.Service, cron.Membership, func()) {
	cache := secrets.New()
	closeSecrets, err := secrets.Serve(ctx, cache)
	clusterSvc, membership, closeCluster := startCluster(ctx, cache, logger)
	if err != nil {
		logger.Warn("secrets endpoint unavailable; direct resolution remains available")
		return cache, clusterSvc, membership, closeCluster
	}
	return cache, clusterSvc, membership, func() { closeCluster(); closeSecrets() }
}
