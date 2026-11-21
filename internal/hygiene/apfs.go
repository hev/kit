package hygiene

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"time"
)

const (
	DefaultAPFSTimeout  = 2 * time.Minute
	MinAPFSReclaimBytes = 1 * BytesPerGiB
	MaxAPFSReclaimBytes = 100 * BytesPerGiB
)

// APFSResult describes the outcome of macOS APFS local snapshot thinning.
type APFSResult struct {
	Eligible    bool   `json:"eligible"`
	Action      string `json:"action"` // "thinned", "would_thin", "skipped", "failed"
	TargetBytes uint64 `json:"target_bytes,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Error       string `json:"error,omitempty"`
}

func defaultExecCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func (r Runner) thinAPFS(ctx context.Context, cfg Config, osName string, freeBytes uint64, underPressure, dryRun bool) APFSResult {
	if osName != "darwin" {
		return APFSResult{Eligible: false, Action: "skipped", Reason: "not macOS"}
	}
	if !cfg.ShouldThinAPFS(osName) {
		return APFSResult{Eligible: false, Action: "skipped", Reason: "disabled in configuration"}
	}
	if !underPressure {
		return APFSResult{Eligible: false, Action: "skipped", Reason: "free space above min_free_gb"}
	}

	minBytes := cfg.MinFreeBytes()
	var target uint64
	if minBytes > freeBytes {
		target = minBytes - freeBytes
	}
	if target < MinAPFSReclaimBytes {
		target = MinAPFSReclaimBytes
	}
	if target > MaxAPFSReclaimBytes {
		target = MaxAPFSReclaimBytes
	}

	if dryRun {
		return APFSResult{
			Eligible:    true,
			Action:      "would_thin",
			TargetBytes: target,
		}
	}

	volume := r.Volume
	if volume == "" {
		volume = "/"
	}
	timeout := r.APFSTimeout
	if timeout <= 0 {
		timeout = DefaultAPFSTimeout
	}
	execFn := r.ExecCommand
	if execFn == nil {
		execFn = defaultExecCommand
	}

	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := execFn(tctx, "tmutil", "thinlocalsnapshots", volume, strconv.FormatUint(target, 10), "4")
	if err != nil {
		msg := "tmutil thinlocalsnapshots failed"
		if errors.Is(tctx.Err(), context.DeadlineExceeded) {
			msg = "tmutil thinlocalsnapshots timed out"
		}
		return APFSResult{
			Eligible:    true,
			Action:      "failed",
			TargetBytes: target,
			Error:       msg,
		}
	}
	return APFSResult{
		Eligible:    true,
		Action:      "thinned",
		TargetBytes: target,
	}
}
