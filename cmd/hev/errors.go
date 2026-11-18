package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/hev/kit/internal/exitcode"
	"github.com/spf13/cobra"
)

// configureErrors makes the root the one place that prints an error: the
// commands stay quiet, flag and argument mistakes become usage errors (exit
// 2), and an unknown command gets a "Did you mean" suggestion.
func configureErrors(root *cobra.Command) {
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return exitcode.UsageErr(err)
	})
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c.SuggestionsMinimumDistance <= 0 {
			c.SuggestionsMinimumDistance = 2
		}
		if c.Args != nil {
			validate := c.Args
			c.Args = func(c *cobra.Command, args []string) error {
				err := validate(c, args)
				if err == nil {
					return nil
				}
				if c.HasAvailableSubCommands() && len(args) > 0 {
					if s := c.SuggestionsFor(args[0]); len(s) > 0 {
						err = exitcode.WithHint(err, fmt.Sprintf("Did you mean %q?", s[0]))
					}
				}
				return exitcode.UsageErr(err)
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

// reportError prints err for the command that failed and returns the exit
// status. stdout never carries error text.
func reportError(stderr io.Writer, cmd *cobra.Command, err error) int {
	var exit *commandExit
	if errors.As(err, &exit) {
		return exit.code // the command already said what went wrong
	}
	if msg := licenseRequiredMessage(err); msg != "" {
		fmt.Fprintln(stderr, msg)
		return exitcode.Error
	}
	fmt.Fprintln(stderr, strings.TrimRight(err.Error(), "\n"))
	if hint := exitcode.Hint(err); hint != "" {
		fmt.Fprintln(stderr, hint)
	}
	code := exitcode.Of(err)
	if code == exitcode.Usage && cmd != nil {
		fmt.Fprintf(stderr, "Run '%s --help' for usage.\n", cmd.CommandPath())
	}
	return code
}
