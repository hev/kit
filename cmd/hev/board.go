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
	RunE: func(cmd *cobra.Command, args []string) error {
		args, err := workspaceRawArgs(args)
		if err != nil {
			return err
		}
		w, err := currentWorkspace()
		if err != nil {
			return err
		}
		return cli.RunWorkspace(args, w)
	},
}
