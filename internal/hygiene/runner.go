package hygiene

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/platformops"
)

// Runner executes bounded disk-hygiene passes and supervises periodic ticks for hevd.
type Runner struct {
	Home        string
	TempDir     string
	DispatchDir string
	Volume      string
	OS          string
	Now         func() time.Time
	DiskFree    func(path string) (uint64, error)
	CompactRuns func(opts dispatch.CompactOptions) (dispatch.CompactReport, error)
	ExecCommand func(ctx context.Context, name string, args ...string) ([]byte, error)
	TmpOwned    func(os.FileInfo) bool
	APFSTimeout time.Duration
	lockState   func(string) (func(), error)
	Logger      *slog.Logger
}

// Report describes the outcome of a single hygiene pass.
type Report struct {
	DryRun          bool                     `json:"dry_run"`
	Timestamp       time.Time                `json:"timestamp"`
	DurationMS      int64                    `json:"duration_ms"`
	UnderPressure   bool                     `json:"under_pressure"`
	FreeBytesBefore uint64                   `json:"free_bytes_before"`
	FreeBytesAfter  uint64                   `json:"free_bytes_after"`
	BytesFreed      int64                    `json:"bytes_freed"`
	CompactedRuns   []string                 `json:"compacted_runs"`
	KeptRuns        []dispatch.CompactAction `json:"kept_runs,omitempty"`
	PrunedTmpDirs   []string                 `json:"pruned_tmp_dirs"`
	KeptTmpDirs     []TmpAction              `json:"kept_tmp_dirs,omitempty"`
	EvictedSlots    int                      `json:"evicted_slots"`
	APFS            APFSResult               `json:"apfs"`
	Errors          []string                 `json:"errors,omitempty"`
}

// StatusReport describes persisted hygiene state and current disk status.
type StatusReport struct {
	Config           Config     `json:"config"`
	State            State      `json:"state"`
	CurrentFreeBytes uint64     `json:"current_free_bytes"`
	UnderPressure    bool       `json:"under_pressure"`
	NextRun          *time.Time `json:"next_run,omitempty"`
}

func (r Runner) resolvedHome() (string, error) {
	if r.Home != "" {
		return r.Home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("home directory unavailable")
	}
	return home, nil
}

func (r Runner) resolvedTempDir() string {
	if r.TempDir != "" {
		return r.TempDir
	}
	return os.TempDir()
}

func (r Runner) resolvedDispatchDir(home string) string {
	if r.DispatchDir != "" {
		return r.DispatchDir
	}
	if d := dispatch.Root(); d != "" {
		return d
	}
	return home
}

func (r Runner) resolvedOS() string {
	if r.OS != "" {
		return r.OS
	}
	return runtime.GOOS
}

func (r Runner) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func (r Runner) diskFree(path string) (uint64, error) {
	if r.DiskFree != nil {
		return r.DiskFree(path)
	}
	return DiskFreeBytes(path)
}

func (r Runner) compactRuns(opts dispatch.CompactOptions) (dispatch.CompactReport, error) {
	if r.CompactRuns != nil {
		return r.CompactRuns(opts)
	}
	return dispatch.CompactEndedRuns(opts)
}

// Run executes one bounded hygiene pass. In dryRun mode, zero files are modified
// and state.json is not written.
func (r Runner) Run(ctx context.Context, cfg Config, dryRun bool) (Report, error) {
	if err := cfg.Validate(); err != nil {
		return Report{}, err
	}
	home, err := r.resolvedHome()
	if err != nil {
		return Report{}, err
	}

	var release func()
	if !dryRun {
		var lockErr error
		release, lockErr = r.acquireStateLock(home)
		if platformops.IsLockBusy(lockErr) {
			return Report{}, errors.New("another hygiene pass is already running")
		}
		if release != nil {
			defer release()
		}
	}

	start := r.now()
	dispatchDir := r.resolvedDispatchDir(home)
	osName := r.resolvedOS()

	rep := Report{
		DryRun:        dryRun,
		Timestamp:     start,
		CompactedRuns: []string{},
		PrunedTmpDirs: []string{},
	}

	freeBefore, diskErr := r.diskFree(dispatchDir)
	if diskErr != nil {
		rep.Errors = append(rep.Errors, "could not measure free disk space")
	} else {
		rep.FreeBytesBefore = freeBefore
	}

	minBytes := cfg.MinFreeBytes()
	underPressure := diskErr == nil && minBytes > 0 && freeBefore < minBytes
	rep.UnderPressure = underPressure

	compactRep, compactErr := r.compactRuns(dispatch.CompactOptions{
		OlderThan: cfg.CompactAfterDuration(),
		Now:       start,
		DryRun:    dryRun,
	})
	if compactErr != nil {
		rep.Errors = append(rep.Errors, sanitizeError(compactErr, home))
	} else {
		rep.CompactedRuns = compactRep.Compacted
		rep.KeptRuns = compactRep.Kept
		rep.BytesFreed += compactRep.BytesFreed
	}

	tmpRep, tmpErr := PruneTmpDirs(r.resolvedTempDir(), cfg.PruneTmpAfterDuration(), start, dryRun, r.TmpOwned)
	if tmpErr != nil {
		rep.Errors = append(rep.Errors, sanitizeError(tmpErr, home))
	} else {
		rep.PrunedTmpDirs = tmpRep.Pruned
		rep.KeptTmpDirs = tmpRep.Kept
		rep.BytesFreed += tmpRep.BytesFreed
	}

	rep.APFS = r.thinAPFS(ctx, cfg, osName, freeBefore, underPressure, dryRun)
	if rep.APFS.Error != "" {
		rep.Errors = append(rep.Errors, rep.APFS.Error)
	}

	// Workspace slots can live on a different filesystem from dispatch runs.
	if minBytes > 0 {
		slotRoot := filepath.Join(home, ".hev", "workspace-slots")
		slotFreeBefore, err := r.diskFree(slotRoot)
		if err != nil {
			rep.Errors = append(rep.Errors, "could not measure workspace slot disk space")
		} else if slotFreeBefore < minBytes {
			rep.UnderPressure = true
			slotCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			rep.EvictedSlots, err = dispatch.EvictWorkspaceSlots(slotCtx, slotRoot, dryRun, func() (bool, error) {
				free, err := r.diskFree(slotRoot)
				return free < minBytes, err
			})
			cancel()
			if err != nil {
				rep.Errors = append(rep.Errors, sanitizeError(err, home))
			}
			if !dryRun && rep.EvictedSlots > 0 {
				if free, err := r.diskFree(slotRoot); err == nil && free > slotFreeBefore {
					rep.BytesFreed += int64(free - slotFreeBefore)
				}
			}
		}
	}

	if dryRun {
		rep.FreeBytesAfter = rep.FreeBytesBefore
	} else if freeAfter, err := r.diskFree(dispatchDir); err == nil {
		rep.FreeBytesAfter = freeAfter
	} else {
		rep.FreeBytesAfter = rep.FreeBytesBefore
	}

	end := r.now()
	if end.After(start) {
		rep.DurationMS = end.Sub(start).Milliseconds()
	}

	if !dryRun {
		// A full or read-only filesystem may prevent creating the lock.
		// Retry once cleanup has had a chance to free space.
		if release == nil {
			var err error
			release, err = r.acquireStateLock(home)
			if err != nil {
				return rep, errors.New("could not lock hygiene state after cleanup")
			}
			defer release()
		}
		s := State{
			LastRun:         &start,
			LastDurationMS:  rep.DurationMS,
			FreeBytesBefore: rep.FreeBytesBefore,
			FreeBytesAfter:  rep.FreeBytesAfter,
			BytesFreed:      rep.BytesFreed,
			CompactedRuns:   len(rep.CompactedRuns),
			PrunedTmpDirs:   len(rep.PrunedTmpDirs),
			EvictedSlots:    rep.EvictedSlots,
			APFSThinned:     rep.APFS.Action == "thinned",
			UnderPressure:   rep.UnderPressure,
			LastError:       strings.Join(rep.Errors, "; "),
		}
		if err := writeState(home, s); err != nil {
			return rep, errors.New("could not persist hygiene state after cleanup: " + err.Error())
		}
	}

	return rep, nil
}

func (r Runner) acquireStateLock(home string) (func(), error) {
	if r.lockState != nil {
		return r.lockState(home)
	}
	dir := stateDir(home)
	if err := platformops.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return platformops.LockPath(filepath.Join(dir, "run.lock"), true)
}

// Status returns the last recorded hygiene state and current disk free space.
// It never creates or modifies files.
func (r Runner) Status(cfg Config) (StatusReport, error) {
	if err := cfg.Validate(); err != nil {
		return StatusReport{}, err
	}
	home, err := r.resolvedHome()
	if err != nil {
		return StatusReport{}, err
	}
	state, err := readState(home)
	if err != nil {
		return StatusReport{}, err
	}
	freeBytes, diskErr := r.diskFree(r.resolvedDispatchDir(home))
	minBytes := cfg.MinFreeBytes()
	underPressure := diskErr == nil && minBytes > 0 && freeBytes < minBytes

	out := StatusReport{
		Config:           cfg,
		State:            state,
		CurrentFreeBytes: freeBytes,
		UnderPressure:    underPressure,
	}
	if cfg.Enabled {
		base := r.now()
		if state.LastRun != nil {
			next := state.LastRun.Add(cfg.EveryDuration())
			if next.After(base) {
				out.NextRun = &next
			} else {
				out.NextRun = &base
			}
		} else {
			next := base.Add(cfg.EveryDuration())
			out.NextRun = &next
		}
	}
	return out, nil
}

// Start supervises the periodic hygiene tick inside hevd when cfg.Enabled is true.
// The returned stop function cancels the loop and waits for any in-flight pass.
func (r Runner) Start(ctx context.Context, cfg Config) func() {
	if !cfg.Enabled {
		return func() {}
	}
	if err := cfg.Validate(); err != nil {
		if r.Logger != nil {
			r.Logger.Warn("hygiene disabled due to invalid configuration", "err", err)
		}
		return func() {}
	}
	loopCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		interval := cfg.EveryDuration()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				if loopCtx.Err() != nil {
					return
				}
				passTimeout := 10 * time.Minute
				if interval < passTimeout {
					passTimeout = interval
				}
				passCtx, passCancel := context.WithTimeout(loopCtx, passTimeout)
				rep, err := r.Run(passCtx, cfg, false)
				passCancel()
				if r.Logger != nil {
					if err != nil {
						r.Logger.Warn("hygiene pass failed", "err", err, "bytes_freed", rep.BytesFreed, "evicted_slots", rep.EvictedSlots)
					} else {
						r.Logger.Info("hygiene pass",
							"under_pressure", rep.UnderPressure,
							"bytes_freed", rep.BytesFreed,
							"compacted_runs", len(rep.CompactedRuns),
							"pruned_tmp_dirs", len(rep.PrunedTmpDirs),
							"evicted_slots", rep.EvictedSlots,
							"apfs_action", rep.APFS.Action,
							"errors", len(rep.Errors),
						)
					}
				}
			}
		}
	}()
	return func() {
		cancel()
		wg.Wait()
	}
}

func sanitizeError(err error, home string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if home != "" {
		msg = strings.ReplaceAll(msg, home, "~")
	}
	return msg
}
