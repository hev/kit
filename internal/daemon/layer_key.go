package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/secrets"
)

// layerKeyTTL is how long the local secrets daemon may serve the Layer key to
// later kit processes. A rotated key reaches every process within this time;
// a daemon restart discards it sooner.
const layerKeyTTL = 6 * time.Hour

// Resolving the key: the environment first for a shell that already has it,
// then 1Password for a machine provisioned from the vault. The keychain is
// deliberately not consulted — `security` refuses non-interactively over ssh,
// which is exactly how the headless host runs, and a lookup that can only
// succeed on a laptop is worse than one that fails the same way everywhere.
//
// When the local secrets daemon serves this account, the 1Password read goes
// through its memory cache, so a run of kit processes costs one request per
// layerKeyTTL. With no daemon the read is the direct `op read` it always was,
// recorded in the host's read log.
func LayerKey() (string, error) {
	layerenv.WarnDeprecated(os.Stderr)
	if key := layerenv.Get("HEVLAYER_API_KEY"); key != "" {
		return key, nil
	}
	// HEVLAYER_API_KEY_REF takes a reference to any provider (op:// or
	// azkv://); HEVLAYER_API_KEY_OP_REF, its older name, still works.
	refVar := "HEVLAYER_API_KEY_REF"
	ref := layerenv.Get(refVar)
	if ref == "" {
		refVar = "HEVLAYER_API_KEY_OP_REF"
		ref = layerenv.Get(refVar)
	}
	if ref == "" {
		return "", fmt.Errorf("no HEVLAYER_API_KEY configured; set HEVLAYER_API_KEY or HEVLAYER_API_KEY_REF (op:// or azkv://; HEVLAYER_API_KEY_OP_REF still works) explicitly")
	}
	cached := []secrets.Ref{{Name: "HEVLAYER_API_KEY", URI: ref, TTL: layerKeyTTL}}
	if secrets.Validate(cached) == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		values, err := secrets.ResolveWith(ctx, cached, secrets.Options{Caller: secrets.DefaultCaller("kit layer-key"), CacheOnly: true})
		cancel()
		if err == nil {
			if key := strings.TrimSpace(values["HEVLAYER_API_KEY"]); key != "" {
				return key, nil
			}
			return "", fmt.Errorf("the secret store returned nothing for %s", refVar)
		}
		// A daemon that answered (a rate limit included) is final: retrying
		// directly would spend the quota the cooldown protects.
		if !errors.Is(err, secrets.ErrUnavailable) {
			return "", fmt.Errorf("read %s through the secrets daemon: %w", refVar, err)
		}
	}
	if !strings.HasPrefix(ref, "op://") {
		// Every other provider reads directly through the package's own
		// resolver: same batching, cooldown and read log, and a notice on
		// stderr. Needs no op and no 1Password token.
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		values, err := secrets.ResolveWith(ctx, cached, secrets.Options{Caller: secrets.DefaultCaller("kit layer-key")})
		if err != nil {
			return "", fmt.Errorf("read %s: %w", refVar, err)
		}
		key := strings.TrimSpace(values["HEVLAYER_API_KEY"])
		if key == "" {
			return "", fmt.Errorf("the secret store returned nothing for %s", refVar)
		}
		return key, nil
	}
	if _, err := exec.LookPath("op"); err != nil {
		return "", fmt.Errorf("no HEVLAYER_API_KEY in the environment and no `op` to read %s", ref)
	}
	// A rate limit any kit process on this host was answered with holds until
	// its reset: no request, the same answer.
	if err := secrets.CheckCooldown(time.Now()); err != nil {
		return "", fmt.Errorf("read %s: %w", refVar, err)
	}
	// The fallback is never silent: one stderr line says the cache was missed.
	fmt.Fprintf(os.Stderr, "%s (kit layer-key: no daemon serves this account at %s)\n", secrets.FallbackNotice, secrets.Endpoint())
	cmd := exec.Command("op", "read", ref)
	// The service-account token is what authorizes 1Password, so it cannot come
	// from 1Password. A non-login shell has no profile to export it.
	if os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") == "" {
		if home, err := os.UserHomeDir(); err == nil {
			if b, err := os.ReadFile(home + "/.config/op/token"); err == nil {
				cmd.Env = append(os.Environ(), "OP_SERVICE_ACCOUNT_TOKEN="+strings.TrimSpace(string(b)))
			}
		}
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	now := time.Now()
	outcome := "ok"
	// op's diagnostic is classified only, never printed: it can echo a reference.
	limitErr := secrets.NoteDirectResult(now, err == nil, stderr.String())
	if limitErr != nil {
		outcome = "rate_limited"
	} else if err != nil {
		outcome = "error"
	} else if strings.TrimSpace(string(out)) == "" {
		outcome = "empty"
	}
	secrets.LogRead(now, secrets.DefaultCaller("kit layer-key direct"), "read", []string{secrets.Label("HEVLAYER_API_KEY", ref)}, outcome)
	if limitErr != nil {
		return "", fmt.Errorf("read %s: %w", refVar, limitErr)
	}
	if err != nil {
		return "", fmt.Errorf("op read %s: %w", ref, err)
	}
	key := strings.TrimSpace(string(out))
	if key == "" {
		return "", fmt.Errorf("op read %s returned nothing", ref)
	}
	return key, nil
}

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
