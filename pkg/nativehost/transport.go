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
	return Run(ctx, cfg.Profiles, listener)
}
