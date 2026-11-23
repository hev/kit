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

func runBoard(args []string) error {
	args, err := workspaceRawArgs(args)
	if err != nil {
		return err
	}
	w, err := currentWorkspace()
	if err != nil {
		return err
	}
	// The process sign-in belongs to the default board. A workspace with
	// isolated storage uses its own board configuration.
	if !w.IsolatedStorage() && (len(args) == 0 || args[0] != "serve") {
		if k := kitSignedIn(); k != nil {
			client := kitClient(k)
			client.Retries = 0
			cli.KitClient = client
			defer func() { cli.KitClient = nil }()
		}
	}
	return cli.RunWorkspace(args, w)
}
