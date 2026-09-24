package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

type Status struct {
	LastRun      time.Time `json:"last_run"`
	UnitsIndexed int       `json:"units_indexed"`
	LastError    string    `json:"last_error,omitempty"`
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
	if err := os.MkdirAll(filepath.Dir(StatusPath()), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(StatusPath(), b, 0o600)
}

// Run continuously indexes local transcripts until interrupted.
func Run(ctx context.Context) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".hev"), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(home, ".hev", "hev.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()
	logger := slog.New(slog.NewJSONHandler(logFile, nil))

	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if cfg.LayerEndpoint == "" {
		cfg.LayerEndpoint = layer.DefaultEndpoint
	}
	if cfg.LayerAPIKey == "" {
		cfg.LayerAPIKey, err = LayerKey()
		if err != nil {
			return err
		}
	}
	if cfg.ScanInterval <= 0 {
		return fmt.Errorf("scan interval must be positive")
	}
	if err := validateLayerTarget(cfg); err != nil {
		return err
	}
	client, err := layer.New(cfg.LayerEndpoint, cfg.LayerAPIKey, cfg.LayerNamespace, os.Getenv("LAYER_EMBED_MODEL")).WithStore(layer.StoreKind(cfg.LayerStore))
	if err != nil {
		return err
	}

	// A local file lock prevents overlapping writers without opening a port.
	lock, err := os.OpenFile(filepath.Join(home, ".hev", "daemon.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("another daemon is already running: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	state := index.LoadState()

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	run := func() {
		s := runIndexCycle(client, state, logger)
		if err := writeStatus(s); err != nil {
			logger.Error("write status", "err", err)
		}
	}
	gate := firstScanGate(cfg, func() error { _, err := client.Health(); return err })
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
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			run()
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

func runIndexCycle(client *layer.Client, state *index.State, logger *slog.Logger) Status {
	s := Status{LastRun: time.Now().UTC()}
	// The same sources `hev index` walks; a harness registered there and not
	// here would be archived only when someone remembers to run it by hand.
	sources := []trace.Source{
		&trace.ClaudeSource{Root: trace.DefaultClaudeRoot()},
		&trace.CodexSource{Root: trace.DefaultCodexRoot()},
	}
	var errors []string
	for _, src := range sources {
		if _, err := os.Stat(sourcePath(src)); os.IsNotExist(err) {
			continue
		}
		rep, err := index.Run(src, client, state, index.Options{})
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		s.UnitsIndexed += rep.UnitsIndexed
		errors = append(errors, rep.Errors...)
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
	default:
		return ""
	}
}
