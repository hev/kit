package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/subscriptions"
	"github.com/spf13/cobra"
)

// subscriptionsEnv builds the read environment; tests replace it.
var subscriptionsEnv = func(plansPath string) (subscriptions.Env, error) {
	env, err := subscriptions.DefaultEnv()
	if err != nil {
		return env, err
	}
	explicit := plansPath != ""
	if !explicit {
		plansPath = filepath.Join(env.StateDir, "subscriptions.toml")
	}
	plans, err := subscriptions.LoadPlans(plansPath)
	switch {
	case err == nil:
		env.Plans = plans
	case explicit:
		return env, fmt.Errorf("--plans: %w", err)
	case !errors.Is(err, os.ErrNotExist):
		// A broken optional file must not hide plan status.
		fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", plansPath, err)
	}
	return env, nil
}

// subscriptionsRPC answers `hev subscriptions --host` on the host.
func subscriptionsRPC(ctx context.Context, q dispatch.Request) (json.RawMessage, error) {
	env, err := subscriptionsEnv("")
	if err != nil {
		return nil, err
	}
	if q.Name == "" {
		return json.Marshal(subscriptions.ReadAll(ctx, env))
	}
	d, err := subscriptions.ReadOne(ctx, env, q.Name)
	if err != nil {
		return nil, err
	}
	return json.Marshal(d)
}

func newSubscriptionsCommand() *cobra.Command {
	var asJSON bool
	var host, configPath, plansPath string
	remote := func(cmd *cobra.Command, name string, out any) error {
		c, err := dispatch.Load(dispatch.ConfigPath(configPath))
		if err != nil {
			return err
		}
		h, ok := c.Hosts[host]
		if !ok {
			return fmt.Errorf("unknown host %s", host)
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()
		r, err := dispatch.RPC(ctx, h, dispatch.Request{Action: "subscriptions", Name: name, Meta: dispatch.Meta{Sender: dispatch.LocalSender()}})
		if err != nil {
			if err.Error() == "unknown action" || strings.Contains(err.Error(), "action subscriptions is unavailable") {
				err = fmt.Errorf("%w (the host's hev may predate hev subscriptions; ask the owner to upgrade)", err)
			}
			return err
		}
		if len(r.Subscriptions) == 0 {
			return fmt.Errorf("host does not support hev subscriptions; upgrade hev on the host")
		}
		return json.Unmarshal(r.Subscriptions, out)
	}
	list := func(cmd *cobra.Command) error {
		var l subscriptions.List
		if host != "" {
			if err := remote(cmd, "", &l); err != nil {
				return err
			}
			for i := range l.Subscriptions {
				l.Subscriptions[i].Host = host
			}
		} else {
			env, err := subscriptionsEnv(plansPath)
			if err != nil {
				return err
			}
			l = subscriptions.ReadAll(cmd.Context(), env)
		}
		if asJSON {
			return writeSubscriptionsJSON(cmd, l)
		}
		return subscriptions.WriteList(cmd.OutOrStdout(), l)
	}
	show := func(cmd *cobra.Command, name string) error {
		if _, err := subscriptions.Lookup(name); err != nil {
			return subscriptionsUsageError(cmd, err)
		}
		var d subscriptions.Detail
		if host != "" {
			if err := remote(cmd, name, &d); err != nil {
				return err
			}
			d.Subscription.Host = host
		} else {
			env, err := subscriptionsEnv(plansPath)
			if err != nil {
				return err
			}
			if d, err = subscriptions.ReadOne(cmd.Context(), env, name); err != nil {
				return err
			}
		}
		if asJSON {
			return writeSubscriptionsJSON(cmd, d)
		}
		return subscriptions.WriteDetail(cmd.OutOrStdout(), d)
	}
	names := strings.Join(subscriptions.Names(), ", ")
	root := &cobra.Command{
		Use:     "subscriptions [NAME]",
		Aliases: []string{"subscription", "subs"},
		Short:   "Check which agent plans are signed in here and how much is used",
		Long: `Show the flat-rate agent plans (subscriptions) kit can check, whether this
machine is signed in to each, how much of each usage window is used and when it
resets. Subscriptions: ` + names + `.

  hev subscriptions              List every subscription (same as ls)
  hev subscriptions show NAME    Every meter for one subscription
  hev subscription NAME          Same as show NAME

Codex readings come from local rollouts and make no network call. Claude
readings make at most one request to the Claude Code usage endpoint and cache
the answer for 10 minutes in ~/.hev/subscriptions/claude.json.
Offline, the list shows the cached reading or unknown.

The spend side is separate: subscriptions.toml still prices plans for
hev cost-price and hev serve. When a configured plan covers the same harness,
show names it. See docs/subscriptions.md.`,
		Args: subscriptionsArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return show(cmd, args[0])
			}
			return list(cmd)
		},
	}
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "Print the versioned JSON shape ("+subscriptions.Schema+")")
	// Accept the original flag without bypassing the provider's request interval.
	root.PersistentFlags().Bool("refresh", false, "Read usage within the provider's request interval")
	_ = root.PersistentFlags().MarkHidden("refresh")
	root.SilenceErrors, root.SilenceUsage = true, true
	root.SetFlagErrorFunc(subscriptionsUsageError)
	root.PersistentFlags().StringVar(&host, "host", "", "Read on a configured dispatch host instead of this machine")
	root.PersistentFlags().StringVar(&configPath, "config", "", "Dispatch TOML configuration (selects --host)")
	root.PersistentFlags().StringVar(&plansPath, "plans", "", "subscriptions.toml naming spend plans (default ~/.hev/subscriptions.toml when present)")
	root.AddCommand(&cobra.Command{
		Use:           "ls",
		Aliases:       []string{"list"},
		Short:         "List every subscription kit can check: plan, state, weekly use and reset",
		Args:          subscriptionsArgs(cobra.NoArgs),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, _ []string) error { return list(cmd) },
	})
	root.AddCommand(&cobra.Command{
		Use:   "show NAME",
		Short: "Show every meter for one subscription (" + names + ")",
		Long: `Show everything known about one subscription: plan tier, login state, each
meter (session window, weekly, model-scoped weekly, credits or extra usage
where the provider reports them) with percent used and reset time, where the
reading came from and how old it is. An unknown NAME exits non-zero.`,
		Args:          subscriptionsArgs(cobra.ExactArgs(1)),
		SilenceUsage:  true,
		SilenceErrors: true,
		ValidArgs:     subscriptions.Names(),
		RunE:          func(cmd *cobra.Command, args []string) error { return show(cmd, args[0]) },
	})
	return root
}

func subscriptionsArgs(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validate(cmd, args); err != nil {
			return subscriptionsUsageError(cmd, err)
		}
		return nil
	}
}

// Match v0.7's usage-error convention while the shared CLI handler is pending.
func subscriptionsUsageError(cmd *cobra.Command, err error) error {
	fmt.Fprintln(cmd.ErrOrStderr(), err)
	fmt.Fprintf(cmd.ErrOrStderr(), "Run '%s --help' for usage.\n", cmd.CommandPath())
	return &commandExit{code: 2}
}

func writeSubscriptionsJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
