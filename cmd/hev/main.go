package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "hev",
	Short: "hev manages resources and captures and analyzes AI coding traces.",
	Long: `hev kit: take back your agency.

hev kit makes your coding agent traces searchable via a hybrid search system
built on hev layer: free and local by default, on turbopuffer with a key.
https://hev.dev/kit

  hev api-resources  Discover implemented resource operations
  hev apply/get/describe  Manage versioned resources (backend required)
  hev logs/watch/wait    Observe resources when supported by the backend

  hev up        Start the gateway, store, dashboard and daemon (no key needed;
                TURBOPUFFER_API_KEY selects the hosted lane)
  hev           Read-only operational overview (--json)
  hev nodes/orders/peers  Inspect section details
  hev browse    Browse traces interactively
  hev d         Start the daemon (or show status if running)
  hev s         Daemon status
  hev index     Index local transcripts into your Layer namespace
  hev traces search  Search them (hybrid: semantic + full-text)
  hev pro       The gateway's edition, and what hev layer pro adds`,
	Args: cobra.NoArgs,
	RunE: runOverview,
}

func init() {
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
	addDirectSurface()
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
