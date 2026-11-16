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
	RunE:               func(cmd *cobra.Command, args []string) error { return runBoard(args) },
}

// runBoard sends board commands to the kit API on a signed-in machine, so
// the team's board is the one read and written. `hev board serve` still
// serves this machine's own board.
func runBoard(args []string) error {
	if len(args) == 0 || args[0] != "serve" {
		if k := kitSignedIn(); k != nil {
			cli.KitAPI, cli.KitKey = k.API, k.Key
		}
	}
	return cli.Run(args)
}
