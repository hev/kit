package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// Resource groups delegate to one selected implementation. Existing backends
// retain their authentication and transport boundaries.
func coordinationCommand(resource string) *cobra.Command {
	cmd := &cobra.Command{Use: resource + " [list|show|create|claim|assign] [flags]", Short: "Inspect and explicitly manage " + resource, DisableFlagParsing: true, SilenceUsage: true}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
			return cmd.Help()
		}
		backend := os.Getenv("HEV_COORDINATION_BACKEND")
		if backend == "" {
			backend = "hev-control"
		}
		child := exec.CommandContext(cmd.Context(), backend, append([]string{resource}, args...)...)
		child.Stdin = cmd.InOrStdin()
		child.Stdout = cmd.OutOrStdout()
		child.Stderr = cmd.ErrOrStderr()
		if err := child.Run(); err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				return &resourceBackendExit{exit.ExitCode()}
			}
			return errors.New("coordination backend unavailable; install hev-control or select HEV_COORDINATION_BACKEND")
		}
		return nil
	}
	return cmd
}
func addDirectSurface() {
	for _, resource := range []string{"nodes", "orders", "runs"} {
		rootCmd.AddCommand(coordinationCommand(resource))
	}
	peers := &cobra.Command{Use: "peers [list|show|send]", Short: "Inspect and communicate with live agent sessions", DisableFlagParsing: true}
	peers.RunE = func(cmd *cobra.Command, args []string) error {
		backend := os.Getenv("HEV_PEERS_BACKEND")
		if backend == "" {
			backend = "peers"
		}
		if len(args) == 0 {
			args = []string{"ls"}
		} else {
			switch args[0] {
			case "list":
				args[0] = "ls"
			case "show":
				args[0] = "read"
			}
		}
		child := exec.CommandContext(cmd.Context(), backend, args...)
		child.Stdin = cmd.InOrStdin()
		child.Stdout = cmd.OutOrStdout()
		child.Stderr = cmd.ErrOrStderr()
		return child.Run()
	}
	rootCmd.AddCommand(peers)
	traces := &cobra.Command{Use: "traces", Short: "List, inspect and search captured traces", Args: cobra.MaximumNArgs(1), RunE: runLs}
	traces.Flags().AddFlagSet(lsCmd.Flags())
	// The same handlers and flags implement both new and compatibility names.
	for _, pair := range []struct {
		name   string
		source *cobra.Command
	}{{"list", lsCmd}, {"show", traceCmd}, {"search", queryCmd}} {
		src := pair.source
		use := strings.Fields(src.Use)
		use[0] = pair.name
		cmd := &cobra.Command{Use: strings.Join(use, " "), Short: src.Short, Long: src.Long, Args: src.Args, RunE: src.RunE}
		cmd.Flags().AddFlagSet(src.Flags())
		traces.AddCommand(cmd)
	}
	rootCmd.AddCommand(traces)
}
