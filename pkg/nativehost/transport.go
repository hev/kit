package nativehost

import (
	"context"
	"errors"
	n "github.com/hev/kit/pkg/nativejob"
	"os"
)

// RunConfig retains the same static config/job contract on each platform.
// Endpoint security and lifecycle are OS adapters, not launch authority.
func RunConfig(ctx context.Context, path string) error {
	if !supported() {
		return ErrHeld
	}
	cfg, err := VerifyConfig(path)
	if err != nil {
		return err
	}
	birth, err := n.ProcessBirth(os.Getpid())
	if err != nil {
		return err
	}
	closeService, err := platformEnterService(path, birth)
	return withServiceLifetime(closeService, err, func() error { return runConfigured(ctx, path, cfg) })
}

// Enter may return a retained owned resource together with an error after a
// partial transition. Always release it, and never conceal a cleanup failure.
func withServiceLifetime(closeService func() error, enterErr error, run func() error) (err error) {
	if closeService != nil {
		defer func() { err = errors.Join(err, closeService()) }()
	}
	if enterErr != nil {
		return enterErr
	}
	if closeService == nil {
		return ErrHeld
	}
	return run()
}

func runConfigured(ctx context.Context, path string, cfg Config) error {
	listener, err := openEndpoint(ctx, path, cfg)
	if err != nil {
		return err
	}
	defer listener.Close()
	if cfg.Remote == nil {
		return Run(ctx, cfg.Profiles, listener)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, 2)
	go func() { results <- Run(ctx, cfg.Profiles, listener) }()
	go func() { results <- runReceiverRPC(ctx, cfg) }()
	first := <-results
	cancel()
	_ = listener.Close()
	second := <-results
	if first != nil {
		return first
	}
	return second
}
