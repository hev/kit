// Package layerenv resolves kit's public environment contract. The gateway
// container retains its own upstream environment contract at the adapter edge.
package layerenv

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// GatewayKey is the upstream gateway variable and a deprecated kit fallback.
const GatewayKey = "TURBOPUFFER_API_KEY"

// Aliases is the compatibility table. Canonical names win; old names are fallbacks.
var Aliases = map[string][]string{
	"HEVLAYER_API_KEY":        {"LAYER_API_KEY", GatewayKey, "TPUF_API_KEY"},
	"HEVLAYER_API_KEY_OP_REF": {"LAYER_API_KEY_OP_REF"},
	"HEVLAYER_ENDPOINT":       {"LAYER_ENDPOINT"},
	"HEVLAYER_NAMESPACE":      {"LAYER_NAMESPACE"},
	"HEVLAYER_STORE":          {"LAYER_STORE"},
	"HEVLAYER_EMBED_MODEL":    {"LAYER_EMBED_MODEL"},
	"HEVLAYER_EMBED_URL":      {"LAYER_EMBED_URL"},
	"HEVLAYER_TELEMETRY":      {"LAYER_TELEMETRY"},
}

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

var warned sync.Once

// WarnDeprecated emits at most one nonfatal migration warning per process.
func WarnDeprecated(out io.Writer) {
	for _, name := range []string{"HEVLAYER_API_KEY", "HEVLAYER_API_KEY_OP_REF", "HEVLAYER_ENDPOINT", "HEVLAYER_NAMESPACE", "HEVLAYER_STORE", "HEVLAYER_EMBED_MODEL", "HEVLAYER_EMBED_URL", "HEVLAYER_TELEMETRY"} {
		for _, alias := range Aliases[name] {
			if os.Getenv(alias) != "" {
				warned.Do(func() { fmt.Fprintf(out, "warning: %s is deprecated; use %s\n", alias, name) })
				return
			}
		}
	}
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
