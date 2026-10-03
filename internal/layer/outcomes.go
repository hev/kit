package layer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/trace"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"reflect"
	"sort"
)

func outcomeSchema(arrays bool) map[string]any {
	s := scalarSchema("pr_state", "pr_merged", "pr_closed", "pr_url", "ci_state", "reverted", "revert_state", "revert_until", "outcome_checked")
	for _, attr := range s {
		attr.(map[string]any)["filterable"] = true
	}
	for _, k := range []string{"revert_until", "outcome_checked"} {
		s[k] = map[string]any{"type": "int", "filterable": true}
	}
	for _, k := range []string{"ci_conclusions", "ci_runs", "revert_commits"} {
		t := "string"
		if arrays {
			t = "[]string"
		}
		s[k] = map[string]any{"type": t, "filterable": true}
	}
	return s
}

// OutcomePage is a bounded ID cursor scan, including unchanged transcripts.
func (c *Client) OutcomePage(ctx context.Context, after string, since int64, limit int) ([]trace.SessionRow, error) {
	return c.BackfillPage(ctx, after, since, 0, limit)
}

// BackfillPage is a bounded read-only scan with a fixed event-time upper bound.
func (c *Client) BackfillPage(ctx context.Context, after string, since, until int64, limit int) ([]trace.SessionRow, error) {
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("outcome page limit must be 1..1000")
	}
	f := any([]any{"end", "Gte", since})
	if until > 0 {
		f = And(f, []any{"start", "Lte", until})
	}
	if after != "" {
		f = And(f, []any{"id", "Gt", after})
	}
	var out struct {
		Rows  []trace.SessionRow `json:"rows"`
		Error string             `json:"error"`
	}
	err := c.doContext(ctx, "POST", "/v2/namespaces/"+c.Namespace+"-sessions/query", map[string]any{"rank_by": []any{"id", "asc"}, "top_k": limit, "filters": f, "exclude_attributes": []string{"first_prompt", "vector"}}, &out)
	if isNamespaceMissing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("outcome query: %s", out.Error)
	}
	return out.Rows, nil
}

// PatchOutcomes touches only enrichment, never transcript or summaries.
func (c *Client) PatchOutcomes(ctx context.Context, row trace.SessionRow) error {
	return c.patchOutcomes(ctx, row, false)
}

// PatchBackfill also fills explicit source linkage without rewriting transcript data.
// Uses an at-write condition on current attributes; later whole-row writers are
// not fenced. Final persisted reconciliation remains required.
func (c *Client) PatchBackfill(ctx context.Context, row trace.SessionRow) error {
	return c.patchOutcomes(ctx, row, true)
}

func (c *Client) patchOutcomes(ctx context.Context, row trace.SessionRow, linkage bool) error {
	unlock, e := lockSessionEnrichment(true)
	if e != nil {
		return e
	}
	defer unlock()
	if err := c.summariesServedContext(ctx); err != nil {
		return err
	}
	var declared map[string]map[string]any
	if err := c.doContext(ctx, "GET", "/v1/namespaces/"+c.Namespace+"-sessions/schema", nil, &declared); err != nil && !isNamespaceMissing(err) {
		return err
	}
	stored, err := c.storedSessionEnrichmentContext(ctx, []string{row.ID})
	if err != nil {
		return err
	}
	if linkage {
		if current, ok := stored[row.ID]; ok && current.PR != "" && row.PR != "" && current.PR != row.PR {
			return fmt.Errorf("backfill PR changed since export")
		}
		if _, ok := stored[row.ID]; !ok {
			return fmt.Errorf("backfill row disappeared: %s", row.ID)
		}
	}
	seen := map[string]bool{}
	for _, sha := range row.Commits {
		seen[sha] = true
	}
	for _, sha := range stored[row.ID].Commits {
		if !seen[sha] {
			row.Commits = append(row.Commits, sha)
			seen[sha] = true
		}
	}
	sort.Strings(row.Commits)
	if row.PR == "" {
		row.PR = stored[row.ID].PR
	}
	b, err := json.Marshal(row.SessionOutcomes)
	if err != nil {
		return err
	}
	var patch map[string]any
	if err = json.Unmarshal(b, &patch); err != nil {
		return err
	}
	for k, v := range row.WorkflowAttributes {
		patch[k] = v
	}
	patch["id"] = row.ID
	patch["pr"] = row.PR
	patch["commits"] = row.Commits
	s := outcomeSchema(c.Caps.Arrays())
	for k := range row.WorkflowAttributes {
		s[k] = map[string]any{"type": "string", "filterable": true}
	}
	if linkage {
		for k, v := range map[string]string{"workdir": row.Workdir, "repo_url": row.RepoURL, "branch": row.Branch} {
			current := stored[row.ID]
			existing := map[string]string{"workdir": current.Workdir, "repo_url": current.RepoURL, "branch": current.Branch}
			if v != "" && existing[k] == "" {
				patch[k] = v
				definition := map[string]any{"type": "string"}
				for key, value := range declared[k] {
					definition[key] = value
				}
				definition["filterable"] = true
				s[k] = definition
			}
		}
	}
	s["pr"] = map[string]any{"type": "string", "filterable": true}
	for _, k := range []string{"commits", "ci_conclusions", "ci_runs", "revert_commits"} {
		typ := "string"
		if c.Caps.Arrays() {
			typ = "[]string"
		}
		if v, ok := declared[k]; ok {
			typ, _ = v["type"].(string)
		}
		if typ != "string" && typ != "[]string" {
			return fmt.Errorf("unsupported outcome attribute %s type %q", k, typ)
		}
		s[k] = map[string]any{"type": typ, "filterable": true}
		if typ == "string" {
			b, _ := json.Marshal(patch[k])
			patch[k] = string(b)
		}
	}

	// Preserve every existing schema option, including scalar outcome settings.
	for k, definition := range s {
		if previous, ok := declared[k]; ok {
			merged := map[string]any{}
			for key, value := range previous {
				merged[key] = value
			}
			for key, value := range definition.(map[string]any) {
				if key == "filterable" || merged[key] == nil {
					merged[key] = value
				}
			}
			s[k] = merged
		}
	}
	body := map[string]any{"patch_rows": []any{patch}, "schema": s}
	if linkage {
		if !c.Caps.Feature(FeatureConditionalWrites).usable() {
			return fmt.Errorf("store does not support conditional backfill patches")
		}
		// Raw values retain absent/null and legacy list encoding for exact conditions.
		var fresh struct {
			Rows []map[string]json.RawMessage `json:"rows"`
		}
		if err = c.doContext(ctx, "POST", "/v2/namespaces/"+c.Namespace+"-sessions/query", map[string]any{"filters": []any{"id", "Eq", row.ID}, "rank_by": []any{"id", "asc"}, "top_k": 1, "exclude_attributes": []string{"first_prompt", "vector"}}, &fresh); err != nil {
			return err
		}
		if len(fresh.Rows) != 1 {
			return fmt.Errorf("backfill row disappeared: %s", row.ID)
		}
		current := fresh.Rows[0]
		// Reject changes since the typed merge read, rather than guarding a stale patch
		// against newer values. Exact raw read also protects empty-only linkage fills.
		raw, _ := json.Marshal(current)
		var freshRow trace.SessionRow
		if err = json.Unmarshal(raw, &freshRow); err != nil {
			return err
		}
		if !reflect.DeepEqual(freshRow, stored[row.ID]) {
			return fmt.Errorf("backfill concurrent change: %s", row.ID)
		}
		conditions := []any{[]any{"id", "Eq", row.ID}}
		keys := map[string]bool{}
		for k := range patch {
			keys[k] = true
		}
		for _, k := range []string{"end"} {
			keys[k] = true
		}
		for k := range keys {
			var value any
			if b := current[k]; len(b) > 0 {
				decoder := json.NewDecoder(bytes.NewReader(b))
				decoder.UseNumber()
				if err = decoder.Decode(&value); err != nil {
					return err
				}
			}
			conditions = append(conditions, []any{k, "Eq", value})
		}
		body["patch_condition"] = []any{"And", conditions}
		body["return_affected_ids"] = true
	}
	var out writeResponse
	if err = c.doContext(ctx, "POST", "/v2/namespaces/"+c.Namespace+"-sessions", body, &out); err != nil {
		return err
	}
	if out.Error != "" {
		return fmt.Errorf("outcome patch: %s", out.Error)
	}
	if linkage {
		if out.PatchedIDs != nil && (len(out.PatchedIDs) != 1 || out.PatchedIDs[0] != row.ID) {
			return fmt.Errorf("backfill affected IDs mismatch: %v", out.PatchedIDs)
		}
		if out.RowsAffected != 1 {
			return fmt.Errorf("backfill patch acknowledged %d rows, expected one", out.RowsAffected)
		}
		observed, err := c.storedSessionEnrichmentContext(ctx, []string{row.ID})
		if err != nil {
			return err
		}
		result, ok := observed[row.ID]
		if !ok {
			return fmt.Errorf("backfill readback row missing")
		}
		b, _ := json.Marshal(result)
		var actual map[string]any
		json.Unmarshal(b, &actual)
		for k, v := range result.WorkflowAttributes {
			actual[k] = v
		}
		for k, want := range patch {
			if k == "commits" || k == "ci_conclusions" || k == "ci_runs" || k == "revert_commits" {
				if text, ok := want.(string); ok {
					var list any
					if err := json.Unmarshal([]byte(text), &list); err != nil {
						return err
					}
					want = list
				}
			}
			a, _ := json.Marshal(actual[k])
			b, _ := json.Marshal(want)
			if string(a) != string(b) {
				return fmt.Errorf("backfill readback conflict on %s", k)
			}
		}
	}
	return nil
}

// A distinct local lock also works inside archive migration's outer lock.
// Sweep uses nonblocking acquisition and retries on the next cycle.
func lockSessionEnrichment(nonblocking bool) (func(), error) {
	path, e := redact.ConfigPath()
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	fd, e := unix.Open(path+".sessions.lock", unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	flags := unix.LOCK_EX
	if nonblocking {
		flags |= unix.LOCK_NB
	}
	if e = unix.Flock(fd, flags); e != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("session enrichment writer busy: %w", e)
	}
	return func() { unix.Flock(fd, unix.LOCK_UN); unix.Close(fd) }, nil
}

// CurrentBackfillRow reads current linkage before producing a new observation.
func (c *Client) CurrentBackfillRow(ctx context.Context, id string) (trace.SessionRow, error) {
	rows, err := c.storedSessionEnrichmentContext(ctx, []string{id})
	if err != nil {
		return trace.SessionRow{}, err
	}
	r, ok := rows[id]
	if !ok {
		return r, fmt.Errorf("backfill row disappeared: %s", id)
	}
	return r, nil
}
