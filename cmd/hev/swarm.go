package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
)

func init() { rootCmd.AddCommand(newSwarmCommand()) }

func newSwarmCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "swarm",
		Short: "Put an orchestrator and its workers on a problem from one file",
		Long: `A swarm is one Markdown file that declares a problem, an orchestrator and
worker roles with concurrency limits (RFC 0013). up compiles it into an
orchestrator and ordinary dispatch runs on this host; a code-only reconcile
tick in the orchestrator's supervisor launches units, repairs failed ones in
place and starts integration when the pieces are done. A person merges.

Phase 1: every unit runs on this host. See docs/swarms.md.`,
	}
	var file string
	var dry bool
	up := &cobra.Command{Use: "up -f FILE", Short: "Validate, compile and apply a swarm file", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		b, e := os.ReadFile(file)
		if e != nil {
			return e
		}
		plan, e := dispatch.SwarmUp(string(b), dry)
		if e != nil {
			return e
		}
		dispatch.WriteSwarmPlan(cmd.OutOrStdout(), plan, dry)
		if !dry {
			fmt.Fprintf(cmd.OutOrStdout(), "applied o-%s; the reconcile tick launches units\n", plan.Spec.Name)
		}
		return nil
	}}
	up.Flags().StringVarP(&file, "file", "f", "", "Swarm file")
	_ = up.MarkFlagRequired("file")
	up.Flags().BoolVar(&dry, "dry-run", false, "Print the compiled orchestrator spec and orders; write no state")
	c.AddCommand(up)

	var asJSON bool
	ps := &cobra.Command{Use: "ps [NAME]", Short: "Show units by role: generation, state, head, restarts", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			views, e := dispatch.ListSwarms()
			if e != nil {
				return e
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(views)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			defer w.Flush()
			fmt.Fprintln(w, "NAME\tSTATE\tUNITS\tDONE\tROUNDS\tRESTARTS")
			for _, v := range views {
				done := 0
				for _, u := range v.Units {
					if u.State == "done" {
						done++
					}
				}
				state := v.Orchestrator
				if v.Down {
					state = "down"
				}
				fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\n", v.Name, state, len(v.Units), done, v.Rounds, v.Restarts)
			}
			return nil
		}
		v, e := dispatch.SwarmPS(args[0])
		if e != nil {
			return e
		}
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(v)
		}
		dispatch.WriteSwarmPS(cmd.OutOrStdout(), v)
		return nil
	}}
	ps.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	c.AddCommand(ps)

	var follow bool
	var round int
	logs := &cobra.Command{Use: "logs NAME [ROLE[/UNIT]]", Short: "Orchestrator and tick log, a round trace, or one unit's run log", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if !dispatch.IsSwarm(name) {
			return fmt.Errorf("no swarm named %s", name)
		}
		dir := dispatch.OrchestratorDir(name)
		if len(args) == 1 {
			files := []string{"supervisor.log", "swarm.log"}
			if round > 0 {
				files = []string{filepath.Join("rounds", fmt.Sprintf("%03d", round), "trace")}
			}
			for _, f := range files {
				b, e := os.ReadFile(filepath.Join(dir, f))
				if e != nil && !os.IsNotExist(e) {
					return e
				}
				if len(files) > 1 {
					fmt.Fprintf(cmd.OutOrStdout(), "--- %s ---\n", f)
				}
				fmt.Fprint(cmd.OutOrStdout(), string(b))
			}
			return nil
		}
		run, ok := dispatch.SwarmUnitRun(name, args[1])
		if !ok {
			return fmt.Errorf("swarm %s has no run for %s", name, args[1])
		}
		var offset int64
		for {
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
			r, e := dispatch.SwarmRunLogs(ctx, run, offset)
			cancel()
			if e != nil {
				return e
			}
			fmt.Fprint(cmd.OutOrStdout(), r.Log)
			offset = r.Offset
			if !follow || !r.Active {
				return nil
			}
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case <-time.After(time.Second):
			}
		}
	}}
	logs.Flags().BoolVarP(&follow, "follow", "f", false, "Follow the unit's run log")
	logs.Flags().IntVar(&round, "round", 0, "Print round N's trace")
	c.AddCommand(logs)

	c.AddCommand(&cobra.Command{Use: "say NAME [ROLE/UNIT] MSG...", Short: "Message the orchestrator or one unit's run; say NAME choose ROLE/vN records a person's race choice", Args: cobra.MinimumNArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		name, rest := args[0], args[1:]
		if !dispatch.IsSwarm(name) {
			return fmt.Errorf("no swarm named %s", name)
		}
		if rest[0] == "choose" {
			if len(rest) < 2 {
				return fmt.Errorf("choose needs ROLE/UNIT")
			}
			reason := strings.Join(rest[2:], " ")
			if reason == "" {
				reason = "chosen by a person"
			}
			if e := dispatch.SwarmChoose(name, rest[1], reason, "person"); e != nil {
				return e
			}
			fmt.Fprintf(cmd.OutOrStdout(), "chose %s\n", rest[1])
			return nil
		}
		if run, ok := dispatch.SwarmUnitRun(name, rest[0]); ok && len(rest) > 1 {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer cancel()
			r, e := dispatch.SwarmSayRun(ctx, run, strings.Join(rest[1:], " "))
			if e != nil {
				return e
			}
			fmt.Fprintln(cmd.OutOrStdout(), r.Message)
			return nil
		}
		if e := dispatch.SwarmSayOrchestrator(name, strings.Join(rest, " ")); e != nil {
			return e
		}
		fmt.Fprintf(cmd.OutOrStdout(), "sent o-%s\n", name)
		return nil
	}})
	for _, action := range []string{"pause", "resume"} {
		action := action
		short := "Stop launches, repairs and rounds; running workers continue"
		if action == "resume" {
			short = "Restart launches, repairs and rounds"
		}
		c.AddCommand(&cobra.Command{Use: action + " NAME", Short: short, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			e := dispatch.SwarmPause(args[0])
			if action == "resume" {
				e = dispatch.SwarmResume(args[0])
			}
			if e != nil {
				return e
			}
			fmt.Fprintln(cmd.OutOrStdout(), action, args[0])
			return nil
		}})
	}
	c.AddCommand(&cobra.Command{Use: "down NAME", Short: "Stop the orchestrator and every current run; keep branches, worktrees and records", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		r, e := dispatch.SwarmDown(cmd.Context(), args[0], nil)
		if e != nil {
			return e
		}
		dispatch.WriteSwarmDown(cmd.OutOrStdout(), r)
		return nil
	}})
	for _, sub := range c.Commands() {
		// Refusals name the problem; usage would bury it.
		sub.SilenceUsage = true
	}
	return c
}
