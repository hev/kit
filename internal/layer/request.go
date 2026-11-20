package layer

import (
	"encoding/json"

	"github.com/hev/kit/pkg/search"
)

// Request sends a native Layer wire request using the client's shared transport,
// authentication, timeout and error handling. Callers own the request schema.
func (c *Client) Request(method, path string, body, out any) error {
	return c.do(method, path, body, out)
}

// TextField is the declaration for a namespace's one searched text column on
// this client's store: full-text always, embedded where the store can. A
// caller that owns its own schema (an app namespace) declares its text column
// with it so it embeds the way the trace archive does.
func (c *Client) TextField() map[string]any { return c.textField() }

// SearchRows runs kit's hybrid retrieval over any namespace the client names,
// by the same route `hev traces search` takes on this store, and returns the
// rows as maps. Rows carry whatever relevance attribute the store reports
// ("$dist" or "$score"), and are in the store's fused order.
func (c *Client) SearchRows(query string, topK int, filter any, attrs []string) ([]map[string]any, error) {
	if topK <= 0 {
		topK = 10
	}
	route, err := c.Caps.SearchRoute()
	if err != nil {
		return nil, err
	}
	body, err := c.multiQueryBody(query, topK, filter, attrs)
	if route == RouteHybridText {
		body, err = c.hybridTextBody(query, topK, filter, attrs)
	}
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := c.do("POST", "/v2/namespaces/"+c.Namespace+"/query", body, &raw); err != nil {
		if isNamespaceMissing(err) {
			return nil, nil
		}
		return nil, err
	}
	return search.Rows[map[string]any](raw)
}
