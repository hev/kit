package daemon

import (
	"context"
	"log/slog"
	"time"

	"github.com/hev/kit/internal/dispatch"
)

// StartJobs is shared by Capture and secrets-only hevd. RunJobs holds a
// host/account lock; supervisors survive cancellation of this scheduler.
func StartJobs(ctx context.Context, logger *slog.Logger) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			if e := dispatch.RunJobs(ctx, dispatch.JobDriver{}); e != nil && ctx.Err() == nil {
				logger.Warn("jobs scheduler unavailable; retrying", "err", e)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}

	}()
	return func() { cancel(); <-done }
}
