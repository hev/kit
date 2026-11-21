package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/subscriptions"
	"github.com/spf13/cobra"
)

func init() {
	dispatch.SubscriptionsHandler = subscriptionsRPC
}

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
		return env, fmt.Errorf("--plans: invalid subscriptions.toml")
	case !errors.Is(err, os.ErrNotExist):
		// A broken optional file must not hide plan status or print secret paths.
		fmt.Fprintln(os.Stderr, "Warning: subscriptions.toml could not be parsed")
	}
	return env, nil
}

// subscriptionsDispatchRPC runs one dispatch RPC for --host / --all; tests can
// replace it to exercise multi-host fan-out without SSH.
var subscriptionsDispatchRPC = dispatch.RPC

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
	var asJSON, all bool
	var host, configPath, plansPath string
	callHost := func(parent context.Context, h dispatch.Host, name string, out any) error {
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		r, err := subscriptionsDispatchRPC(ctx, h, dispatch.Request{Action: "subscriptions", Name: name, Meta: dispatch.Meta{Sender: dispatch.LocalSender()}})
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
	loadHosts := func() (dispatch.Config, []string, error) {
		c, err := dispatch.Load(dispatch.ConfigPath(configPath))
		if err != nil {
			return c, nil, err
		}
		if len(c.Hosts) == 0 {
			return c, nil, fmt.Errorf("no hosts configured in dispatch configuration")
		}
		names := make([]string, 0, len(c.Hosts))
		for n := range c.Hosts {
			names = append(names, n)
		}
		sort.Strings(names)
		return c, names, nil
	}
	remote := func(cmd *cobra.Command, name string, out any) error {
		c, err := dispatch.Load(dispatch.ConfigPath(configPath))
		if err != nil {
			return err
		}
		h, ok := c.Hosts[host]
		if !ok {
			return fmt.Errorf("unknown host %s", host)
		}
		return callHost(cmd.Context(), h, name, out)
	}
	hostUnknownStatus := func(hostName string, p subscriptions.Provider, err error) subscriptions.Status {
		reason := safeHostError(err)
		return subscriptions.Status{
			Host:            hostName,
			Name:            p.Name,
			Harness:         p.Harness,
			State:           subscriptions.Unknown,
			StateReason:     reason,
			PlanReason:      reason,
			WeeklyReason:    reason,
			ObservedReason:  reason,
			Source:          "dispatch rpc",
			Meters:          []subscriptions.Meter{},
			ConfiguredPlans: []string{},
		}
	}
	list := func(cmd *cobra.Command) error {
		if host != "" && all {
			return subscriptionsUsageError(cmd, fmt.Errorf("cannot combine --host and --all"))
		}
		var l subscriptions.List
		switch {
		case all:
			c, hostNames, err := loadHosts()
			if err != nil {
				return err
			}
			perHost := make([][]subscriptions.Status, len(hostNames))
			var wg sync.WaitGroup
			for i, hName := range hostNames {
				wg.Add(1)
				go func(idx int, hn string, h dispatch.Host) {
					defer wg.Done()
					var hl subscriptions.List
					if err := callHost(cmd.Context(), h, "", &hl); err != nil {
						rows := make([]subscriptions.Status, 0, len(subscriptions.Providers()))
						for _, p := range subscriptions.Providers() {
							rows = append(rows, hostUnknownStatus(hn, p, err))
						}
						perHost[idx] = rows
						return
					}
					for j := range hl.Subscriptions {
						hl.Subscriptions[j].Host = hn
					}
					perHost[idx] = hl.Subscriptions
				}(i, hName, c.Hosts[hName])
			}
			wg.Wait()
			l = subscriptions.List{
				Schema:        subscriptions.Schema,
				GeneratedAt:   time.Now().UTC(),
				Subscriptions: []subscriptions.Status{},
			}
			for _, rows := range perHost {
				l.Subscriptions = append(l.Subscriptions, rows...)
			}
		case host != "":
			if err := remote(cmd, "", &l); err != nil {
				return err
			}
			for i := range l.Subscriptions {
				l.Subscriptions[i].Host = host
			}
		default:
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
		if host != "" && all {
			return subscriptionsUsageError(cmd, fmt.Errorf("cannot combine --host and --all"))
		}
		p, err := subscriptions.Lookup(name)
		if err != nil {
			return subscriptionsUsageError(cmd, err)
		}
		if all {
			c, hostNames, err := loadHosts()
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			perHost := make([]subscriptions.Status, len(hostNames))
			var wg sync.WaitGroup
			for i, hName := range hostNames {
				wg.Add(1)
				go func(idx int, hn string, h dispatch.Host) {
					defer wg.Done()
					var hd subscriptions.Detail
					if err := callHost(cmd.Context(), h, p.Name, &hd); err != nil {
						perHost[idx] = hostUnknownStatus(hn, p, err)
						return
					}
					hd.Subscription.Host = hn
					perHost[idx] = hd.Subscription
				}(i, hName, c.Hosts[hName])
			}
			wg.Wait()
			if asJSON {
				return writeSubscriptionsJSON(cmd, subscriptions.List{
					Schema:        subscriptions.Schema,
					GeneratedAt:   now,
					Subscriptions: perHost,
				})
			}
			for i, s := range perHost {
				if i > 0 {
					fmt.Fprintln(cmd.OutOrStdout())
				}
				if err := subscriptions.WriteDetail(cmd.OutOrStdout(), subscriptions.Detail{
					Schema:       subscriptions.Schema,
					GeneratedAt:  now,
					Subscription: s,
				}); err != nil {
					return err
				}
			}
			return nil
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
  hev subscriptions --all        List across every configured dispatch host
  hev subscriptions show NAME    Every meter for one subscription
  hev subscription NAME          Same as show NAME

Codex readings come from local rollouts and make no network call. Claude
readings make at most one request to the Claude Code usage endpoint and cache
the answer for 10 minutes in ~/.hev/subscriptions/claude.json. Antigravity
(agy) distinguishes Google AI OAuth (Ultra, Pro, Standard, Enterprise) from
Gemini API-key billing, caches model-tier checks for 10 minutes in
~/.hev/subscriptions/agy.json, and reports bounded local transcript activity
without fabricating quota percentages. Offline, the list shows the cached
reading or unknown.

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
	root.PersistentFlags().BoolVar(&all, "all", false, "Read on every configured dispatch host")
	root.PersistentFlags().StringVar(&configPath, "config", "", "Dispatch TOML configuration (selects --host and --all)")
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
meter (session window, weekly, model-scoped weekly, credits, extra usage or
local activity where the provider reports them) with percent used and reset
time, where the reading came from and how old it is. An unknown NAME exits
non-zero.`,
		Args:          subscriptionsArgs(cobra.ExactArgs(1)),
		SilenceUsage:  true,
		SilenceErrors: true,
		ValidArgs:     subscriptions.Names(),
		RunE:          func(cmd *cobra.Command, args []string) error { return show(cmd, args[0]) },
	})
	return root
}

func safeHostError(err error) string {
	if err == nil {
		return "host unreachable"
	}
	s := strings.SplitN(strings.TrimSpace(err.Error()), "\n", 2)[0]
	switch {
	case s == "unknown action", strings.Contains(s, "does not support hev subscriptions"), strings.Contains(s, "predate hev subscriptions"):
		return "host hev predates hev subscriptions; upgrade hev on the host"
	case strings.Contains(s, "agy") && strings.Contains(s, "unknown subscription"):
		return "host hev predates agy subscription tracking; upgrade hev on the host"
	default:
		return "host unreachable over dispatch rpc"
	}
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
