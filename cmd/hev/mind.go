package main

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

var mindCmd = &cobra.Command{
	Use:   "mind",
	Short: "Book a session with a human",
	Long:  "Open the consulting booking page in your browser.",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := "https://calendar.app.google/6aa1yKnGsAhmk5Se8"
		var c *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			c = exec.Command("open", url)
		case "linux":
			c = exec.Command("xdg-open", url)
		default:
			fmt.Printf("Open this URL in your browser:\n  %s\n", url)
			return nil
		}
		return c.Run()
	},
}
