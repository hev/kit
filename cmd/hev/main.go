package main

import (
	"fmt"
	"os"

	"github.com/hev/kit/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "hev",
	Short: "Meta-harness for coding agents and clusters",
	Long: `hev kit is a meta-harness for coding agents on macOS, Linux, and Windows.
A workspace binds subscriptions and authentication to a home folder and a
private network with a cluster of hevd hosts. Collect traces, coordinate agents,
and dispatch work to the hosts that accept your repositories.

  hev up        Start an archive and capture
  hev d / s     Start capture or check daemon status
  hev dispatch  Send an order and follow its run
  hev swarm     Put an orchestrator and workers on a problem
  hev watch     Watch sources with deterministic instructions
  hev peers     Coordinate agent sessions
  hev board     Keep shared findings and decisions
  hev traces    List, inspect and search captured sessions

https://hevkit.com/docs`,
	Args: cobra.NoArgs,
	RunE: runOverview,
}

func init() {
	rootCmd.Version = version.String()
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.AddCommand(&cobra.Command{Use: "version", Args: cobra.NoArgs, Run: func(cmd *cobra.Command, _ []string) { fmt.Fprintln(cmd.OutOrStdout(), version.String()) }})
	rootCmd.Flags().Bool("json", false, "Print structured operational overview")
	rootCmd.AddCommand(dCmd)
	rootCmd.AddCommand(sCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(boardCmd)
	rootCmd.AddCommand(bucketCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(mindCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(serveCmd)

	// Hide commands that aren't part of the main write path.
	daemonCmd.Hidden = true
	mindCmd.Hidden = true
}

func main() {
	// A harness hook does as little as possible and always exits 0.
	if isHookInvocation(os.Args) {
		os.Exit(runHook(os.Args[2:], os.Stdin, os.Stdout))
	}
	// peers, peers-inbox-hook and peers-runtime may be symlinks to hev; the
	// hidden `hev peers-native` is how the helpers re-enter this binary. All
	// of them are pinned to this implementation, never HEV_PEERS_BACKEND.
	if args, ok := pinnedPeersArgs(os.Args); ok {
		c := newPeersCommand(true)
		c.SetArgs(args)
		if err := c.Execute(); err != nil {
			if exit, ok := err.(*commandExit); ok {
				os.Exit(exit.code)
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	addDirectSurface()
	groupClusterCommands(rootCmd)
	installPluginHelp(rootCmd)
	if handled, code := executePlugin(rootCmd, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	if err := rootCmd.Execute(); err != nil {
		if exit, ok := err.(*commandExit); ok {
			os.Exit(exit.code)
		}
		if msg := licenseRequiredMessage(err); msg != "" {
			fmt.Fprintln(os.Stderr, msg)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Keep the cluster workflow first.
func groupClusterCommands(root *cobra.Command) {
	root.AddGroup(
		&cobra.Group{ID: "cluster", Title: "Cluster:"},
		&cobra.Group{ID: "other", Title: "Other commands:"},
	)
	root.SetHelpCommandGroupID("other")
	root.SetCompletionCommandGroupID("other")
	for _, cmd := range root.Commands() {
		switch cmd.Name() {
		case "up", "d", "s", "dispatch", "swarm", "peers", "board", "traces", "watch":
			cmd.GroupID = "cluster"
		default:
			cmd.GroupID = "other"
		}
	}
}
