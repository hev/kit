package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hev/kit/internal/dispatch"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/peers"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/trace"
)

type Status struct {
	Redactions       redact.Counts `json:"redactions,omitempty"`
	LastRun          time.Time     `json:"last_run"`
	UnitsIndexed     int           `json:"units_indexed"`
	MigrationPending bool          `json:"migration_pending,omitempty"`
	LastError        string        `json:"last_error,omitempty"`
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

	defer startSecrets(ctx, logger)()

	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	// hevd reads its Layer key reference (op:// or azkv://) before opening the target, so a
	// rate-limit answer keeps the daemon up (serving the secrets endpoint)
	// until the reset instead of exiting into a supervisor restart loop.
	_, keyRef := layerenv.KeyRef()
	if k := strings.TrimSpace(cfg.LayerAPIKey); (k == "" || k == layerenv.PlaceholderKey) && keyRef != "" {
		// A stop signal must end the wait cleanly, closing the endpoint.
		waitCtx, stopWait := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		key, err := layerKeyAfterCooldown(waitCtx, logger)
		stopWait()
		if err != nil {
			return err
		}
		cfg.LayerAPIKey = key
	}
	if cfg.ScanInterval <= 0 {
		return fmt.Errorf("scan interval must be positive")
	}
	target, err := OpenTarget(cfg, "")
	if err != nil {
		return err
	}
	if target.Retired {
		logger.Warn("local Docker lane retired", "message", RetiredMessage(cfg))
	}
	client := target.Layer
	if target.Files == nil {
		cfg.LayerAPIKey, cfg.LayerEndpoint = client.APIKey, client.Endpoint
		if err := validateLayerTarget(cfg); err != nil {
			return err
		}
	} else {
		logger.Info("no Layer API key: keeping traces in files", "archive", target.Files.Dir())
	}

	// A local file lock prevents overlapping writers without opening a port.
	release, err := platformops.LockPath(filepath.Join(home, ".hev", "daemon.lock"), true)
	if err != nil {
		return fmt.Errorf("another daemon is already running: %w", err)
	}
	defer release()
	ctx, closeControl, err := platformops.DaemonContext(ctx)
	if err != nil {
		return err
	}
	defer closeControl()
	state := index.LoadState()

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	defer startOptionalPeers(ctx, cfg.ConfigPath, logger)()
	outcomeCursor := ""
	archive := target.Archive()
	run := func() {
		if err := dispatch.SweepQueueBackstop(); err != nil {
			logger.Warn("dispatch queue sweep", "err", err)
		}
		if client != nil {
			if err := client.ReconcileQueryFilters(); err != nil {
				logger.Error("reconcile archive query filters", "err", err)
				return // Retry before ingestion on the next scan.
			}
		}
		s := runIndexCycleImages(archive, state, logger, target.Images)
		if target.Files != nil {
			// Fold this scan into the search index now, so the next search
			// does not pay for it.
			if err := target.Files.Refresh(); err != nil {
				logger.Error("refresh archive search index", "err", err)
			}
		}
		sweepCtx, sweepCancel := context.WithTimeout(ctx, 45*time.Second)
		next, updated, sweepErr := index.SweepOutcomes(sweepCtx, archive, index.NewOutcomeEnricher(), outcomeCursor, time.Now().Add(-60*24*time.Hour).UnixMilli(), 10)
		sweepCancel()
		outcomeCursor = next
		if sweepErr != nil {
			logger.Warn("outcome sweep", "err", sweepErr)
		} else if updated > 0 {
			logger.Info("outcome sweep", "updated", updated)
		}
		if err := writeStatus(s); err != nil {
			logger.Error("write status", "err", err)
		}
	}
	var gate func(context.Context) error
	if client != nil {
		gate = firstScanGate(cfg, func() error { _, err := client.Health(); return err })
	}
	return scanLoop(ctx, gate, run, cfg.ScanInterval, logger)
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
			logger.Warn("gateway not healthy; scanning anyway", "err", err)
		}
	}
	run()
	if AfterCycle != nil && AfterCycle(ctx, logger) {
		return ErrRestart
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			run()
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

func validateLayerTarget(cfg *Config) error {
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
	s := Status{LastRun: time.Now().UTC(), Redactions: redact.Counts{}}
	// The same sources `hev index` walks; a harness registered there and not
	// here would be archived only when someone remembers to run it by hand.
	piRoot, openCodeRoot := trace.DefaultPiRoot(), trace.DefaultOpenCodeRoot()
	if cfg, err := LoadConfig(); err == nil {
		piRoot, openCodeRoot = cfg.PiSessionsDir, cfg.OpenCodeDataDir
	}
	sources := []trace.Source{
		&trace.ClaudeSource{Root: trace.DefaultClaudeRoot()},
		&trace.CodexSource{Root: trace.DefaultCodexRoot()},
		&trace.DotSource{Root: trace.DefaultDotRoot()},
		&trace.PiSource{Root: piRoot},
		&trace.OpenCodeSource{Root: openCodeRoot},
	}
	var errors []string
	for _, src := range sources {
		rep, err := index.Run(src, client, state, index.Options{Images: images})
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		if rep.MigrationPending {
			s.MigrationPending = true
			logger.Warn("historical redaction migration pending; run hev index explicitly after reviewing archive rebuild scope", "source", sourcePath(src))
		}
		if len(rep.RemovedMissingSessions) > 0 {
			logger.Warn("archive upgrade removed sessions with missing sources", "session_ids", rep.RemovedMissingSessions)
		}
		s.Redactions.Add(rep.Redactions)
		s.UnitsIndexed += rep.UnitsIndexed
		errors = append(errors, rep.Errors...)
	}
	// This host's dispatch runs, as trace facts, so a dashboard on any host
	// sharing the archive shows them in its Asks view. An archive in files is
	// not shared; the dashboard here reads the run records themselves.
	if spans, ok := client.(index.SpanWriter); ok {
		if rep, err := index.RunTraceSpans(dispatch.Root(), spans, state, false); err != nil {
			errors = append(errors, "trace spans: "+err.Error())
		} else {
			s.Redactions.Add(rep.Redactions)
			s.UnitsIndexed += rep.UnitsIndexed
			errors = append(errors, rep.Errors...)
		}
	}
	cfg, cfgErr := LoadConfig()
	if cfgErr != nil {
		errors = append(errors, cfgErr.Error())
	} else if cfg.CaptureInstructions && cfg.CaptureInstructionsExplicit {
		if rep, err := index.RunInstructions(trace.InstructionSource{Sessions: sources, Workdirs: instructionWorkdirs}, client); err != nil {
			errors = append(errors, "instructions: "+err.Error())
		} else {
			s.Redactions.Add(rep.Redactions)
			s.UnitsIndexed += rep.UnitsIndexed
			errors = append(errors, rep.Errors...)
		}
	}
	// Save successful unit signatures even when another unit failed. Failed
	// units remain absent and retry on the next cycle.
	if err := state.Save(); err != nil {
		errors = append(errors, "save index state: "+err.Error())
	}
	if len(errors) > 0 {
		s.LastError = strings.Join(errors, "; ")
	}
	logger.Info("index cycle", "last_run", s.LastRun, "units_indexed", s.UnitsIndexed, "last_error", s.LastError)
	return s
}

func sourcePath(src trace.Source) string {
	switch s := src.(type) {
	case *trace.ClaudeSource:
		return s.Root
	case *trace.CodexSource:
		return s.Root
	case *trace.PiSource:
		return s.Root
	case *trace.OpenCodeSource:
		return s.Root
	case *trace.DotSource:
		return s.Root
	default:
		return ""
	}
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
	closeSecrets, err := secrets.Serve(ctx, secrets.New())
	if err != nil {
		logger.Warn("secrets endpoint unavailable; direct resolution remains available")
		return func() {}
	}
	return closeSecrets
}
