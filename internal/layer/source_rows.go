package layer

import (
	"context"
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
	// source.bytes measures provider decoded responses, not this JSON encoding.
	// Independently cap the complete decoded HTTP body (including envelope and
	// whitespace) at 256 MiB. Envelope overhead shares this fixed wire budget;
	// serialization expansion can therefore fail closed even within source budget.
	const maxWireBytes = 256 << 20
	var out projectedRowsResponse
	body := map[string]any{"include_attributes": attrs, "scan": map[string]any{"max_rows": maxRows, "max_bytes": maxBytes, "consistency": "observed"}}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	err := c.doContextBounded(ctx, "POST", "/v2/namespaces/"+url.PathEscape(c.Namespace+"-sessions")+"/scans/rows", body, &out, maxWireBytes)
	if projectedSourceUnsupported(err) {
		return c.listProjectedSessionRows(-1, nil, false, false, attrs)
	}
	if err != nil {
		return nil, err
	}
	if !out.Source.Complete || out.Source.Consistency != "observed" || out.Source.Rows != len(out.Rows) || out.Source.Rows > maxRows || out.Source.Pages < 1 || out.Source.Bytes < 0 || (len(out.Rows) > 0 && out.Source.Bytes == 0) || out.Source.Bytes > maxBytes || out.Source.Threads < 1 {
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

// Pointers distinguish required zero values from absent or null metadata.
// The custom decoder also requires an actual rows array and rejects error
// envelopes even when a server includes otherwise valid success fields.
type projectedRowsResponse struct {
	Rows   []trace.SessionRow
	Source struct {
		Rows        int    `json:"rows"`
		Pages       int    `json:"pages"`
		Bytes       int    `json:"bytes"`
		Complete    bool   `json:"complete"`
		Consistency string `json:"consistency"`
		Threads     int    `json:"threads"`
	}
}

func (out *projectedRowsResponse) UnmarshalJSON(raw []byte) error {
	var wire struct {
		Error  json.RawMessage     `json:"error"`
		Errors json.RawMessage     `json:"errors"`
		Rows   *[]trace.SessionRow `json:"rows"`
		Source *struct {
			Rows        *int    `json:"rows"`
			Pages       *int    `json:"pages"`
			Bytes       *int    `json:"bytes"`
			Complete    *bool   `json:"complete"`
			Consistency *string `json:"consistency"`
			Threads     *int    `json:"threads"`
		} `json:"source"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	if wire.Error != nil || wire.Errors != nil {
		return fmt.Errorf("dashboard source scan returned error envelope")
	}
	s := wire.Source
	if wire.Rows == nil || s == nil || s.Rows == nil || s.Pages == nil || s.Bytes == nil || s.Complete == nil || s.Consistency == nil || s.Threads == nil {
		return fmt.Errorf("dashboard source scan missing required rows or source metadata")
	}
	out.Rows = *wire.Rows
	out.Source.Rows, out.Source.Pages, out.Source.Bytes = *s.Rows, *s.Pages, *s.Bytes
	out.Source.Complete, out.Source.Consistency, out.Source.Threads = *s.Complete, *s.Consistency, *s.Threads
	return nil
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
