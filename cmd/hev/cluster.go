package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/spf13/cobra"
)

const clusterHint = "Cluster is disabled; add [cluster] with bind and key_ref to ~/.hev/config.toml (or HEV_CONFIG)."

func newClusterCommand() *cobra.Command {
	root := &cobra.Command{Use: "cluster", Short: "Inspect encrypted hevd membership and usage facts"}
	for _, action := range []string{"ls", "facts", "status", "join", "leave"} {
		action := action
		use := action
		args := cobra.NoArgs
		if action == "facts" {
			use += " [NODE]"
			args = cobra.MaximumNArgs(1)
		}
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
			if action == "facts" {
				q.Action = "ls"
			}
			if len(args) > 0 {
				q.Address = args[0]
			}
			r, e := cluster.Query(cmd.Context(), q)
			if e != nil {
				return e
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if action == "facts" {
				members := r.Members
				if len(args) > 0 {
					members = nil
					for _, m := range r.Members {
						if m.Name == args[0] {
							members = append(members, m)
						}
					}
					if len(members) == 0 {
						return errors.New("cluster node not found")
					}
				}
				if asJSON {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(members)
				}
				for _, m := range members {
					fmt.Fprintf(cmd.OutOrStdout(), "%s ages(seconds): %v; stale: %v\n", m.Name, m.Ages, m.Stale)
					raw, _ := json.MarshalIndent(m.NodeFacts, "", "  ")
					fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				}
				return nil
			}
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
		if action == "ls" || action == "facts" || action == "status" {
			c.Flags().Bool("json", false, "Print JSON")
		}
		root.AddCommand(c)
	}
	return root
}
func printClusterMembers(out io.Writer, members []cluster.Member) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATE\tADDRESS\tVERSION/CHANNEL\tOS/ARCH\tDISPATCH/CAP\tCHANGED\tSESSIONS\tSLOTS (QUEUE)\tBUILD\tHEALTH")
	var alive, totalSessions, totalFree uint64
	for _, m := range members {
		sessions, slots, build, health, ns, nf := clusterColumns(m, time.Now())
		if m.State == "alive" {
			alive++
			totalSessions += ns
			totalFree += nf
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s/%s\t%s/%s\t%t/%d\t%s ago\t%s\t%s\t%s\t%s\n", m.Name, m.State, m.Address, m.Version, m.Channel, m.OS, m.Arch, m.Dispatch, m.Cap, m.Since, sessions, slots, build, health)
	}
	fmt.Fprintf(w, "Total: %d alive; %d sessions; %d free slots\n", alive, totalSessions, totalFree)
	return w.Flush()
}
func printClusterStatus(out io.Writer, s *cluster.Status) error {
	fmt.Fprintf(out, "Enabled: %t; running: %t; key loaded: %t\nBound: %s\nMembers: %v\nSeed state: %s; next retry: %s\nKey retry: %s\n", s.Enabled, s.Running, s.KeyLoaded, s.Bound, s.Counts, s.SeedState, s.SeedRetry, s.KeyRetry)
	if s.Error != "" {
		fmt.Fprintln(out, s.Error)
	}
	rolling, _ := json.Marshal(s.Rolling)
	fmt.Fprintf(out, "Rolling hour/day per node: %s\nCounters: %s\n", rolling, s.Counters)
	facts, _ := json.Marshal(s.Facts)
	ages, _ := json.Marshal(s.Ages)
	stale, _ := json.Marshal(s.Stale)
	fmt.Fprintf(out, "Node facts: %s\nAges(seconds): %s\nStale sections: %s\n", facts, ages, stale)
	return nil
}
func init() { rootCmd.AddCommand(newClusterCommand()) }

func clusterColumns(m cluster.Member, now time.Time) (sessions, slots, build, health string, total, free uint64) {
	sessions, slots, build, health = "-", "-", "-", "unknown"
	f := m.NodeFacts
	if f == nil {
		return
	}
	ages := f.Ages(now)
	var failures []string
	stale := func(k string) bool {
		if ages[k] > int64(cluster.FactsStale.Seconds()) {
			failures = append(failures, k+":stale")
			return true
		}
		return false
	}
	if s := f.Sessions; s != nil {
		if stale("sessions") {
			sessions = "stale"
		} else {
			var working uint64
			for _, v := range s.Harness {
				working += uint64(v.Working)
				total += uint64(v.Working) + uint64(v.Idle)
			}
			sessions = fmt.Sprintf("%d/%d", working, total)
		}
	}
	if d := f.Dispatch; d != nil {
		if stale("dispatch") {
			slots = "stale"
		} else {
			free = uint64(d.Free)
			slots = fmt.Sprintf("%d/%d (%d)", d.Free, d.Cap, d.Queue)
		}
	}
	if b := f.Build; b != nil {
		if stale("build") {
			build = "stale"
		} else {
			build = b.Commit
			if build == "" {
				build = "unknown"
			}
			if b.Outcome == "failing" {
				failures = append(failures, "build")
			}
			if b.Staged {
				build += "+staged"
			}
		}
	}
	if c := f.Credentials; c != nil && !stale("credentials") {
		for _, v := range c.Items {
			if !v.At.IsZero() && now.Sub(v.At) > cluster.FactsStale {
				failures = append(failures, "credentials:stale")
				break
			}
			if v.State == "failing" {
				failures = append(failures, "credentials")
				break
			}
		}
	}
	if c := f.Capture; c != nil && !stale("capture") {
		if !c.Running || c.Error != "" {
			failures = append(failures, "capture")
		}
	}
	health = "ok"
	if len(failures) > 0 {
		health = strings.Join(failures, ",")
	}
	return
}
