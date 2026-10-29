package layer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hev/kit/internal/trace"
)

// SummaryPage includes the archived prompt, unlike the Git/outcome projection.
// It scans by ID so missing summaries older than a newest-first cap are reachable.
func (c *Client) SummaryPage(ctx context.Context, after string, since, until int64, limit int) ([]trace.SessionRow, error) {
	if limit < 1 || limit > 1000 || since >= until {
		return nil, fmt.Errorf("invalid summary page bounds")
	}
	filter := And([]any{"end", "Gte", since}, []any{"start", "Lte", until})
	if after != "" {
		filter = And(filter, []any{"id", "Gt", after})
	}
	var out struct {
		Rows  []trace.SessionRow `json:"rows"`
		Error string             `json:"error"`
	}
	err := c.doContext(ctx, "POST", "/v2/namespaces/"+c.Namespace+"-sessions/query", map[string]any{"rank_by": []any{"id", "asc"}, "top_k": limit, "filters": filter, "exclude_attributes": []string{"vector"}}, &out)
	if err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("summary query: %s", out.Error)
	}
	return out.Rows, nil
}

// PatchSummaryRepair writes summary, reason and the derived display title. Guard the observed summary
// and source revision, then verify readback before acknowledging a repair.
func (c *Client) PatchSummaryRepair(ctx context.Context, before, after trace.SessionRow) error {
	if strings.TrimSpace(before.Summary) != "" {
		return fmt.Errorf("refusing to replace existing summary")
	}
	if strings.TrimSpace(after.Summary) == "" && strings.TrimSpace(after.SummaryReason) == "" {
		return fmt.Errorf("empty summary requires a reason")
	}
	if !c.Caps.Feature(FeatureConditionalWrites).usable() {
		return fmt.Errorf("conditional summary patches unsupported")
	}
	if err := c.SummariesServed(); err != nil {
		return err
	}
	current, err := c.readSessionSource(before.ID)
	if err != nil {
		return err
	}
	if len(current) == 0 {
		return fmt.Errorf("summary row disappeared: %s", before.ID)
	}
	// Display text is deliberately unindexed. Guard its coupled completion flag
	// and indexed source revision; compare the text in the preceding observation.
	// Legacy writers that ignore this contract still require final reconciliation.
	conditions := []any{}
	for _, field := range []string{"summary_backfilled", "end", "session_id", "harness"} {
		var value any
		if raw := current[field]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &value); err != nil {
				return err
			}
		}
		conditions = append(conditions, []any{field, "Eq", value})
	}
	var observed trace.SessionRow
	b, _ := json.Marshal(current)
	if err := json.Unmarshal(b, &observed); err != nil {
		return err
	}
	if observed.Summary != before.Summary || observed.End != before.End || observed.FirstPrompt != before.FirstPrompt || observed.FirstPromptShort != before.FirstPromptShort || observed.SummaryBackfilled != before.SummaryBackfilled {
		return fmt.Errorf("summary source changed: %s", before.ID)
	}
	summary := strings.Join(strings.Fields(after.Summary), " ")
	reason := after.SummaryReason
	if summary != "" {
		reason = ""
	}
	var out writeResponse
	err = c.doContext(ctx, "POST", "/v2/namespaces/"+c.Namespace+"-sessions", map[string]any{
		"patch_rows":      []any{map[string]any{"id": before.ID, "summary": summary, "summary_reason": reason, "summary_backfilled": summary != "", "first_prompt_short": trace.ShortPrompt(after.FirstPrompt)}},
		"patch_condition": []any{"And", conditions}, "schema": indexedSchema(scalarSchema("summary_reason", "summary_backfilled"), "sessions"), "return_affected_ids": true,
	}, &out)
	if err != nil {
		return err
	}
	if out.Error != "" {
		return fmt.Errorf("summary patch: %s", out.Error)
	}
	if out.RowsAffected != 1 {
		return fmt.Errorf("summary conflict for %s: affected %d", before.ID, out.RowsAffected)
	}
	stored, err := c.readSessionSource(before.ID)
	if err != nil {
		return err
	}
	var gotSummary, gotReason, gotTitle string
	var gotBackfilled bool
	json.Unmarshal(stored["summary"], &gotSummary)
	json.Unmarshal(stored["summary_reason"], &gotReason)
	json.Unmarshal(stored["first_prompt_short"], &gotTitle)
	json.Unmarshal(stored["summary_backfilled"], &gotBackfilled)
	if gotSummary != summary || gotReason != reason || gotBackfilled != (summary != "") || gotTitle != trace.ShortPrompt(after.FirstPrompt) {
		return fmt.Errorf("summary readback mismatch: %s", before.ID)
	}
	return nil
}
