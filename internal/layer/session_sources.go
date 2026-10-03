package layer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/hev/kit/internal/trace"
)

// Each row has its own at-write version condition. No new revision attributes
// are needed, and partial updates leave attributes unknown to this client intact.
func (c *Client) writeSessionSources(rows []trace.SessionRow) (WriteResult, error) {
	var result WriteResult
	if !c.Caps.ReadSide() {
		return result, nil
	}
	if !c.Caps.Feature(FeatureConditionalWrites).usable() {
		return result, fmt.Errorf("session ingestion requires conditional writes")
	}
	unlock, err := lockSessionEnrichment(false)
	if err != nil {
		return result, err
	}
	defer unlock()
	var declared map[string]map[string]any
	if err = c.do("GET", "/v1/namespaces/"+c.Namespace+"-sessions/schema", nil, &declared); err != nil && !isNamespaceMissing(err) {
		return result, err
	}
	schema := sessionSchema(c.Caps.Arrays())
	for k, v := range declared {
		schema[k] = v
	}
	typ := schema["commits"].(map[string]any)["type"]
	if typ != "string" && typ != "[]string" {
		return result, fmt.Errorf("unsupported session commits type %q; no write performed", typ)
	}
	if typ == "[]string" && !c.Caps.Arrays() {
		return result, fmt.Errorf("session commits uses unsupported arrays")
	}
	seen := map[string]bool{}
	for _, input := range rows {
		if input.ID == "" || seen[input.ID] {
			return result, fmt.Errorf("missing or duplicate session ID")
		}
		seen[input.ID] = true
	}
	for _, input := range rows {
		done := false
		for attempt := 0; attempt < 3; attempt++ {
			current, err := c.readSessionSource(input.ID)
			if err != nil {
				return result, err
			}
			row := input
			row.Commits = append(trace.StringList(nil), input.Commits...)
			existing := current != nil
			var old trace.SessionRow
			if existing {
				if err = c.summariesServedContext(context.Background()); err != nil {
					return result, err
				}
				typedLinkage := map[string]json.RawMessage{}
				for _, k := range []string{"id", "session_id", "end", "commits", "pr", "workdir", "repo_url", "branch"} {
					if raw, ok := current[k]; ok {
						typedLinkage[k] = raw
					}
				}
				b, _ := json.Marshal(typedLinkage)
				if err = json.Unmarshal(b, &old); err != nil {
					return result, err
				}
				if old.End > input.End {
					done = true
					break
				} // An already newer source is not rolled back.
				if old.SessionID != "" && input.SessionID != "" && old.SessionID != input.SessionID {
					return result, fmt.Errorf("session identity conflict: %s", input.ID)
				}
				if row.SessionID == "" {
					row.SessionID = old.SessionID
				}
				// Linkage can advance with capture, but populated linkage belongs to the
				// current row. Its exact values join the conditional merge below.
				if old.PR != "" {
					row.PR = old.PR
				}
				if old.Workdir != "" {
					row.Workdir = old.Workdir
				}
				if old.RepoURL != "" {
					row.RepoURL = old.RepoURL
				}
				if old.Branch != "" {
					row.Branch = old.Branch
				}
				row.Commits = append(append(trace.StringList{}, input.Commits...), old.Commits...)
			}
			sort.Strings(row.Commits)
			unique := row.Commits[:0]
			for _, sha := range row.Commits {
				if len(unique) == 0 || unique[len(unique)-1] != sha {
					unique = append(unique, sha)
				}
			}
			row.Commits = unique
			b, err := json.Marshal(row)
			if err != nil {
				return result, err
			}
			obj := map[string]json.RawMessage{}
			if err = json.Unmarshal(b, &obj); err != nil {
				return result, err
			}
			counts, _ := json.Marshal(row.ToolCounts)
			obj["tool_counts"], _ = json.Marshal(string(counts))
			if existing {
				delete(obj, "summary")
				// Never resend populated linkage that capture did not advance.
				// Unchanged fields need no filterability or at-write merge guard.
				for _, k := range []string{"pr", "workdir", "repo_url", "branch"} {
					var value string
					json.Unmarshal(obj[k], &value)
					var storedValue string
					json.Unmarshal(current[k], &storedValue)
					if value == storedValue {
						delete(obj, k)
					}
				}
				if reflect.DeepEqual(row.Commits, old.Commits) || (len(row.Commits) == 0 && len(old.Commits) == 0) {
					delete(obj, "commits")
				}

				outcomeBytes, _ := json.Marshal(row.SessionOutcomes)
				var outcomes map[string]json.RawMessage
				json.Unmarshal(outcomeBytes, &outcomes)
				// Empty outcomes still have an ownership contract; remove every static
				// outcome and any flattened workflow/commit keys from ingestion patches.
				for k := range outcomeSchema(c.Caps.Arrays()) {
					delete(obj, k)
				}
				for k := range outcomes {
					delete(obj, k)
				}
				for k := range obj {
					if k == "outcome_details" || strings.HasPrefix(k, "ci_workflow_") || strings.HasPrefix(k, "commit_outcome_") {
						delete(obj, k)
					}
				}
			}
			writeSchema := map[string]any{}
			for k, raw := range obj {
				if k == "id" {
					continue
				}
				if def, ok := schema[k]; ok {
					writeSchema[k] = def
					if def.(map[string]any)["type"] == "string" && len(raw) > 0 && (raw[0] == '[' || raw[0] == '{') {
						obj[k], _ = json.Marshal(string(raw))
					}
				}
			}
			body := map[string]any{"schema": writeSchema, "return_affected_ids": true}
			if existing {
				// Exact source end and linkage guards reject a concurrent source advance
				// or enrichment fill; outcome-only changes need no guard because omitted.
				conditions := []any{[]any{"id", "Eq", input.ID}}
				keys := []string{"end", "start", "tool_count", "session_id"}
				for _, k := range []string{"commits", "pr", "workdir", "repo_url", "branch"} {
					if _, changed := obj[k]; changed {
						keys = append(keys, k)
					}
				}
				for _, k := range keys {
					var value any
					if raw := current[k]; len(raw) > 0 {
						d := json.NewDecoder(bytes.NewReader(raw))
						d.UseNumber()
						if err = d.Decode(&value); err != nil {
							return result, err
						}
					}
					conditions = append(conditions, []any{k, "Eq", value})
				}
				body["patch_rows"] = []any{obj}
				body["patch_condition"] = []any{"And", conditions}
			} else {
				body["upsert_rows"] = []any{obj}
				body["upsert_condition"] = []any{"id", "Eq", nil}
			}
			var out writeResponse
			if err = c.do("POST", "/v2/namespaces/"+c.Namespace+"-sessions", body, &out); err != nil {
				return result, err
			}
			if out.Error != "" {
				return result, fmt.Errorf("session source write: %s", out.Error)
			}
			affected := out.RowsAffected
			if affected == 0 {
				continue
			} // Lost absent-ID/source race: read fresh before retry.
			if affected != 1 {
				return result, fmt.Errorf("session source write affected %d rows for %s", affected, input.ID)
			}
			if len(out.PatchedIDs) > 0 && (len(out.PatchedIDs) != 1 || out.PatchedIDs[0] != input.ID) {
				return result, fmt.Errorf("session source affected ID mismatch")
			}
			readback, err := c.readSessionSource(input.ID)
			if err != nil {
				return result, err
			}
			if readback == nil {
				return result, fmt.Errorf("session source readback missing: %s", input.ID)
			}
			for k, want := range obj {
				var a, b any
				da := json.NewDecoder(bytes.NewReader(want))
				da.UseNumber()
				if err = da.Decode(&a); err != nil {
					return result, err
				}
				db := json.NewDecoder(bytes.NewReader(readback[k]))
				db.UseNumber()
				if len(readback[k]) > 0 {
					if err = db.Decode(&b); err != nil {
						return result, err
					}
				}
				if !reflect.DeepEqual(a, b) {
					return result, fmt.Errorf("session source readback conflict on %s/%s", input.ID, k)
				}
			}
			result.RowsUpserted++
			result.EmbeddingTokens += int(out.Performance.EmbeddingTokens)
			done = true
			break
		}
		if !done {
			return result, fmt.Errorf("session source conflict after 3 attempts: %s", input.ID)
		}
	}
	return result, nil
}

func (c *Client) readSessionSource(id string) (map[string]json.RawMessage, error) {
	var out struct {
		Rows  []map[string]json.RawMessage `json:"rows"`
		Error string                       `json:"error"`
	}
	err := c.do("POST", "/v2/namespaces/"+c.Namespace+"-sessions/query", map[string]any{"filters": []any{"id", "In", []string{id}}, "rank_by": []any{"id", "asc"}, "top_k": 1, "exclude_attributes": []string{"vector"}}, &out)
	if isNamespaceMissing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("session source read: %s", out.Error)
	}
	if len(out.Rows) == 0 {
		return nil, nil
	}
	if len(out.Rows) != 1 {
		return nil, fmt.Errorf("session source duplicate ID")
	}
	var returnedID string
	if err = json.Unmarshal(out.Rows[0]["id"], &returnedID); err != nil || returnedID != id {
		return nil, fmt.Errorf("session source returned wrong ID")
	}
	return out.Rows[0], nil
}
