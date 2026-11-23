package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/storagemode"
	"github.com/hev/kit/internal/trace"
)

type TargetStatus struct {
	Name         string           `json:"name"`
	Mode         string           `json:"mode"`
	Storage      storagemode.Mode `json:"storage"`
	Endpoint     string           `json:"endpoint,omitempty"`
	Namespace    string           `json:"namespace,omitempty"`
	KeySource    string           `json:"key_source,omitempty"`
	LastRun      time.Time        `json:"last_run"`
	UnitsIndexed int              `json:"units_indexed"`
	LastError    string           `json:"last_error,omitempty"`
}

func (t captureTarget) status() TargetStatus {
	s := TargetStatus{Name: t.Name, Mode: t.Mode, Storage: storagemode.Layer}
	if t.ArchiveDir != "" {
		s.Storage = storagemode.Filesystem
	}
	if t.err == nil {
		s.Namespace = t.Namespace
		s.Endpoint = CaptureEndpoint(t.Endpoint)
	}
	if t.KeyRef != "" {
		s.KeySource = keySource(t.KeyRef)
	}
	return s
}

type captureStatus struct {
	mu     sync.Mutex
	status Status
	logger *slog.Logger
}

// Publish completion immediately. A slower target keeps its last result while
// the other workers continue publishing theirs.
func (s *captureStatus) publish(i int, target TargetStatus, result Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target.LastRun, target.UnitsIndexed, target.LastError = result.LastRun, result.UnitsIndexed, result.LastError
	if len(s.status.Targets) > 0 {
		s.status.Targets[i] = target
	}
	if i == 0 {
		result.Targets = s.status.Targets
		s.status = result
	}
	if err := writeStatus(s.status); err != nil {
		s.logger.Error("write capture status", "error", captureError("save status", err))
	}
}

func openLegacyPrimary(ctx context.Context, cfg *Config, logger *slog.Logger) (Target, error) {
	// With only the legacy target, stay up serving the secret endpoint while
	// a rate limit clears. This is the existing supervisor startup behavior.
	_, ref := layerenv.KeyRef()
	conf := *cfg
	if key := strings.TrimSpace(conf.LayerAPIKey); (key == "" || key == layerenv.PlaceholderKey) && ref != "" {
		waitCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		key, err := layerKeyAfterCooldown(waitCtx, logger)
		stop()
		if err != nil {
			return Target{}, fmt.Errorf("primary capture: %s", captureError("resolve API key", err))
		}
		conf.LayerAPIKey = key
	}
	return openPrimary(&conf, logger)
}

func openPrimary(cfg *Config, logger *slog.Logger) (Target, error) {
	target, err := OpenTarget(cfg, "")
	if err != nil {
		var noKey *storagemode.NoKeyError
		if errors.As(err, &noKey) {
			return Target{}, errors.New("primary capture: Layer API key unavailable; check HEVLAYER_API_KEY, HEVLAYER_API_KEY_REF, or [layer].api_key")
		}
		return Target{}, errors.New("primary capture: could not open archive")
	}
	if target.Retired {
		logger.Warn("local Docker lane retired", "message", RetiredMessage(cfg))
	}
	if target.Layer != nil {
		conf := *cfg
		conf.LayerAPIKey, conf.LayerEndpoint = target.Layer.APIKey, target.Layer.Endpoint
		if err := validateLayerTarget(&conf); err != nil {
			return Target{}, err
		}
	}
	return target, nil
}

func recordPrimary(target Target, logger *slog.Logger) {
	record := recordFor(target)
	record.Endpoint = CaptureEndpoint(record.Endpoint)
	if err := writeStorageRecord(record); err != nil {
		logger.Warn("write storage record", "error", captureError("save primary storage", err))
	}
}

type captureCycle struct {
	state         *index.State
	workdirs      trace.WorkdirCache
	outcomeCursor string
	options       index.Options
	profiles      []trace.Source
}

func (c *captureCycle) run(ctx context.Context, target Target, logger *slog.Logger) Status {
	if target.Layer != nil {
		target.Layer = target.Layer.WithContext(ctx)
		if err := target.Layer.ReconcileQueryFilters(); err != nil {
			return Status{LastRun: time.Now().UTC(), LastError: captureError("reconcile archive query filters", err)}
		}
	}
	archive := target.Archive()
	opts := c.options
	opts.Images, opts.Context = target.Images, ctx
	result := indexCapture(archive, c.state, logger, opts, &c.workdirs, c.profiles)
	if opts.FilterTurns == nil && ctx.Err() == nil {
		sweepCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		next, updated, err := index.SweepOutcomes(sweepCtx, archive, index.NewOutcomeEnricher(), c.outcomeCursor, time.Now().Add(-60*24*time.Hour).UnixMilli(), 10)
		cancel()
		c.outcomeCursor = next
		if err != nil {
			logger.Warn("outcome sweep", "error", captureError("outcome sweep", err))
		} else if updated > 0 {
			logger.Info("outcome sweep", "updated", updated)
		}
	}
	return result
}

func runCapture(ctx context.Context, cfg *Config, home string, primary Target, resolve captureKeyResolver, logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
	}()
	status := captureStatus{logger: logger}
	primaryStatus := TargetStatus{Name: primaryTarget, Mode: "mirror"}
	multi := len(cfg.captureTargets) > 0
	if multi {
		primaryStatus.Namespace = cfg.LayerNamespace
		primaryStatus.Storage = storagemode.Filesystem
		if resolution, err := ResolveMode(cfg); err == nil {
			primaryStatus.Storage, primaryStatus.KeySource = resolution.Mode, resolution.Source
		} else {
			primaryStatus.Storage = storagemode.Layer
		}
		primarySpec := targetConfig{Namespace: cfg.LayerNamespace}
		if primaryStatus.Storage == storagemode.Filesystem {
			primarySpec.ArchiveDir = filearchive.Root(cfg.ArchiveDir)
		} else {
			primarySpec.Endpoint = cfg.LayerEndpoint
			if primarySpec.Endpoint == "" {
				primarySpec.Endpoint = layer.DefaultEndpoint
			}
			primaryStatus.Endpoint = CaptureEndpoint(primarySpec.Endpoint)
		}
		specs := checkCaptureDestinations(cfg.captureTargets, primarySpec)
		status.status.Targets = append(status.status.Targets, primaryStatus)
		for _, spec := range specs {
			status.status.Targets = append(status.status.Targets, spec.status())
		}
		for i, spec := range specs {
			workers.Add(1)
			go func() {
				defer workers.Done()
				spec.run(ctx, cfg, home, resolve, logger, func(target TargetStatus, result Status) {
					status.publish(i+1, target, result)
				})
			}()
		}
	}

	cycle := captureCycle{state: index.LoadState()}
	run := func() {
		if err := dispatch.SweepQueueBackstop(); err != nil {
			logger.Warn("dispatch queue sweep", "error", captureError("sweep dispatch queue", err))
		}
		if primary.Mode == "" {
			var err error
			conf := *cfg
			_, ref := layerenv.KeyRef()
			if key := strings.TrimSpace(conf.LayerAPIKey); (key == "" || key == layerenv.PlaceholderKey) && ref != "" {
				conf.LayerAPIKey, err = resolveCaptureKey(ctx, ref, resolve)
			}
			if err == nil {
				primary, err = openPrimary(&conf, logger)
			}
			if err != nil {
				result := Status{LastRun: time.Now().UTC(), LastError: err.Error()}
				logger.Error("capture target failed", "target", primaryTarget, "error", result.LastError)
				status.publish(0, primaryStatus, result)
				return
			}
			recordPrimary(primary, logger)
			if primary.Layer != nil {
				if gate := firstScanGate(cfg, func() error { _, err := primary.Layer.Health(); return err }); gate != nil {
					if err := gate(ctx); err != nil {
						logger.Warn("gateway health check failed", "error", captureError("gateway health check", err))
					}
				}
			}
		}
		primaryStatus.Storage = primary.Mode
		if primary.Layer != nil {
			primaryStatus.Endpoint = CaptureEndpoint(primary.Layer.Endpoint)
		}
		status.publish(0, primaryStatus, cycle.run(ctx, primary, logger.With("target", primaryTarget)))
	}
	var gate func(context.Context) error
	if primary.Layer != nil {
		gate = firstScanGate(cfg, func() error { _, err := primary.Layer.Health(); return err })
	}
	return scanLoop(ctx, gate, run, cfg.ScanInterval, logger)
}

func (t captureTarget) run(ctx context.Context, cfg *Config, home string, resolve captureKeyResolver, logger *slog.Logger, publish func(TargetStatus, Status)) {
	status := t.status()
	logger = logger.With("target", t.Name)
	if t.err != nil {
		logger.Error("invalid capture target", "error", t.err.Error())
		publish(status, Status{LastRun: time.Now().UTC(), LastError: t.err.Error()})
		return
	}
	cycle := captureCycle{
		state:    index.LoadStateAt(filepath.Join(home, ".hev", "targets", strings.ToLower(t.Name), "state.json")),
		profiles: t.profileSources(cfg.Host),
	}
	if t.Mode == "workspace" {
		cycle.options.FilterTurns = t.filterTurns
	}
	scope := t.stateScope()
	if cycle.state.CaptureScope != scope {
		cycle.state.Units = make(map[string]string)
		cycle.state.CaptureScope = scope
	}
	var target Target
	var refreshKey time.Time
	defer func() {
		if target.Layer != nil {
			target.Layer.HTTP.CloseIdleConnections()
		}
	}()
	ticker := time.NewTicker(cfg.ScanInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		result := Status{LastRun: time.Now().UTC()}
		next := target
		var err error
		// A valid cached credential can keep writing while another reference
		// is stuck in the same provider's fetch queue.
		if !result.LastRun.Before(refreshKey) {
			next, err = t.open(ctx, cfg, resolve, target)
			if err == nil && next.Layer != nil && keySource(t.KeyRef) != "environment" {
				refreshKey = result.LastRun.Add(layerenv.KeyTTL)
			}
		}
		if err != nil {
			result.LastError = err.Error()
		} else {
			if target.Layer != nil && target.Layer != next.Layer {
				target.Layer.HTTP.CloseIdleConnections()
			}
			target = next
			scanCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			result = cycle.run(scanCtx, target, logger)
			if errors.Is(scanCtx.Err(), context.DeadlineExceeded) {
				result.LastError = "capture scan timed out; will resume next scan"
			}
			cancel()
		}
		if result.LastError != "" {
			logger.Error("capture target failed", "error", result.LastError)
		}
		publish(status, result)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
