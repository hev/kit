package kitcapture

import (
	"context"
	"time"

	"github.com/hev/kit/internal/platformops"
)

func lockFile(ctx context.Context, path string) (func(), error) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		unlock, err := platformops.LockPath(path, true)
		if !platformops.IsLockBusy(err) {
			return unlock, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
