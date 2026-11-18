package layer

import (
	"encoding/json"
	"fmt"
)

// ExportSuffixes are the namespaces an archive keeps, by suffix: chunks,
// blocks, sessions, evals and instruction versions.
var ExportSuffixes = []string{"", "-blocks", "-sessions", "-evals", "-instructions"}

// ExportRows pages every row of one of the archive's namespaces in id order,
// with every attribute but the vector, and hands each page to fn as raw
// JSON objects. A namespace that does not exist exports nothing.
func (c *Client) ExportRows(suffix string, page int, fn func(rows []json.RawMessage) error) error {
	if page < 1 || page > 10000 {
		page = 1000
	}
	cursor := ""
	for {
		body := map[string]any{"rank_by": []any{"id", "asc"}, "top_k": page, "exclude_attributes": []string{"vector"}}
		if cursor != "" {
			body["filters"] = []any{"id", "Gt", cursor}
		}
		var out struct {
			Rows  []json.RawMessage `json:"rows"`
			Error string            `json:"error"`
		}
		if err := c.do("POST", "/v2/namespaces/"+c.Namespace+suffix+"/query", body, &out); err != nil {
			if isNamespaceMissing(err) {
				return nil
			}
			return err
		}
		if out.Error != "" {
			return fmt.Errorf("export %s%s: %s", c.Namespace, suffix, out.Error)
		}
		if len(out.Rows) == 0 {
			return nil
		}
		if err := fn(out.Rows); err != nil {
			return err
		}
		if len(out.Rows) < page {
			return nil
		}
		var last struct {
			ID any `json:"id"`
		}
		if err := json.Unmarshal(out.Rows[len(out.Rows)-1], &last); err != nil {
			return err
		}
		next := fmt.Sprint(last.ID)
		if next <= cursor {
			return fmt.Errorf("export %s%s: pagination did not advance", c.Namespace, suffix)
		}
		cursor = next
	}
}
