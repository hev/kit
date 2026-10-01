package layer

import "fmt"

// ArchiveIdentityRows reads only ownership coordinates, never secret text.
// Missing namespaces are empty; all other failures stop migration.
func (c *Client) ArchiveIdentityRows(suffix string) ([]Row, error) {
	var rows []Row
	cursor := ""
	for {
		body := map[string]any{"rank_by": []any{"id", "asc"}, "top_k": 1000, "include_attributes": []string{"session_id", "source_path", "host", "harness"}}
		if suffix == "-blocks" {
			body["include_attributes"] = []string{"session_id"}
		}
		if suffix == "-sessions" {
			body["include_attributes"] = []string{"session_id", "host", "harness"}
		}
		if cursor != "" {
			body["filters"] = []any{"id", "Gt", cursor}
		}
		var out struct {
			Rows  []Row  `json:"rows"`
			Error string `json:"error"`
		}
		if err := c.do("POST", "/v2/namespaces/"+c.Namespace+suffix+"/query", body, &out); err != nil {
			if isNamespaceMissing(err) {
				return rows, nil
			}
			return nil, err
		}
		if out.Error != "" {
			return nil, fmt.Errorf("archive inventory: %s", out.Error)
		}
		rows = append(rows, out.Rows...)
		if len(out.Rows) < 1000 {
			return rows, nil
		}
		next := out.Rows[len(out.Rows)-1].ID
		if next <= cursor {
			return nil, fmt.Errorf("archive inventory pagination did not advance")
		}
		cursor = next
	}
}

// DeleteSessionArchive removes content-addressed chunks as well as read-side
// blocks and titles. The exact session filter never deletes a namespace.
func (c *Client) DeleteSessionArchive(id string) error {
	for _, suffix := range []string{"", "-blocks", "-sessions"} {
		var out struct {
			Error string `json:"error"`
		}
		if err := c.do("POST", "/v2/namespaces/"+c.Namespace+suffix, map[string]any{"delete_by_filter": []any{"session_id", "Eq", id}}, &out); err != nil {
			if isNamespaceMissing(err) {
				continue
			}
			return err
		}
		if out.Error != "" {
			return fmt.Errorf("delete session archive: %s", out.Error)
		}
		rows, err := c.archiveSessionRows(suffix, id)
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			return fmt.Errorf("session %s still has rows in %s", id, suffix)
		}
	}
	return nil
}
func (c *Client) archiveSessionRows(suffix, id string) ([]Row, error) {
	var out struct {
		Rows  []Row  `json:"rows"`
		Error string `json:"error"`
	}
	err := c.do("POST", "/v2/namespaces/"+c.Namespace+suffix+"/query", map[string]any{"filters": []any{"session_id", "Eq", id}, "rank_by": []any{"id", "asc"}, "top_k": 1, "include_attributes": []string{"session_id"}}, &out)
	if isNamespaceMissing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("verify cleanup: %s", out.Error)
	}
	return out.Rows, nil
}
