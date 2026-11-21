package license

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/secrets"
)

const (
	secretTTL       = 6 * time.Hour
	refreshInterval = time.Minute
)

type Config struct {
	Key    string `toml:"key" json:"key,omitempty"`
	KeyRef string `toml:"key_ref" json:"key_ref,omitempty"`
}

func Configuration() (Config, error) {
	if key := strings.TrimSpace(os.Getenv("HEV_LICENSE")); key != "" {
		return Config{Key: key}, nil
	}
	if ref := strings.TrimSpace(os.Getenv("HEV_LICENSE_REF")); ref != "" {
		return Config{KeyRef: ref}, nil
	}
	var config struct {
		License Config `toml:"license"`
	}
	if _, err := toml.DecodeFile(layerenv.ConfigPath(), &config); err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, errors.New("could not read Kit license configuration")
	}
	config.License.Key = strings.TrimSpace(config.License.Key)
	config.License.KeyRef = strings.TrimSpace(config.License.KeyRef)
	return config.License, nil
}

func readReference(ctx context.Context, ref string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	uri, err := secrets.ExpandRef(ref)
	if err != nil {
		return "", errors.New("invalid Kit license secret reference")
	}
	const name = "HEV_LICENSE"
	refs := []secrets.Ref{{Name: name, URI: uri, TTL: secretTTL}}
	if secrets.Validate(refs) != nil {
		return "", errors.New("invalid Kit license secret reference")
	}
	opts := secrets.Options{Caller: secrets.DefaultCaller("kit license"), Notice: io.Discard, CacheOnly: true}
	values, err := secrets.ResolveWith(ctx, refs, opts)
	if errors.Is(err, secrets.ErrUnavailable) {
		if strings.HasPrefix(uri, "op://") && secrets.CheckCooldown(time.Now()) != nil {
			return "", errors.New("Kit license secret provider is in cooldown")
		}
		opts.CacheOnly = false
		values, err = secrets.ResolveWith(ctx, refs, opts)
	}
	if err != nil {
		if !secrets.CooldownReset(err).IsZero() {
			return "", errors.New("Kit license secret provider is in cooldown")
		}
		return "", errors.New("could not resolve Kit license secret")
	}
	key := strings.TrimSpace(values[name])
	if key == "" {
		return "", errors.New("Kit license secret is empty")
	}
	return key, nil
}

func floor(reason string, now time.Time) Status {
	return Status{Timeline: Timeline{State: Floor}, Reason: reason, CheckedAt: now, Until: now.Add(secretTTL)}
}

func verified(key string, now time.Time) Status {
	claims, err := Production().Verify(key)
	if err != nil {
		return floor(err.Error(), now)
	}
	s := floor("", now)
	s.Valid = true
	s.Expires = &claims.Expires
	s.Timeline = Evaluate(now, claims.Expires, claims.Tier, DefaultGrace)
	s.Subject = redact.PublicTokens(claims.Subject)
	s.Tier = redact.PublicTokens(claims.Tier)
	s.JTI = redact.PublicTokens(claims.JTI)
	for _, feature := range claims.Features {
		s.Features = append(s.Features, redact.PublicTokens(feature))
	}
	if len(claims.Limits) != 0 {
		s.Limits = make(map[string]uint64, len(claims.Limits))
		for name, value := range claims.Limits {
			// A scrubbed name must never acquire a limit from a different claim.
			if redact.PublicTokens(name) == name {
				s.Limits[name] = value
			}
		}
	}
	if s.State == Floor {
		s.Reason = "expired"
	}
	return s
}

// Resolve refreshes the host projection using Kit production trust.
func Resolve(ctx context.Context) Status {
	cfg, err := Configuration()
	key := cfg.Key
	if err == nil && key == "" && cfg.KeyRef != "" {
		key, err = readReference(ctx, cfg.KeyRef)
	}
	now := time.Now().UTC()
	var s Status
	switch {
	case err != nil:
		s = floor(err.Error(), now)
	case key == "":
		s = floor("missing", now)
	default:
		s = verified(key, now)
	}
	// Inspection still works when the home directory is unwritable.
	if ctx.Err() == nil {
		_ = writeProjection(s)
	}
	return s
}

func projectionPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".hev", "license-status.json")
}

func writeProjection(s Status) error {
	path := projectionPath()
	if path == "" {
		return errors.New("Kit license projection needs a home directory")
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return platformops.WriteFileAtomic(path, data, 0600)
}

// ReadProjection performs only local I/O. The receiver's host account owns
// the projection and is the trust boundary for these entitlement checks.
func ReadProjection(now time.Time) Status {
	path := projectionPath()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxTokenSize*4 {
		return floor("status unavailable", now)
	}
	f, err := os.Open(path)
	if err != nil {
		return floor("status unavailable", now)
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxTokenSize*4 {
		return floor("status unavailable", now)
	}
	var s Status
	if json.NewDecoder(io.LimitReader(f, maxTokenSize*4)).Decode(&s) != nil || s.CheckedAt.After(now) || now.After(s.Until) {
		return floor("status unavailable", now)
	}
	if s.Valid && s.Expires != nil {
		s.Timeline = Evaluate(now, *s.Expires, s.Tier, DefaultGrace)
		if s.State == Floor {
			s.Reason = "expired"
		}
	} else {
		s.Valid = false
		s.Timeline = Timeline{State: Floor}
	}
	return s
}

// Inspect uses local configuration and cached status without accessing a vault.
func Inspect(now time.Time) (Status, bool) {
	cfg, err := Configuration()
	switch {
	case err != nil:
		return floor(err.Error(), now), true
	case cfg.Key != "":
		return verified(cfg.Key, now), true
	default:
		s := ReadProjection(now)
		return s, cfg.KeyRef != "" || (s.Reason != "missing" && s.Reason != "status unavailable")
	}
}

// Refresh keeps vault I/O independent of daemon indexing and receiver checks.
func Refresh(ctx context.Context) {
	tick := time.NewTicker(refreshInterval)
	defer tick.Stop()
	for ctx.Err() == nil {
		Resolve(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
