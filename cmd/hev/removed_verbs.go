package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Verbs that once delegated to external resource and coordination backends.
// They stay registered, hidden, so a stale script gets a clear error rather
// than an unknown-command guess or an executable found on PATH.
var removedVerbs = []string{"apply", "get", "describe", "logs", "wait", "api-resources", "controller", "orders", "runs"}

type commandExit struct{ code int }

func (e *commandExit) Error() string { return fmt.Sprintf("exited %d", e.code) }

func removedVerbCommand(verb string) *cobra.Command {
	return &cobra.Command{
		Use:                verb,
		Hidden:             true,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(*cobra.Command, []string) error {
			return fmt.Errorf("hev %s has been removed; use hev dispatch or hev orchestrate", verb)
		},
	}
}

func addRemovedVerbs() {
	for _, verb := range removedVerbs {
		rootCmd.AddCommand(removedVerbCommand(verb))
	}
}
