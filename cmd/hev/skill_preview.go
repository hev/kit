package main

import (
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/trace"
	"github.com/spf13/cobra"
)

// This command deliberately has no client, credentials, or apply mode.
func init() {
	rootCmd.AddCommand(&cobra.Command{Use: "skills-preview HARNESS TRANSCRIPT...", Short: "Preview skill backfill from local transcripts (no store access)", Args: cobra.MinimumNArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		var src trace.Source
		switch args[0] {
		case "claude_code":
			src = &trace.ClaudeSource{}
		case "codex":
			src = &trace.CodexSource{}
		case "pi":
			src = &trace.PiSource{}
		case "dot":
			src = &trace.DotSource{}
		default:
			return fmt.Errorf("unknown harness %q", args[0])
		}
		var turns []trace.Turn
		for _, p := range args[1:] {
			rows, err := src.Read(trace.Unit{Key: p})
			if err != nil {
				return err
			}
			turns = append(turns, rows...)
		}
		sessions := trace.Sessions(turns, nil, "")
		gain := 0
		for _, s := range sessions {
			if len(s.Skills) > 0 {
				gain++
			}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"harness": args[0], "sessions_scanned": len(sessions), "sessions_with_skills": gain, "sessions": sessions})
	}})
}
