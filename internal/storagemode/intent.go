package storagemode

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/layerenv"
)

// Files is only the answer when nothing on the machine says Layer. Resolve
// finds no key; Intent then asks whether anything else names hev layer. A
// process that finds no key on a machine set up for Layer must fail with
// NoKeyError rather than read or write an empty file archive beside a daemon
// that writes to Layer.

// Intent lists what in kit's config file and this environment says the
// machine keeps its traces in hev layer, in words for an operator:
//
//   - [layer].endpoint naming a gateway that is not on this machine;
//   - [layer].namespace, unless the table is the one the retired local Docker
//     lane wrote (store "pgvector", api_key "local" or a loopback endpoint);
//   - HEVLAYER_ENDPOINT in the environment.
//
// A config that does not exist names nothing. One that cannot be read is an
// error: kit cannot tell what the operator asked for.
func Intent(configPath string) ([]string, error) {
	var reasons []string
	if v := layerenv.Get("HEVLAYER_ENDPOINT"); v != "" && !Loopback(v) {
		reasons = append(reasons, "HEVLAYER_ENDPOINT is set")
	}
	if configPath == "" {
		configPath = layerenv.ConfigPath()
	}
	var file struct {
		Layer struct {
			Endpoint  string `toml:"endpoint"`
			Namespace string `toml:"namespace"`
			Store     string `toml:"store"`
			APIKey    string `toml:"api_key"`
		} `toml:"layer"`
	}
	if _, err := toml.DecodeFile(configPath, &file); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return reasons, nil
		}
		return reasons, fmt.Errorf("read %s to find the storage mode: %w", configPath, err)
	}
	l := file.Layer
	endpoint := strings.TrimSpace(l.Endpoint)
	retired := strings.EqualFold(strings.TrimSpace(l.Store), "pgvector") ||
		strings.TrimSpace(l.APIKey) == layerenv.PlaceholderKey ||
		(endpoint != "" && Loopback(endpoint))
	if endpoint != "" && !Loopback(endpoint) {
		reasons = append(reasons, fmt.Sprintf("%s names a Layer endpoint ([layer] endpoint = %q)", configPath, endpoint))
	}
	if ns := strings.TrimSpace(l.Namespace); ns != "" && !retired {
		reasons = append(reasons, fmt.Sprintf("%s names a Layer namespace ([layer] namespace = %q)", configPath, ns))
	}
	return reasons, nil
}

// Loopback reports whether endpoint is on this machine (the retired local
// Docker lane's gateway). Such an endpoint names no shared archive.
func Loopback(endpoint string) bool {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// NoKeyError is what a command gets on a machine set up for hev layer when it
// cannot get a key: it never falls back to files. The message says the
// machine is set up for hev layer, why no key was found and what to set. It
// never carries a key.
type NoKeyError struct {
	// Reasons are the signals that the machine is set up for Layer.
	Reasons []string
	// Cause is why no key was found: nil when no key source is configured in
	// this process, or the read error of a configured one.
	Cause error
	// ConfigPath is kit's config file, named in the advice.
	ConfigPath string
}

func (e *NoKeyError) Error() string {
	var b strings.Builder
	b.WriteString("this machine is set up for hev layer, so kit will not use a file archive here")
	if len(e.Reasons) > 0 {
		b.WriteString(" (" + strings.Join(e.Reasons, "; ") + ")")
	}
	b.WriteString(", but this process has no Layer API key: ")
	if e.Cause != nil {
		b.WriteString("the configured key could not be read: " + e.Cause.Error())
	} else {
		b.WriteString("HEVLAYER_API_KEY and HEVLAYER_API_KEY_OP_REF are not set in this environment and [layer].api_key in " + e.ConfigPath + " is empty")
	}
	b.WriteString(".\nSet HEVLAYER_API_KEY, or HEVLAYER_API_KEY_OP_REF to a 1Password reference, in this environment (a capture daemon given its key only in its service unit does not share it with shells or dispatched workers), or set [layer].api_key in " + e.ConfigPath + ".")
	b.WriteString("\nTo keep traces in files on this machine instead, remove endpoint and namespace from [layer] in " + e.ConfigPath + " (name a file archive with namespace under [archive]) and unset HEVLAYER_ENDPOINT.")
	return b.String()
}

func (e *NoKeyError) Unwrap() error { return e.Cause }

// IsNoKey reports whether err is a NoKeyError.
func IsNoKey(err error) bool {
	var e *NoKeyError
	return errors.As(err, &e)
}
