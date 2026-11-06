package main

import (
	"github.com/hev/kit/internal/intake"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{Use: "intake", Short: "Poll configured work sources with deterministic rules", DisableFlagParsing: true, RunE: func(cmd *cobra.Command, args []string) error { return intake.Run(args, cmd.OutOrStdout()) }})
}
