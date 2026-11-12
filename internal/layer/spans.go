package layer

import (
	"fmt"

	"github.com/hev/kit/internal/trace"
)

// spanSchema declares the spans namespace: dispatch trace facts, one row per
// ask and per attempt. Scalars only, so every store lane can hold it.
func spanSchema() map[string]any {
	schema := map[string]any{}
	for _, k := range []string{"kind", "trace_id", "run", "run_key", "publisher", "parent_span_id", "action", "ask_kind", "text", "caller", "source", "title", "repo", "branch", "host", "harness", "model", "outcome", "validation", "head", "pr", "session_id", "untraced", "validations", "requested_by"} {
		schema[k] = map[string]any{"type": "string", "filterable": false}
	}
	for _, k := range []string{"start", "attempt", "revision", "revisions", "run_start", "end", "result", "exit_code", "unconfirmed"} {
		schema[k] = map[string]any{"type": "int", "filterable": false}
	}
	return indexedSchema(schema, "spans")
}

// WriteTraceSpans upserts trace facts into the spans namespace.
func (c *Client) WriteTraceSpans(rows []trace.SpanRow) (WriteResult, error) {
	if len(rows) == 0 {
		return WriteResult{}, nil
	}
	return c.writeRows(c.Namespace+"-spans", rows, spanSchema())
}

// ListTraceSpanRows reads trace facts matching filter (nil for all), by ID
// cursor. An archive with no spans namespace yet reads as empty.
func (c *Client) ListTraceSpanRows(filter any) ([]trace.SpanRow, error) {
	var rows []trace.SpanRow
	cursor := ""
	for {
		f := filter
		if cursor != "" {
			page := []any{"id", "Gt", cursor}
			if f == nil {
				f = page
			} else {
				f = And(f, page)
			}
		}
		body := map[string]any{"rank_by": []any{"id", "asc"}, "top_k": 1000, "include_attributes": true}
		if f != nil {
			body["filters"] = f
		}
		var out struct {
			Rows  []trace.SpanRow `json:"rows"`
			Error string          `json:"error"`
		}
		err := c.do("POST", "/v2/namespaces/"+c.Namespace+"-spans/query", body, &out)
		if isNamespaceMissing(err) {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		if out.Error != "" {
			return nil, fmt.Errorf("trace spans: %s", out.Error)
		}
		for _, r := range out.Rows {
			if r.ID <= cursor {
				return nil, fmt.Errorf("trace span pagination did not advance")
			}
			cursor = r.ID
			rows = append(rows, r)
		}
		if len(out.Rows) < 1000 {
			return rows, nil
		}
	}
}
