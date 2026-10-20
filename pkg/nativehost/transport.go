package nativehost

import "context"

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
