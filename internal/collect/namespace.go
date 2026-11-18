package collect

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// AttrNamespace is the OTel resource attribute a host app declares its
// namespace in (`@hevmind/trace` sets it from register's namespace). A
// collector routes each resource's spans by it and refuses spans whose
// namespace is missing or not on its allowlist.
const AttrNamespace = "hev.namespace"

// DefaultTracesPrefix prefixes an app namespace to name its Layer namespace:
// story-prod's turns live in hev-app-traces-story-prod.
const DefaultTracesPrefix = "hev-app-traces"

var namespaceRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)

// ValidNamespace reports whether ns is a well-formed app namespace,
// `<app>-<env>` by convention (story-staging, story-prod).
func ValidNamespace(ns string) bool { return namespaceRE.MatchString(ns) }

// ErrNoNamespaces is returned when a collector is given no namespaces to
// serve: with an empty allowlist it could only refuse every span.
var ErrNoNamespaces = errors.New("HEV_COLLECT_NAMESPACES is not set: hev collect refuses to serve without the namespaces its service key may write and read (e.g. --namespaces story-prod)")

// ParseNamespaces reads a comma-separated allowlist. Every entry must be a
// valid namespace; duplicates collapse. An empty list is ErrNoNamespaces.
func ParseNamespaces(list string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(list, ",") {
		ns := strings.TrimSpace(part)
		if ns == "" {
			continue
		}
		if !ValidNamespace(ns) {
			return nil, fmt.Errorf("namespace %q must match ^[a-z0-9][a-z0-9-]{0,47}$", ns)
		}
		if !seen[ns] {
			seen[ns] = true
			out = append(out, ns)
		}
	}
	if len(out) == 0 {
		return nil, ErrNoNamespaces
	}
	sort.Strings(out)
	return out, nil
}

// LayerNamespace is the Layer namespace an app namespace's turns live in:
// `<prefix>-<namespace>`. The app names only the suffix, and only from the
// allowlist, so it can never reach another Layer namespace such as the
// coding-agent trace archive.
func LayerNamespace(prefix, ns string) string {
	if prefix == "" {
		prefix = DefaultTracesPrefix
	}
	return prefix + "-" + ns
}

// spanNamespace is the namespace a span's resource declares, or "".
func spanNamespace(sp Span) string {
	v, _ := sp.Resource[AttrNamespace].(string)
	return v
}
