// Package layerenv resolves kit's public environment contract. The gateway
// container retains its own upstream environment contract at the adapter edge.
package layerenv

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// GatewayKey is the upstream provider credential, separate from the gateway bearer.
const GatewayKey = "TURBOPUFFER_API_KEY"

// Aliases is the compatibility table. New names always take precedence; different
// nonempty values are rejected rather than silently choosing a credential.
var Aliases = map[string][]string{
	"HEVLAYER_API_KEY":        {"LAYER_API_KEY"},
	"HEVLAYER_PROVIDER_KEY":   {GatewayKey, "TPUF_API_KEY"},
	"HEVLAYER_API_KEY_OP_REF": {"LAYER_API_KEY_OP_REF"},
	"HEVLAYER_ENDPOINT":       {"LAYER_ENDPOINT"},
	"HEVLAYER_NAMESPACE":      {"LAYER_NAMESPACE"},
	"HEVLAYER_STORE":          {"LAYER_STORE"},
	"HEVLAYER_EMBED_MODEL":    {"LAYER_EMBED_MODEL"},
	"HEVLAYER_EMBED_URL":      {"LAYER_EMBED_URL"},
	"HEVLAYER_TELEMETRY":      {"LAYER_TELEMETRY"},
}

// ProviderAliases are the legacy BYO provider key names. They never select hosted.
var ProviderAliases = Aliases["HEVLAYER_PROVIDER_KEY"]

func ProviderKey() string { return Get("HEVLAYER_PROVIDER_KEY") }

const HostedEndpoint = "https://aws-us-east-1.hevlayer.com"

func Get(name string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	for _, alias := range Aliases[name] {
		if value := os.Getenv(alias); value != "" {
			return value
		}
	}
	return ""
}

var warned sync.Map

func Validate(out io.Writer) error {
	// Stable ordering makes errors and warnings predictable across platforms.
	for _, name := range []string{"HEVLAYER_API_KEY", "HEVLAYER_PROVIDER_KEY", "HEVLAYER_API_KEY_OP_REF", "HEVLAYER_ENDPOINT", "HEVLAYER_NAMESPACE", "HEVLAYER_STORE", "HEVLAYER_EMBED_MODEL", "HEVLAYER_EMBED_URL", "HEVLAYER_TELEMETRY"} {
		source, value := name, os.Getenv(name)
		for _, alias := range Aliases[name] {
			old := os.Getenv(alias)
			if old == "" {
				continue
			}
			if value != "" && old != value {
				return fmt.Errorf("conflicting values for %s and %s; unset the deprecated alias and use %s", source, alias, name)
			}
			if value == "" {
				source, value = alias, old
			}
		}
	}
	for name, aliases := range Aliases {
		for _, alias := range aliases {
			if os.Getenv(alias) == "" {
				continue
			}
			if _, loaded := warned.LoadOrStore(alias, true); !loaded {
				fmt.Fprintf(out, "warning: %s is deprecated; use %s\n", alias, name)
			}
		}
	}
	return nil
}

// Names includes aliases for environment guards and isolated child processes.
func Names() []string {
	var names []string
	for name, aliases := range Aliases {
		names = append(names, name)
		names = append(names, aliases...)
	}
	return names
}
