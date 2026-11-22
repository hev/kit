package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hev/kit/internal/score"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newScoreCommand())
}

func newScoreCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "score",
		Short: "Pin, verify and run host-owned scorers",
		Long: `Manage host-pinned scorers and run synchronous commit evaluations (RFC 0021, RFC 0023).

  hev score pin <scorer-file>
  hev score verify <scorer-file>
  hev score run <scorer-file> --commit <rev> [--parent <rev>] [--repo <path>] [--json]

See docs/scorers.md.`,
	}

	root.AddCommand(newScorePinCommand())
	root.AddCommand(newScoreVerifyCommand())
	root.AddCommand(newScoreRunCommand())

	for _, sub := range root.Commands() {
		sub.SilenceUsage = true
	}
	return root
}

func newScorePinCommand() *cobra.Command {
	var pinsPath string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "pin <scorer-file>",
		Short: "Compute and write the host SHA-256 pin set for a scorer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ps, err := score.Pin(pinsPath, args[0])
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(ps)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "pinned %s  %s\n", ps.Scorer, ps.Pin)
			fmt.Fprintf(cmd.OutOrStdout(), "  spec     sha256:%s  %s\n", ps.Spec.SHA256, ps.Spec.Path)
			fmt.Fprintf(cmd.OutOrStdout(), "  command  sha256:%s  %s\n", ps.Command.SHA256, ps.Command.Path)
			for i, d := range ps.Data {
				fmt.Fprintf(cmd.OutOrStdout(), "  data[%d]  sha256:%s  %s\n", i, d.SHA256, d.Path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&pinsPath, "pins", "", "Host scorer pin store path (default ~/.config/hev/scorers/pins.toml)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	return cmd
}

func newScoreVerifyCommand() *cobra.Command {
	var pinsPath string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "verify <scorer-file>",
		Short: "Verify a scorer's host SHA-256 pin set",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, ps, err := score.Verify(pinsPath, args[0])
			if err != nil {
				var verr *score.PinVerificationError
				if asJSON && errors.As(err, &verr) {
					_ = json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"scorer":     verr.Scorer,
						"status":     verr.Status,
						"mismatches": verr.Mismatches,
						"error":      verr.Error(),
					})
					return &commandExit{code: 1}
				}
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"scorer": spec.Name,
					"status": "verified",
					"pin":    ps.Pin,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "verified %s  %s\n", spec.Name, ps.Pin)
			return nil
		},
	}
	cmd.Flags().StringVar(&pinsPath, "pins", "", "Host scorer pin store path (default ~/.config/hev/scorers/pins.toml)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	return cmd
}

func newScoreRunCommand() *cobra.Command {
	var scorerFlag, commit, parent, repo, pinsPath string
	var extraProtect []string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "run <scorer-file> --commit <rev>",
		Short: "Verify pin set, check protected paths, and run a metric scorer on a commit",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scorerRef, err := resolveScorerArg(args, scorerFlag)
			if err != nil {
				return err
			}
			resolvedRepo, resolvedCommit := splitRepoCommit(repo, commit)
			if strings.TrimSpace(resolvedCommit) == "" {
				return fmt.Errorf("--commit is required")
			}

			res, err := score.RunCommit(cmd.Context(), score.RunOptions{
				ScorerPath:   scorerRef,
				PinStorePath: pinsPath,
				RepoPath:     resolvedRepo,
				Commit:       resolvedCommit,
				Parent:       parent,
				ExtraProtect: extraProtect,
			})
			if err != nil {
				if asJSON {
					_ = json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"status": "error",
						"error":  err.Error(),
					})
					fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
					return &commandExit{code: 1}
				}
				return err
			}

			if asJSON {
				if encErr := json.NewEncoder(cmd.OutOrStdout()).Encode(res); encErr != nil {
					return encErr
				}
				if res.Status == score.StatusDisqualified {
					return &commandExit{code: 2}
				}
				return nil
			}

			shortCommit := res.Commit
			if len(shortCommit) > 12 {
				shortCommit = shortCommit[:12]
			}
			if res.Status == score.StatusDisqualified {
				fmt.Fprintf(cmd.OutOrStdout(), "disqualified %s @ %s: touched protected path(s): %s\n",
					res.Scorer, shortCommit, strings.Join(res.ProtectedPaths, ", "))
				return &commandExit{code: 2}
			}

			unit := ""
			if res.Unit != "" {
				unit = " " + res.Unit
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s @ %s: value=%g%s mark=%d (%s, scale=%d, %s)\n",
				res.Scorer, shortCommit, *res.Value, unit, *res.Mark, res.Direction, res.Scale, res.Pin[:min(len(res.Pin), 19)])
			return nil
		},
	}
	cmd.Flags().StringVar(&scorerFlag, "scorer", "", "Scorer file path or host scorer name (alternative to positional <scorer-file>)")
	cmd.Flags().StringVar(&commit, "commit", "", "Candidate commit revision to score (required)")
	cmd.Flags().StringVar(&parent, "parent", "", "Parent commit revision for protected-path diff check (parent...commit)")
	cmd.Flags().StringVar(&repo, "repo", "", "Git repository path (default current directory)")
	cmd.Flags().StringSliceVar(&extraProtect, "protect", nil, "Additional protected repository path/glob patterns")
	cmd.Flags().StringVar(&pinsPath, "pins", "", "Host scorer pin store path (default ~/.config/hev/scorers/pins.toml)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print structured JSON output")
	return cmd
}

func resolveScorerArg(args []string, flagVal string) (string, error) {
	flagVal = strings.TrimSpace(flagVal)
	if len(args) > 0 && flagVal != "" {
		return "", fmt.Errorf("specify <scorer-file> positionally or with --scorer, not both")
	}
	raw := flagVal
	if len(args) > 0 {
		raw = strings.TrimSpace(args[0])
	}
	if raw == "" {
		return "", fmt.Errorf("scorer file is required")
	}
	if _, err := os.Lstat(raw); err == nil {
		return raw, nil
	}
	// If no path separator was given, check ~/.config/hev/scorers/<name>/scorer.md.
	if !strings.ContainsAny(raw, `/\`) && !strings.HasSuffix(raw, ".md") {
		if dir, err := score.DefaultScorerDir(); err == nil {
			candidate := filepath.Join(dir, raw, "scorer.md")
			if _, serr := os.Lstat(candidate); serr == nil {
				return candidate, nil
			}
		}
	}
	return raw, nil
}

// splitRepoCommit supports --commit REPO@REV shorthand when --repo is omitted
// and REPO names an existing directory (ignoring git reflog syntax like HEAD@{1}).
func splitRepoCommit(repoFlag, commitFlag string) (repo, commit string) {
	repoFlag = strings.TrimSpace(repoFlag)
	commitFlag = strings.TrimSpace(commitFlag)
	if repoFlag != "" || strings.Contains(commitFlag, "@{") {
		return repoFlag, commitFlag
	}
	if before, after, ok := strings.Cut(commitFlag, "@"); ok && before != "" && after != "" {
		if info, err := os.Stat(before); err == nil && info.IsDir() {
			return before, after
		}
	}
	return repoFlag, commitFlag
}
