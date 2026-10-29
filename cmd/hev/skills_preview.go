package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hev/kit/internal/trace"
	"github.com/spf13/cobra"
)

// This command deliberately has no backend client or apply flag. It reparses
// local evidence, reports coverage, and can export a reviewable read-side plan.
func newSkillsPreviewCmd() *cobra.Command {
	var claude, codex, pi, dot, existing, output string
	cmd := &cobra.Command{Use: "skills-preview", Short: "Preview skill backfill from local transcripts (never writes a backend)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseline := map[string]trace.SessionRow{}
			if existing != "" {
				b, err := os.ReadFile(existing)
				if err != nil {
					return err
				}
				var rows []trace.SessionRow
				if err = json.Unmarshal(b, &rows); err != nil {
					return err
				}
				for _, r := range rows {
					baseline[r.SessionID] = r
				}
			}
			sources := []trace.Source{&trace.ClaudeSource{Root: claude}, &trace.CodexSource{Root: codex}, &trace.PiSource{Root: pi}, &trace.DotSource{Root: dot}}
			type coverage struct {
				Sessions   int `json:"sessions"`
				WithSkills int `json:"with_skills"`
				WouldGain  int `json:"would_gain"`
			}
			counts := map[string]coverage{"claude_code": {}, "codex": {}, "pi": {}, "dot": {}}
			var turns []trace.Turn
			for _, src := range sources {
				units, err := src.Units()
				if err != nil {
					return err
				}
				for _, u := range units {
					ts, err := src.Read(u)
					if err != nil {
						return fmt.Errorf("%s: %w", u.Key, err)
					}
					turns = append(turns, ts...)
				}
			}
			rows := trace.Sessions(turns, nil, "")
			for _, r := range rows {
				n := counts[r.Harness]
				n.Sessions++
				if len(r.Skills) > 0 {
					n.WithSkills++
					old := baseline[r.SessionID]
					gain := false
					for _, name := range r.Skills {
						found := false
						for _, v := range old.Skills {
							if v == name {
								found = true
							}
						}
						if !found {
							gain = true
						}
					}
					if gain {
						n.WouldGain++
					}
				}
				counts[r.Harness] = n
			}
			if output != "" {
				b, err := json.MarshalIndent(struct {
					Sessions []trace.SessionRow `json:"sessions"`
					Blocks   []trace.BlockRow   `json:"blocks"`
				}{rows, trace.Blocks(turns)}, "", "  ")
				if err != nil {
					return err
				}
				f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
				if err != nil {
					return err
				}
				_, err = f.Write(b)
				closeErr := f.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(counts)
		}}
	cmd.Flags().StringVar(&claude, "claude-root", trace.DefaultClaudeRoot(), "Claude transcript root")
	cmd.Flags().StringVar(&codex, "codex-root", trace.DefaultCodexRoot(), "Codex transcript root")
	cmd.Flags().StringVar(&pi, "pi-root", trace.DefaultPiRoot(), "PI transcript root")
	cmd.Flags().StringVar(&dot, "dot-root", trace.DefaultDotRoot(), "dot transcript root")
	cmd.Flags().StringVar(&existing, "existing-sessions", "", "Optional local JSON session export to compare; otherwise assumes no captured skills")
	cmd.Flags().StringVar(&output, "out", "", "Create a local JSON read-side candidate (must not exist)")
	return cmd
}
func init() { rootCmd.AddCommand(newSkillsPreviewCmd()) }
