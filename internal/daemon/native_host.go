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

// Run retains capture/indexing and optionally hosts fixed native receivers in
// this SAME managed daemon. No ambient execution mode is enabled by default.
func Run(ctx context.Context) error {
	path := os.Getenv("HEV_NATIVE_HOST_CONFIG")
	role := os.Getenv("HEV_CAPTURE_ROLE")
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
