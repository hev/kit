package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// summarizeArchiveSession never treats an empty model answer as completion.
func summarizeArchiveSession(row trace.SessionRow, summarize func(trace.SessionRow) (string, error)) trace.SessionRow {
	if strings.TrimSpace(row.Summary) != "" {
		return row
	}
	row.Summary = ""
	row.SummaryBackfilled = false
	if strings.TrimSpace(row.FirstPrompt) == "" {
		row.FirstPrompt = row.FirstPromptShort
	}
	row.FirstPrompt = trace.RequestText(row.FirstPrompt)
	if strings.TrimSpace(row.FirstPrompt) == "" {
		row.SummaryReason = trace.InjectedSummaryReason
		return row
	}
	answer, err := summarize(row)
	summary, valid := trace.ParseSummaryAnswer(answer)
	if err != nil {
		// Keep provider diagnostics out of the archive; they can contain input.
		row.SummaryReason = "Configured summarizer failed; retry summary repair"
	} else if !valid {
		row.SummaryReason = "Configured summarizer returned no task or invalid output; retry summary repair"
	} else if trace.PreambleSummary(summary, row.FirstPrompt) {
		row.SummaryReason = "summary describes setup, not the request"
	} else {
		row.Summary = strings.Join(strings.Fields(summary), " ")
		if chars := []rune(row.Summary); len(chars) > 200 {
			row.Summary = string(chars[:200])
		}
		row.SummaryBackfilled = true
		row.SummaryReason = ""
	}
	return row
}

func runSummaryRepair(ctx context.Context, out io.Writer, cl *layer.Client, sinceText, untilText, after string, limit int, apply bool, summarize func([]trace.SessionRow) ([]string, error)) error {
	since, err := time.Parse(time.RFC3339, sinceText)
	if err != nil {
		return fmt.Errorf("summary repair requires --since RFC3339: %w", err)
	}
	until, err := time.Parse(time.RFC3339, untilText)
	if err != nil || !until.After(since) {
		return fmt.Errorf("summary repair requires --until after --since")
	}
	if apply {
		if err := cl.SummariesServed(); err != nil {
			return err
		}
	}
	rows, err := cl.SummaryPage(ctx, after, since.UnixMilli(), until.UnixMilli(), limit)
	if err != nil {
		return err
	}
	repairedRows := append([]trace.SessionRow(nil), rows...)
	// Reuse the existing bounded batch adapter rather than one model call per row.
	for start := 0; start < len(rows); start += 40 {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := start + 40
		if end > len(rows) {
			end = len(rows)
		}
		var inputs []trace.SessionRow
		positions := []int{}
		for i := start; i < end; i++ {
			row := rows[i]
			if strings.TrimSpace(row.Summary) != "" {
				continue
			}
			if strings.TrimSpace(row.FirstPrompt) == "" {
				row.FirstPrompt = row.FirstPromptShort
			}
			row.FirstPrompt = trace.RequestText(row.FirstPrompt)
			if row.FirstPrompt == "" && row.SessionID != "" {
				blocks, err := cl.ListBlockRows(row.SessionID)
				if err != nil {
					return fmt.Errorf("read summary source %s: %w", row.ID, err)
				}
				row.FirstPrompt = trace.SummarySource("", blocks)
			}
			row.FirstPromptShort = trace.ShortPrompt(row.FirstPrompt)
			if strings.TrimSpace(row.FirstPrompt) == "" {
				repairedRows[i] = summarizeArchiveSession(row, nil)
				continue
			}
			inputs = append(inputs, row)
			positions = append(positions, i)
		}
		if len(inputs) == 0 {
			continue
		}
		titles, modelErr := summarize(inputs)
		if modelErr == nil && len(titles) != len(inputs) {
			modelErr = fmt.Errorf("summarizer response count mismatch")
		}
		for j, i := range positions {
			var title string
			if modelErr == nil {
				title = titles[j]
			}
			repairedRows[i] = summarizeArchiveSession(inputs[j], func(trace.SessionRow) (string, error) { return title, modelErr })
		}
	}
	next := after
	for i, before := range rows {
		if before.ID <= next {
			return fmt.Errorf("summary cursor did not advance")
		}
		if strings.TrimSpace(before.Summary) == "" {
			repaired := repairedRows[i]
			if apply {
				if err := cl.PatchSummaryRepair(ctx, before, repaired); err != nil {
					return err
				}
			}
			// No transcript text or credentials in the preview receipt.
			if err := json.NewEncoder(out).Encode(map[string]any{"id": before.ID, "harness": before.Harness, "summary": repaired.Summary, "summary_reason": repaired.SummaryReason, "summary_backfilled": strings.TrimSpace(repaired.Summary) != "", "applied": apply}); err != nil {
				return err
			}
		}
		next = before.ID
	}
	return json.NewEncoder(out).Encode(map[string]any{"next_after": next, "complete": len(rows) < limit, "scanned": len(rows), "applied": apply})
}
