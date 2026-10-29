package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hev/kit/internal/trace"
	"github.com/spf13/cobra"
)

func init() { rootCmd.AddCommand(newSkillsPreviewCommand()) }

func newSkillsPreviewCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "skills-preview", Short: "Preview skill backfill from local transcripts (never writes to a store)", Args: cobra.NoArgs}
	roots := map[string]*string{}
	for _, h := range []string{"claude_code", "codex", "pi", "dot"} {
		roots[h] = cmd.Flags().String(h, "", "Transcript root for "+h+" (only explicit roots are scanned)")
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		sources := map[string]trace.Source{"claude_code": &trace.ClaudeSource{Root: *roots["claude_code"]}, "codex": &trace.CodexSource{Root: *roots["codex"]}, "pi": &trace.PiSource{Root: *roots["pi"]}, "dot": &trace.DotSource{Root: *roots["dot"]}}
		type report struct {
			Harness         string           `json:"harness"`
			Sessions        int              `json:"sessions"`
			WouldGainSkills int              `json:"would_gain_skills"`
			Rows            []map[string]any `json:"rows"`
		}
		reports := []report{}
		for _, h := range []string{"claude_code", "codex", "pi", "dot"} {
			if *roots[h] == "" {
				continue
			}
			if _, err := os.Stat(*roots[h]); err != nil {
				return fmt.Errorf("%s root: %w", h, err)
			}
			src := sources[h]
			units, err := src.Units()
			if err != nil {
				return err
			}
			var turns []trace.Turn
			for _, u := range units {
				t, err := src.Read(u)
				if err != nil {
					return fmt.Errorf("%s %s: %w", h, u.Key, err)
				}
				turns = append(turns, t...)
			}
			rows := trace.Sessions(turns, nil, "")
			r := report{Harness: h, Sessions: len(rows), Rows: []map[string]any{}}
			for _, row := range rows {
				if len(row.Skills) > 0 {
					r.WouldGainSkills++
					r.Rows = append(r.Rows, map[string]any{"session_id": row.SessionID, "skills": row.Skills})
				}
			}
			reports = append(reports, r)
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(reports)
	}
	return cmd
}
