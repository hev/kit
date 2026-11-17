package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/secrets"
)

// LayerKey resolves the Layer API key from the environment or 1Password
// (layerenv.ReadKey), through the local secrets cache when a daemon serves
// the account.
func LayerKey() (string, error) { return layerenv.ReadKey() }

// layerKeyAfterCooldown is hevd's Layer-key read. A rate-limit answer does not
// end the daemon: exiting would drop the secrets endpoint and hand the
// supervisor a restart loop (each start one more read before the cooldown
// file existed), and a restart cannot get the key before the reset anyway. So
// hevd stays up, keeps serving cached secrets, waits until the reset time and
// tries once more. Any other error ends the daemon as before.
func layerKeyAfterCooldown(ctx context.Context, logger *slog.Logger) (string, error) {
	for {
		key, err := LayerKey()
		if err == nil || !secrets.IsRateLimit(err) {
			return key, err
		}
		reset := secrets.CooldownReset(err)
		if wait := time.Until(reset); wait < time.Minute {
			reset = time.Now().Add(time.Minute)
		}
		logger.Warn("Layer key rate limited; secrets endpoint stays up until the reset", "retry_after", reset.UTC().Format(time.RFC3339))
		fmt.Fprintf(os.Stderr, "hevd: %v; serving secrets and waiting until %s\n", err, reset.UTC().Format(time.RFC3339))
		t := time.NewTimer(time.Until(reset))
		select {
		case <-ctx.Done():
			t.Stop()
			return "", ctx.Err()
		case <-t.C:
		}
	}
}
