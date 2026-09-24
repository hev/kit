package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "hev",
	Short: "hev kit captures and analyzes AI coding traces.",
	Long: `hev kit: take back your agency.

hev kit makes your coding agent traces searchable via a hybrid search system
built on turbopuffer and hev layer. https://hev.dev/kit

  hev up        Start the gateway, dashboard and daemon (needs TURBOPUFFER_API_KEY)
  hev           Browse traces interactively
  hev d         Start the daemon (or show status if running)
  hev s         Daemon status
  hev index     Index local transcripts into your Layer namespace
  hev find      Search them (hybrid: semantic + full-text)`,
	Args: cobra.NoArgs,
	RunE: runTUI,
}

func init() {
	rootCmd.AddCommand(dCmd)
	rootCmd.AddCommand(sCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(bucketCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(mindCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(traceCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(findCmd)
	rootCmd.AddCommand(serveCmd)

	// Hide commands that aren't part of the main write path.
	daemonCmd.Hidden = true
	mindCmd.Hidden = true
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
