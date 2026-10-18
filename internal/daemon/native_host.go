package daemon

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/hev/kit/pkg/nativehost"
)

// Run retains capture/indexing and optionally hosts fixed native receivers in
// this SAME managed daemon. No ambient execution mode is enabled by default.
func Run(ctx context.Context) error {
	path := os.Getenv("HEV_NATIVE_HOST_CONFIG")
	if path == "" {
		return runCapture(ctx)
	}
	if _, err := nativehost.VerifyConfig(path); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
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
