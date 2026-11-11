package daemon

import (
	"context"
	"errors"
	"fmt"
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
	ref := layerenv.Get("HEVLAYER_API_KEY_OP_REF")
	if ref == "" {
		return "", fmt.Errorf("no HEVLAYER_API_KEY configured; set HEVLAYER_API_KEY or HEVLAYER_API_KEY_OP_REF explicitly")
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
			return "", fmt.Errorf("1Password returned nothing for HEVLAYER_API_KEY_OP_REF")
		}
		// A daemon that answered (a rate limit included) is final: retrying
		// directly would spend the quota the cooldown protects.
		if !errors.Is(err, secrets.ErrUnavailable) {
			return "", fmt.Errorf("read HEVLAYER_API_KEY_OP_REF through the secrets daemon: %w", err)
		}
	}
	if _, err := exec.LookPath("op"); err != nil {
		return "", fmt.Errorf("no HEVLAYER_API_KEY in the environment and no `op` to read %s", ref)
	}
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
	out, err := cmd.Output()
	outcome := "ok"
	if err != nil {
		outcome = "error"
	} else if strings.TrimSpace(string(out)) == "" {
		outcome = "empty"
	}
	secrets.LogRead(time.Now(), secrets.DefaultCaller("kit layer-key direct"), "read", []string{secrets.Label("HEVLAYER_API_KEY", ref)}, outcome)
	if err != nil {
		return "", fmt.Errorf("op read %s: %w", ref, err)
	}
	key := strings.TrimSpace(string(out))
	if key == "" {
		return "", fmt.Errorf("op read %s returned nothing", ref)
	}
	return key, nil
}
