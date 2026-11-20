package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/hev/kit/internal/notify"
	"github.com/spf13/cobra"
)

// Seams for tests; production resolves through hev secrets and calls Slack.
var (
	notifyCheckResolver notify.Resolver = notify.ResolveSecret
	notifyCheckBase                     = notify.SlackAPI
)

func init() { rootCmd.AddCommand(newNotifyCommand()) }

func newNotifyCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "notify",
		Short: "Check this node's chat notifiers (notify.toml)",
		Long: `Named notifiers let orchestrators on this node post code-composed events to
a chat app. They are defined in notify.toml beside this node's dispatch.toml
and referenced from an orchestrator spec's notify: mapping.

https://hevkit.com/docs/notify`,
	}
	var asJSON bool
	check := &cobra.Command{
		Use:   "check [NOTIFIER]",
		Short: "Resolve each credential and probe auth and channel membership without posting",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			only := ""
			if len(args) == 1 {
				only = args[0]
			}
			ok, err := runNotifyCheck(cmd.Context(), cmd.OutOrStdout(), only, asJSON)
			if err != nil {
				return err
			}
			if !ok {
				return &commandExit{code: 1}
			}
			return nil
		},
		SilenceUsage: true,
	}
	check.Flags().BoolVar(&asJSON, "json", false, "Print results as JSON")
	c.AddCommand(check)
	return c
}

// runNotifyCheck prints labels, workspace names and classified errors only:
// never a token, reference, webhook URL or channel ID.
func runNotifyCheck(ctx context.Context, out io.Writer, only string, asJSON bool) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, err := notify.Load()
	if err != nil {
		return false, fmt.Errorf("%v (%s)", err, notify.Path())
	}
	results, err := notify.Check(ctx, cfg, only, notifyCheckResolver, func(token string) notify.Slack {
		return notify.Slack{BaseURL: notifyCheckBase, Token: token}
	})
	if err != nil {
		return false, err
	}
	ok := true
	for _, r := range results {
		ok = ok && r.OK()
	}
	if asJSON {
		return ok, json.NewEncoder(out).Encode(results)
	}
	for _, r := range results {
		fmt.Fprintln(out, r.String())
	}
	return ok, nil
}
