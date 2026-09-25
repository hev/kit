package main

import (
	"github.com/spf13/cobra"
)

var mindCmd = &cobra.Command{
	Use:   "mind",
	Short: "Book a session with a human",
	Long:  "Open the consulting booking page in your browser.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return openURL(cmd.OutOrStdout(), "https://calendar.app.google/6aa1yKnGsAhmk5Se8")
	},
}
