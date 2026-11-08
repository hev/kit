package main

import (
	"strings"

	"github.com/spf13/cobra"
)

func addDirectSurface() {
	addRemovedVerbs()
	rootCmd.AddCommand(newNodesCommand())
	rootCmd.AddCommand(newAdmissionCommand())
	rootCmd.AddCommand(newPeersCommand())
	traces := &cobra.Command{Use: "traces", Short: "List, inspect and search captured traces", Args: cobra.MaximumNArgs(1), RunE: runLs}
	traces.Flags().AddFlagSet(lsCmd.Flags())
	// Reparent the existing trace handlers under the sole 0.3 interface.
	for _, pair := range []struct {
		name   string
		source *cobra.Command
	}{{"list", lsCmd}, {"show", traceCmd}, {"search", queryCmd}} {
		src := pair.source
		use := strings.Fields(src.Use)
		use[0] = pair.name
		src.Use = strings.Join(use, " ")
		src.Aliases = nil
		if pair.name == "list" {
			src.Aliases = []string{"ls"}
		}
		traces.AddCommand(src)
	}
	rootCmd.AddCommand(traces)
}
