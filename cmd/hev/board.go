package main

import (
	"github.com/hev/kit/internal/board/cli"
	"github.com/spf13/cobra"
)

var boardCmd = &cobra.Command{
	Use:                "board",
	Short:              "Shared searchable memory beside session traces",
	DisableFlagParsing: true,
	SilenceUsage:       true,
	SilenceErrors:      true,
	RunE:               func(cmd *cobra.Command, args []string) error { return cli.Run(args) },
}
