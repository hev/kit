package layerenv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/secrets"
)

// ConfigPath is kit's config file: HEV_CONFIG, or ~/.hev/config.toml.
func ConfigPath() string {
	if p := os.Getenv("HEV_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(home, ".hev", "config.toml")
}

// PlaceholderKey is what the retired local Docker lane wrote as its API
// key. It is not a key, and counts as none.
const PlaceholderKey = "local"

// KeyTTL is how long the local secrets daemon may serve the Layer key to
// later kit processes. A rotated key reaches every process within this time;
// a daemon restart discards it sooner.
const KeyTTL = 6 * time.Hour

// KeyRef is the configured secret reference for the Layer API key and the
// variable it came from: HEVLAYER_API_KEY_REF (secret://, op:// or azkv://) first, then
// its older name HEVLAYER_API_KEY_OP_REF. Both empty means none.
func KeyRef() (name, ref string) {
	for _, name := range []string{"HEVLAYER_API_KEY_REF", "HEVLAYER_API_KEY_OP_REF"} {
		if ref := Get(name); ref != "" {
			return name, ref
		}
	}
	return "", ""
}

// ReadKey resolves the Layer API key: the environment first for a shell that
// already has it, then 1Password for a machine provisioned from the vault. The keychain is
// deliberately not consulted — `security` refuses non-interactively over ssh,
// which is exactly how the headless host runs, and a lookup that can only
// succeed on a laptop is worse than one that fails the same way everywhere.
//
// When the local secrets daemon serves this account, the 1Password read goes
// through its memory cache, so a run of kit processes costs one request per
// KeyTTL. With no daemon the read is the direct `op read` it always was,
// recorded in the host's read log.
func ReadKey() (string, error) {
	WarnDeprecated(os.Stderr)
	if key := Get("HEVLAYER_API_KEY"); key != "" {
		return key, nil
	}
	refVar, ref := KeyRef()
	if ref == "" {
		return "", fmt.Errorf("no HEVLAYER_API_KEY configured; set HEVLAYER_API_KEY or HEVLAYER_API_KEY_REF (secret://, op:// or azkv://; HEVLAYER_API_KEY_OP_REF still works) explicitly")
	}
	// secret://NAME resolves through this host's secret manager first, so
	// every path below sees the explicit reference it would see if written
	// out: the same cache entry, cooldown, read log and fallback.
	ref, err := secrets.ExpandRef(ref)
	if err != nil {
		return "", fmt.Errorf("%s: %w", refVar, err)
	}
	cached := []secrets.Ref{{Name: "HEVLAYER_API_KEY", URI: ref, TTL: KeyTTL}}
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
