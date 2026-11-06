package main

import (
	"fmt"

	"github.com/hev/kit/internal/intake"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{Use: "watch", Aliases: []string{"intake"}, Short: "Watch sources and run deterministic instructions", Long: `Watch Linear statuses, mentions, or an email inbox with deterministic when clauses.
Each clause has its own dispatch, comment, or notification instructions.
Existing intake.toml files work unchanged; intake is an alias for watch.

  hev watch run --config watch.toml
  hev watch check --config watch.toml
  hev watch linear render --config watch.toml --status Ready --ticket TEST-1
  hev watch install --config watch.toml
  hev watch start|stop|status

Use hev watch linear status to inspect the local watch state.`, DisableFlagParsing: true, SilenceUsage: true, SilenceErrors: true, RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 || len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
			return cmd.Help()
		}
		if err := intake.Run(args, cmd.OutOrStdout()); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), err)
			return &commandExit{code: intake.ExitCode(err)}
		}
		return nil
	}})
}
