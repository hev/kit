package layer

import (
	"fmt"

	"github.com/hev/kit/internal/skillsources"
	"github.com/hev/kit/pkg/search"
)

// The skills collection (RFC 0018) is the operator's pinned shelf snapshots
// in <namespace>-skills: one row per chunk of a skill file, keyed by (source,
// commit, path, part), carrying the pack's licence and attribution. It is the
// collection RFC 0007's `--in skills` will name.

// SkillsNamespace is the skills collection beside a trace namespace.
func SkillsNamespace(namespace string) string { return namespace + "-skills" }

// SkillAttrs are the attributes every skills hit comes back with.
var SkillAttrs = []string{"text", "pack", "skill", "kind", "source", "url", "commit", "path", "part", "license", "author", "homepage"}

func skillSchema(text map[string]any) map[string]any {
	schema := scalarSchema("pack", "skill", "kind", "source", "url", "commit", "path", "part", "license", "author", "homepage")
	schema["part"] = map[string]any{"type": "int", "filterable": false}
	schema = indexedSchema(schema, "skills")
	schema["text"] = text
	return schema
}

// WriteSkillChunks upserts shelf chunks into the skills collection.
func (c *Client) WriteSkillChunks(chunks []skillsources.Chunk) (WriteResult, error) {
	var total WriteResult
	for start := 0; start < len(chunks); start += 64 {
		end := min(start+64, len(chunks))
		// The text column is embedded, so the first write must name the
		// distance metric, as trace chunks do.
		body := map[string]any{"upsert_rows": chunks[start:end], "schema": skillSchema(c.textField()), "distance_metric": "cosine_distance"}
		var out writeResponse
		if err := c.do("POST", "/v2/namespaces/"+SkillsNamespace(c.Namespace), body, &out); err != nil {
			return total, err
		}
		if out.Status != "OK" && out.Error != "" {
			return total, fmt.Errorf("layer write: %s", out.Error)
		}
		total.RowsUpserted += out.RowsUpserted
		total.EmbeddingTokens += int(out.Performance.EmbeddingTokens)
	}
	return total, nil
}

// SearchSkills runs kit's hybrid query over the skills collection.
func (c *Client) SearchSkills(query string, topK int, filter any) ([]skillsources.Chunk, error) {
	if topK <= 0 {
		topK = 10
	}
	route, err := c.Caps.SearchRoute()
	if err != nil {
		return nil, err
	}
	q := search.Query{Phrasings: []string{query}, TopK: topK, Filter: filter, Attrs: SkillAttrs}
	var body map[string]any
	if route == RouteHybridText {
		body, err = c.hybridTextBody(query, topK, filter, SkillAttrs)
	} else {
		body, err = q.Body()
	}
	if err != nil {
		return nil, err
	}
	var out struct {
		Rows    []skillsources.Chunk `json:"rows"`
		Results []struct {
			Rows []skillsources.Chunk `json:"rows"`
		} `json:"results"`
		Error string `json:"error"`
	}
	if err := c.do("POST", "/v2/namespaces/"+SkillsNamespace(c.Namespace)+"/query", body, &out); err != nil {
		if isNamespaceMissing(err) {
			return nil, nil
		}
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("layer query: %s", out.Error)
	}
	if route == RouteHybridText {
		return out.Rows, nil
	}
	if len(out.Results) == 0 {
		return nil, nil
	}
	return out.Results[0].Rows, nil
}
