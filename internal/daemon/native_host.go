package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hev/kit/internal/platformops"
	n "github.com/hev/kit/pkg/nativejob"
	"os"
	"os/signal"
	"syscall"

	"github.com/hev/kit/pkg/nativehost"
)

// RunOptions selects roles within the same managed daemon. NativeHostConfig
// must satisfy the existing checked profile contract; options grant no platform
// capability and do not change the process or user's environment.
type RunOptions struct {
	NativeHostConfig string
	CaptureRole      string
}

// Run preserves the existing environment-based service configuration.
func Run(ctx context.Context) error {
	return RunWithOptions(ctx, RunOptions{NativeHostConfig: os.Getenv("HEV_NATIVE_HOST_CONFIG"), CaptureRole: os.Getenv("HEV_CAPTURE_ROLE")})
}

// RunWithOptions gives native user units explicit per-process configuration
// without a shell wrapper or persistent environment edits. The same role and
// unsupported-capability checks apply as for Run.
func RunWithOptions(ctx context.Context, options RunOptions) error {
	path := options.NativeHostConfig
	role := options.CaptureRole
	if role == "" {
		role = "required"
	}
	if role != "required" && role != "held" {
		return fmt.Errorf("invalid capture role")
	}
	if role == "held" && path == "" {
		return fmt.Errorf("held capture requires explicit native host profile")
	}
	if path == "" {
		return runCapture(ctx)
	}
	cfg, err := nativehost.VerifyConfig(path)
	if err != nil {
		return err
	}
	if role == "held" {
		if err := requireHeldCapture(platformops.RequireTraceWrites()); err != nil {
			return err
		}
		pins := map[string]string{}
		for _, p := range cfg.Profiles {
			pins[p.ID] = n.Digest(p)
		}
		if err := json.NewEncoder(os.Stderr).Encode(struct {
			Version        string            `json:"version"`
			Capture        string            `json:"capture"`
			Reason         string            `json:"reason"`
			Native         string            `json:"native"`
			ProfileDigests map[string]string `json:"profile_digests"`
		}{"daemon-role/v1", "held", "unsupported_trace_writes", "starting", pins}); err != nil {
			return err
		}
	}
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if role == "held" {
		return nativehost.RunConfig(ctx, path)
	}
	errors := make(chan error, 2)
	go func() { errors <- nativehost.RunConfig(ctx, path) }()
	go func() { errors <- runCapture(ctx) }()
	first := <-errors
	cancel()
	second := <-errors
	if first != nil {
		return first
	}
	return second
}

// requireHeldCapture accepts only the explicit platform capability sentinel.
// A supported platform or an operational failure cannot disable capture.
func requireHeldCapture(err error) error {
	if err == nil {
		return fmt.Errorf("capture supported; held role refused")
	}
	if !errors.Is(err, platformops.ErrTraceWritesUnsupported) {
		return err
	}
	return nil
}
