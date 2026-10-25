package layer

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/hev/kit/internal/trace"
)

// dashboardSourceRows acquires every projected revision once. Latest selection,
// filters, facets and joins remain in the caller. Never split physical cursors.
func (c *Client) dashboardSourceRows(attrs []string) ([]trace.SessionRow, error) {
	const maxRows = 1000000
	const maxBytes = 268435456
	var out struct {
		Rows   []trace.SessionRow `json:"rows"`
		Source struct {
			Rows        int    `json:"rows"`
			Pages       int    `json:"pages"`
			Bytes       int    `json:"bytes"`
			Complete    bool   `json:"complete"`
			Consistency string `json:"consistency"`
			Threads     int    `json:"threads"`
		} `json:"source"`
	}
	body := map[string]any{"include_attributes": attrs, "scan": map[string]any{"max_rows": maxRows, "max_bytes": maxBytes, "consistency": "observed"}}
	err := c.do("POST", "/v2/namespaces/"+url.PathEscape(c.Namespace+"-sessions")+"/scans/rows", body, &out)
	if projectedSourceUnsupported(err) {
		return c.listProjectedSessionRows(-1, nil, false, false, attrs)
	}
	if err != nil {
		return nil, err
	}
	if !out.Source.Complete || out.Source.Consistency != "observed" || out.Source.Rows != len(out.Rows) || out.Source.Rows > maxRows || out.Source.Pages < 1 || out.Source.Bytes < 0 || out.Source.Bytes > maxBytes || out.Source.Threads < 1 {
		return nil, fmt.Errorf("dashboard source scan returned invalid or incomplete accounting")
	}
	seen := make(map[string]bool, len(out.Rows))
	previous := ""
	for _, row := range out.Rows {
		if strings.TrimSpace(row.ID) == "" || (len(attrs) > 0 && strings.TrimSpace(row.SessionID) == "") || seen[row.ID] || row.ID <= previous {
			return nil, fmt.Errorf("dashboard source scan returned unusable or unordered identity")
		}
		seen[row.ID] = true
		previous = row.ID
	}
	return out.Rows, nil
}

// Only an explicit capability rejection authorizes the sequential projected
// compatibility path. Missing endpoints, auth, throttle and data errors fail.
func projectedSourceUnsupported(err error) bool {
	var e *HTTPError
	if !errors.As(err, &e) || e.Status != http.StatusUnprocessableEntity {
		return false
	}
	var body struct {
		Error   string `json:"error"`
		Feature string `json:"feature"`
	}
	if json.Unmarshal(e.Body, &body) != nil {
		return false
	}
	return body.Error == "UnsupportedByStore" && body.Feature == "projected_source_rows"
}
