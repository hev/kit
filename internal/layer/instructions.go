package layer

import "fmt"

// InstructionVersion stores one row per observed version, without duplicating
// file content outside the ordinary embedded L1 chunks.
type InstructionVersion struct {
	ID          string   `json:"id"`
	Path        string   `json:"path"`
	Project     string   `json:"project"`
	Host        string   `json:"host"`
	Mtime       string   `json:"mtime"`
	ContentHash string   `json:"content_hash"`
	ValidFrom   string   `json:"valid_from"`
	ValidTo     string   `json:"valid_to"`
	ChunkIDs    []string `json:"chunk_ids"`
}

func (c *Client) InstructionVersions(path, host string) ([]InstructionVersion, error) {
	var rows []InstructionVersion
	cursor := ""
	for {
		filter := And([]any{"path", "Eq", path}, []any{"host", "Eq", host})
		if cursor != "" {
			filter = And(filter, []any{"id", "Gt", cursor})
		}
		var out struct {
			Rows  []InstructionVersion `json:"rows"`
			Error string               `json:"error"`
		}
		err := c.do("POST", "/v2/namespaces/"+c.Namespace+"-instructions/query", map[string]any{"filters": filter, "rank_by": []any{"id", "asc"}, "top_k": 1000, "include_attributes": true}, &out)
		if isNamespaceMissing(err) {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		if out.Error != "" {
			return nil, fmt.Errorf("instruction versions: %s", out.Error)
		}
		for _, r := range out.Rows {
			if r.ID <= cursor {
				return nil, fmt.Errorf("instruction pagination did not advance")
			}
			cursor = r.ID
			rows = append(rows, r)
		}
		if len(out.Rows) < 1000 {
			return rows, nil
		}
	}
}
func (c *Client) WriteInstructionVersions(rows []InstructionVersion) error {
	schema := map[string]any{}
	for _, k := range []string{"path", "project", "host", "mtime", "content_hash", "valid_from", "valid_to"} {
		schema[k] = map[string]any{"type": "string", "filterable": true}
	}
	schema["chunk_ids"] = map[string]any{"type": "[]string", "filterable": false}
	_, err := c.writeRows(c.Namespace+"-instructions", rows, schema)
	return err
}
