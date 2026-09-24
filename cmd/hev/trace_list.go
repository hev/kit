package main

import (
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/cost"
	"github.com/spf13/cobra"
	"path/filepath"
	"time"
)

func init() {
	var dir, harness, since string
	var asJSON, complete bool
	cmd := &cobra.Command{Use: "list", Short: "List local factory accounting traces, including attribution gaps", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		duration, err := parseSinceDuration(since)
		if err != nil {
			return err
		}
		rows, err := cost.ReadRows(filepath.Join(dir, "sessions.jsonl"))
		if err != nil {
			return err
		}
		cutoff := time.Now().Add(-duration).UnixMilli()
		out := []map[string]any{}
		gaps := 0
		for _, r := range rows {
			if r.End < cutoff || (harness != "" && harness != r.Harness) {
				continue
			}
			if r.Model == "" || r.Tokens == 0 || len(r.Errors) > 0 || r.AccountingError != "" {
				gaps++
			}
			b, _ := json.Marshal(r)
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				return err
			}
			m["tokens"] = r.Tokens
			out = append(out, m)
		}
		if asJSON {
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(out); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "%d traces; %d accounting/attribution gaps\n", len(out), gaps)
		}
		if complete && gaps > 0 {
			return fmt.Errorf("%d incomplete accounting traces", gaps)
		}
		return nil
	}}
	cmd.Flags().StringVar(&dir, "factory-dir", ".", "Directory containing factory-ingested sessions.jsonl")
	cmd.Flags().StringVar(&harness, "harness", "", "Filter harness (codex or claude_code)")
	cmd.Flags().StringVar(&since, "since", "7d", "Lookback window")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit accounting rows as JSON")
	cmd.Flags().BoolVar(&complete, "require-complete", false, "Fail when any selected row lacks usage/model/attribution")
	traceCmd.AddCommand(cmd)
}
