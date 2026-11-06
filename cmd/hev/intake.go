package main

import (
	"fmt"
	"github.com/hev/kit/internal/intake"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{Use: "intake", Short: "Poll configured work sources with deterministic rules", DisableFlagParsing: true, SilenceUsage: true, SilenceErrors: true, RunE: func(cmd *cobra.Command, args []string) error {
		if err := intake.Run(args, cmd.OutOrStdout()); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), err)
			return &resourceBackendExit{code: intake.ExitCode(err)}
		}
		return nil
	}})
}
