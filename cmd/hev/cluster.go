package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/hostlabel"
	"github.com/spf13/cobra"
)

const clusterHint = "Cluster is disabled; add [cluster] with bind and key_ref to ~/.hev/config.toml (or HEV_CONFIG)."

func newClusterCommand() *cobra.Command {
	root := newNodesCommand()
	root.Use = "cluster"
	root.Short = "Manage cluster membership and SSH dispatch access"
	root.Hidden = false

	for _, c := range root.Commands() {
		switch c.Name() {
		case "join":
			joinSSH := c.RunE
			c.Use += " | join ADDR:PORT"
			c.Short = "Join with an SSH invitation or an encrypted cluster address"
			c.RunE = func(cmd *cobra.Command, args []string) error {
				// Invite codes are base64url and cannot contain a colon.
				if len(args) == 1 && strings.Contains(args[0], ":") {
					return runClusterCommand(cmd, "join", args[0])
				}
				return joinSSH(cmd, args)
			}
		case "ls":
			listSSH := c.RunE
			var sshOnly bool
			c.Flags().BoolVar(&sshOnly, "ssh", false, "List SSH dispatch access even when encrypted membership is configured")
			c.Short = "List cluster members or SSH dispatch access"
			c.RunE = func(cmd *cobra.Command, args []string) error {
				if sshOnly {
					return listSSH(cmd, args)
				}
				cfg, err := cluster.LoadConfig("")
				if err != nil {
					return err
				}
				if cfg == nil {
					return listSSH(cmd, args)
				}
				return queryCluster(cmd, "ls", "")
			}
		}
	}

	root.AddCommand(
		&cobra.Command{Use: "status", Short: "Show encrypted hevd membership and usage status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return runClusterCommand(cmd, "status", "")
		}},
		&cobra.Command{Use: "leave", Short: "Leave encrypted hevd membership", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return runClusterCommand(cmd, "leave", "")
		}},
	)
	return root
}

func runClusterCommand(cmd *cobra.Command, action, address string) error {
	cfg, err := cluster.LoadConfig("")
	if err != nil {
		return err
	}
	if cfg == nil {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		fmt.Fprintln(cmd.OutOrStdout(), clusterHint)
		return errors.New("cluster disabled")
	}
	return queryCluster(cmd, action, address)
}

func queryCluster(cmd *cobra.Command, action, address string) error {
	r, err := cluster.Query(cmd.Context(), cluster.Request{Action: action, Address: address})
	if err != nil {
		return err
	}
	if r.MembershipChanged {
		event := "cluster-join"
		if action == "leave" {
			event = "cluster-remove"
		}
		name := r.MemberName
		if name == "" {
			name = hostlabel.Self(os.Hostname)
		}
		clusterNodeEvent(cmd, event, name, "node")
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	switch action {
	case "ls":
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(r.Members)
		}
		return printClusterMembers(cmd.OutOrStdout(), r.Members)
	case "status":
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(r.Status)
		}
		return printClusterStatus(cmd.OutOrStdout(), r.Status)
	default:
		fmt.Fprintln(cmd.OutOrStdout(), "cluster "+action+": ok")
		return nil
	}
}
func printClusterMembers(out io.Writer, members []cluster.Member) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATE\tADDRESS\tVERSION/CHANNEL\tOS/ARCH\tDISPATCH/CAP\tACTIVITY\tCHANGED")
	for _, m := range members {
		name := m.Name
		if m.Self {
			name += "*"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s/%s\t%s/%s\t%t/%d\t%s\t%s ago\n", name, m.State, m.Address, m.Version, m.Channel, m.OS, m.Arch, m.Dispatch, m.Cap, m.Activity.Short(), m.Since)
	}
	return w.Flush()
}
func printClusterStatus(out io.Writer, s *cluster.Status) error {
	if s.Self != "" {
		fmt.Fprintf(out, "Self: %s\n", s.Self)
	}
	fmt.Fprintf(out, "Enabled: %t; running: %t; key loaded: %t\nBound: %s\nMembers: %v\nSeed state: %s; next retry: %s\nKey retry: %s\n", s.Enabled, s.Running, s.KeyLoaded, s.Bound, s.Counts, s.SeedState, s.SeedRetry, s.KeyRetry)
	if s.Error != "" {
		fmt.Fprintln(out, s.Error)
	}
	rolling, _ := json.Marshal(s.Rolling)
	fmt.Fprintf(out, "Rolling hour/day per node: %s\nCounters: %s\n", rolling, s.Counters)
	return nil
}
func init() { rootCmd.AddCommand(newClusterCommand()) }
