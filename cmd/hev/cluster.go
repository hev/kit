package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/hev/kit/internal/cluster"
	"github.com/spf13/cobra"
)

const clusterHint = "Cluster is disabled; add [cluster] with bind and key_ref to ~/.hev/config.toml (or HEV_CONFIG)."

func newClusterCommand() *cobra.Command {
	root := &cobra.Command{Use: "cluster", Short: "Inspect encrypted hevd membership and usage facts"}
	for _, action := range []string{"ls", "status", "join", "leave"} {
		action := action
		use := action
		args := cobra.NoArgs
		if action == "join" {
			use += " ADDR"
			args = cobra.ExactArgs(1)
		}
		c := &cobra.Command{Use: use, Args: args, RunE: func(cmd *cobra.Command, args []string) error {
			cfg, e := cluster.LoadConfig("")
			if e != nil {
				return e
			}
			if cfg == nil {
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				fmt.Fprintln(cmd.OutOrStdout(), clusterHint)
				return errors.New("cluster disabled")
			}
			q := cluster.Request{Action: action}
			if len(args) > 0 {
				q.Address = args[0]
			}
			r, e := cluster.Query(cmd.Context(), q)
			if e != nil {
				return e
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if action == "ls" {
				if asJSON {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(r.Members)
				}
				return printClusterMembers(cmd.OutOrStdout(), r.Members)
			}
			if action == "status" {
				if asJSON {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(r.Status)
				}
				return printClusterStatus(cmd.OutOrStdout(), r.Status)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "cluster "+action+": ok")
			return nil
		}}
		if action == "ls" || action == "status" {
			c.Flags().Bool("json", false, "Print JSON")
		}
		root.AddCommand(c)
	}
	return root
}
func printClusterMembers(out io.Writer, members []cluster.Member) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATE\tADDRESS\tVERSION/CHANNEL\tOS/ARCH\tDISPATCH/CAP\tCHANGED")
	for _, m := range members {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s/%s\t%s/%s\t%t/%d\t%s ago\n", m.Name, m.State, m.Address, m.Version, m.Channel, m.OS, m.Arch, m.Dispatch, m.Cap, m.Since)
	}
	return w.Flush()
}
func printClusterStatus(out io.Writer, s *cluster.Status) error {
	fmt.Fprintf(out, "Enabled: %t; running: %t; key loaded: %t\nBound: %s\nMembers: %v\nSeed state: %s; next retry: %s\nKey retry: %s\n", s.Enabled, s.Running, s.KeyLoaded, s.Bound, s.Counts, s.SeedState, s.SeedRetry, s.KeyRetry)
	if s.Error != "" {
		fmt.Fprintln(out, s.Error)
	}
	rolling, _ := json.Marshal(s.Rolling)
	fmt.Fprintf(out, "Rolling hour/day per node: %s\nCounters: %s\n", rolling, s.Counters)
	return nil
}
func init() { rootCmd.AddCommand(newClusterCommand()) }
