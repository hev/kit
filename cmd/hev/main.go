package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hev/kit/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "hev",
	Short: "Make your machine part of a software factory",
	Long: `hev kit is what you need to make your machine part of a software factory.
One binary collects traces, dispatches work to your hosts, and controls which
repos each host accepts. Add headless machines locally or in the cloud.

  hev up        Start an archive and capture
  hev d / s     Start capture or check daemon status
  hev dispatch  Send an order and follow its run
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
	if strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") == "peers" {
		c := newPeersCommand()
		c.SetArgs(os.Args[1:])
		if err := c.Execute(); err != nil {
			os.Exit(1)
		}
		return
	}
	addCoreCommands()
	groupFactoryCommands(rootCmd)
	installPluginHelp(rootCmd)
	if handled, code := executePlugin(rootCmd, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	if err := rootCmd.Execute(); err != nil {
		if exit, ok := err.(*resourceBackendExit); ok {
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

// Keep the factory workflow first while preserving the resource bridge.
func groupFactoryCommands(root *cobra.Command) {
	root.AddGroup(
		&cobra.Group{ID: "factory", Title: "Factory:"},
		&cobra.Group{ID: "other", Title: "Other commands:"},
		&cobra.Group{ID: "legacy", Title: "Legacy:"},
	)
	root.SetHelpCommandGroupID("other")
	root.SetCompletionCommandGroupID("other")
	for _, cmd := range root.Commands() {
		switch cmd.Name() {
		case "up", "d", "s", "dispatch", "peers", "board", "traces", "watch":
			cmd.GroupID = "factory"
		case "apply", "get", "describe", "controller", "logs", "wait", "api-resources":
			cmd.GroupID = "legacy"
		default:
			cmd.GroupID = "other"
		}
	}
}
