package layer

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// ReconcileQueryFilters enables the CLI's Eq filters on an existing archive.
// Only affected attributes are sent, with every existing setting retained.
// There are no row writes, type changes or embedding declarations synthesized.
// A missing namespace is left to the first ordinary write. Failures are returned
// so hevd can report them and retry on its next scan, without caching success.
// Dependent queries may return 409 while the provider builds the new index.
func (c *Client) ReconcileQueryFilters() error {
	path := "/v1/namespaces/" + url.PathEscape(c.Namespace) + "/schema"
	var schema map[string]map[string]json.RawMessage
	if err := c.do("GET", path, nil, &schema); err != nil {
		var h *HTTPError
		if errors.As(err, &h) && h.Status == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("read query filter schema: %w", err)
	}
	changes := map[string]map[string]json.RawMessage{}
	for _, field := range []string{"workdir", "harness", "plan"} {
		attr, exists := schema[field]
		if !exists {
			continue
		}
		var typ string
		if err := json.Unmarshal(attr["type"], &typ); err != nil || typ != "string" {
			return fmt.Errorf("query filter %s has incompatible type; refusing schema update", field)
		}
		var enabled bool
		if raw, explicit := attr["filterable"]; explicit {
			if err := json.Unmarshal(raw, &enabled); err != nil {
				return fmt.Errorf("invalid %s filterable: %w", field, err)
			}
		} else {
			// Turbopuffer defaults scalar filtering on unless another text index is enabled.
			enabled = true
			for _, setting := range []string{"full_text_search", "regex", "glob", "fuzzy"} {
				if raw, ok := attr[setting]; ok && string(raw) != "false" && string(raw) != "null" {
					enabled = false
				}
			}
		}
		if enabled {
			continue
		}
		attr["filterable"] = json.RawMessage("true")
		changes[field] = attr
	}
	if len(changes) == 0 {
		return nil
	}
	if err := c.do("POST", "/v2/namespaces/"+url.PathEscape(c.Namespace), map[string]any{"schema": changes}, nil); err != nil {
		return fmt.Errorf("enable query filters: %w", err)
	}
	return nil
}
