package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/update"
	"github.com/spf13/cobra"
)

// newHostsUpdateCommand is `hev dispatch hosts update`: the owner asks each
// configured host to update itself. The request carries a channel and a
// version at most; each host fetches and verifies the build itself.
func newHostsUpdateCommand(load func() (dispatch.Config, error), host *string, asJSON *bool) *cobra.Command {
	var all, check bool
	var to, channel string
	c := &cobra.Command{
		Use:   "update",
		Short: "Ask hosts to update themselves to the newest build on their channel, or a named one",
		Long: `Ask each selected host to run hev update on itself and report, per host, the
version before and after, or why not. Name one host with --host, or every
configured host with --all. --channel moves the hosts to that channel (and
records it on each); --to names a version; --check reports only.

Updating a host is an owner action: a host refuses the request from an
invited member. The request carries only a channel and a version; each host
fetches the build from the official repository and verifies it itself.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := load()
			if err != nil {
				return err
			}
			if (*host == "") == !all {
				return fmt.Errorf("name one host with --host NAME, or every host with --all")
			}
			if channel != "" && !update.ValidChannel(channel) {
				return fmt.Errorf("unknown channel %q (want release or edge)", channel)
			}
			if to != "" {
				v, err := update.NormalizeVersion(to)
				if err != nil {
					return err
				}
				to = v
			}
			hosts := []string{*host}
			if all {
				hosts = hosts[:0]
				for n := range cfg.Hosts {
					hosts = append(hosts, n)
				}
				sort.Strings(hosts)
			}
			res := dispatch.UpdateHosts(cmd.Context(), cfg, hosts, channel, to, check)
			if *asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "HOST\tBEFORE\tAFTER\tRESULT")
			failed := 0
			for _, n := range hosts {
				r := res[n]
				result := ""
				switch {
				case r.Error != "":
					failed++
					result = r.Error
				case r.Report != nil:
					result = r.Report.Message
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", n, dash(r.Before), dash(r.After), strings.ReplaceAll(result, "\n", " "))
			}
			w.Flush()
			if failed > 0 {
				cmd.SilenceUsage = true
				return fmt.Errorf("%d of %d hosts did not update", failed, len(hosts))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&all, "all", false, "Every configured host")
	c.Flags().StringVar(&to, "to", "", "Version to move to (X.Y.Z or X.Y.Z-edge.<sha>)")
	c.Flags().StringVar(&channel, "channel", "", "release or edge; recorded on each host")
	c.Flags().BoolVar(&check, "check", false, "Report what would change; change nothing")
	return c
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// annotateChannels fills in, for each host that reports a channel, the
// newest build on it and whether the host is behind. One lookup per channel;
// an unreachable source leaves "?".
func annotateChannels(ctx context.Context, inv map[string]dispatch.Response) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	latest := map[string]*update.Build{}
	found := map[string]*update.Build{}
	for n, r := range inv {
		if r.Error != "" || r.Channel == "" {
			continue
		}
		b, seen := latest[r.Channel]
		if !seen {
			if got, err := updateSource.Latest(ctx, r.Channel); err == nil {
				b = &got
			}
			latest[r.Channel] = b
		}
		if b == nil {
			r.Behind = "?"
			inv[n] = r
			continue
		}
		r.ChannelLatest = b.Version
		r.Behind = behind(ctx, r.Version, *b, found)
		inv[n] = r
	}
}

func behind(ctx context.Context, have string, latest update.Build, found map[string]*update.Build) string {
	if have == latest.Version {
		return "no"
	}
	cmp, ok := update.Compare(have, latest.Version)
	if !ok && update.VersionChannel(latest.Version) == update.ChannelEdge {
		mine, seen := found[have]
		if !seen {
			if b, err := updateSource.Find(ctx, have); err == nil {
				mine = &b
			}
			found[have] = mine
		}
		if mine != nil && mine.Sequence > 0 && latest.Sequence > 0 {
			if n := latest.Sequence - mine.Sequence; n > 0 {
				return fmt.Sprintf("yes, %d commits (%s)", n, latest.Version)
			} else if n < 0 {
				return "no (ahead)"
			}
		}
		if _, err := update.NormalizeVersion(have); err == nil {
			return "yes (" + latest.Version + ")"
		}
	}
	switch {
	case !ok:
		return "?"
	case cmp < 0:
		return "yes (" + latest.Version + ")"
	}
	return "no (ahead)"
}
