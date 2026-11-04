package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func init() { rootCmd.AddCommand(newOrchestrateCommand()) }
func newOrchestrateCommand() *cobra.Command {
	c := &cobra.Command{Use: "orchestrate", Short: "Manage opt-in orchestrators with durable checkpoints"}
	var file string
	var dry bool
	apply := &cobra.Command{Use: "apply -f FILE", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		b, e := os.ReadFile(file)
		if e != nil {
			return e
		}
		s, e := dispatch.ApplyOrchestrator(string(b), dry)
		if e != nil {
			return e
		}
		if dry {
			fmt.Fprintln(cmd.OutOrStdout(), "valid", s.Name)
			return nil
		}
		if s.AllowNested {
			fmt.Fprintln(cmd.ErrOrStderr(), "warning: nesting enabled; maximum depth is 2")
		}
		if e = dispatch.LaunchOrchestrator(s.Name); e != nil {
			return e
		}
		fmt.Fprintln(cmd.OutOrStdout(), "applied", s.Name)
		return nil
	}}
	apply.Flags().StringVarP(&file, "file", "f", "", "Markdown spec")
	_ = apply.MarkFlagRequired("file")
	apply.Flags().BoolVar(&dry, "dry-run", false, "Validate without writing state or launching a session")
	c.AddCommand(apply)
	var goal, runs, every, until, host, name, harness, model, reply, lifetime string
	start := &cobra.Command{Use: "start --goal TEXT --runs SELECTOR", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if name == "" {
			name = fmt.Sprintf("adhoc-%d", time.Now().UnixNano())
		}
		s := dispatch.OrchestratorSpec{Kind: "orchestrator", Name: name, Goal: goal, Host: host, Mode: "once", Harness: harness, Model: model, Reply: reply, Idle: "skip", Source: "none", Authority: []string{"say", "notify"}}
		s.Manages.Runs.Selector = runs
		if runs != "mine" && !strings.HasPrefix(runs, "issue=") {
			s.Manages.Runs.Selector = ""
			s.Manages.Runs.Names = strings.Split(runs, ",")
		}
		if every != "" {
			s.Mode = "loop"
			s.Every = every
			s.Until = until
			s.MaxLifetime = lifetime
		}
		if s.Reply == "" {
			s.Reply = os.Getenv("HEV_SESSION")
		}
		b, e := yaml.Marshal(s)
		if e != nil {
			return e
		}
		if _, e = dispatch.ApplyOrchestrator("---\n"+string(b)+"---\n"+goal+"\n", false); e != nil {
			return e
		}
		if e = dispatch.LaunchOrchestrator(name); e != nil {
			return e
		}
		fmt.Fprintln(cmd.OutOrStdout(), "started", name)
		return nil
	}}
	start.Flags().StringVar(&goal, "goal", "", "Standing goal")
	_ = start.MarkFlagRequired("goal")
	start.Flags().StringVar(&runs, "runs", "", "mine, issue=GLOB or comma-separated names")
	_ = start.MarkFlagRequired("runs")
	start.Flags().StringVar(&name, "name", "", "Orchestrator name")
	start.Flags().StringVar(&every, "every", "", "Round interval")
	start.Flags().StringVar(&until, "until", "goal", "goal, date, or never")
	start.Flags().StringVar(&lifetime, "max-lifetime", "24h", "Maximum loop lifetime")
	start.Flags().StringVar(&host, "host", "", "Host label for this local supervisor")
	start.Flags().StringVar(&harness, "harness", "codex", "Fresh-session harness")
	start.Flags().StringVar(&model, "model", "", "Model")
	start.Flags().StringVar(&reply, "reply", "", "Owner session (required for mine)")
	c.AddCommand(start)
	var asJSON bool
	ls := &cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		rows, e := dispatch.ListOrchestrators()
		if e != nil {
			return e
		}
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(rows)
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
		defer w.Flush()
		fmt.Fprintln(w, "NAME\tHOST\tMODE\tSTATE\tROUNDS\tNEXT\tRUNS\tTODAY\tSPEND\tGOAL")
		for _, r := range rows {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%d\t%d\tunknown\t%s\n", r.Name, r.Host, r.Mode, r.State, r.Rounds, roundTime(r.Next), r.ManagedRuns, r.RunsToday, r.Goal)
		}
		return nil
	}}
	ls.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	c.AddCommand(ls)
	var round int
	for _, action := range []string{"show", "logs", "wake", "pause", "resume", "stop", "handoff"} {
		action := action
		var to string
		sub := &cobra.Command{Use: action + " NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if _, e := dispatch.LoadOrchestrator(name); e != nil {
				return e
			}
			dir := dispatch.OrchestratorDir(name)
			switch action {
			case "show":
				for _, f := range []string{"spec.md", "checkpoint.md", "state.json", "budget.json", "budget-owner.json"} {
					b, e := os.ReadFile(filepath.Join(dir, f))
					if e != nil && !os.IsNotExist(e) {
						return e
					}
					fmt.Fprintf(cmd.OutOrStdout(), "--- %s ---\n%s\n", f, b)
				}
				entries, e := os.ReadDir(filepath.Join(dir, "rounds"))
				if e != nil && !os.IsNotExist(e) {
					return e
				}
				fmt.Fprint(cmd.OutOrStdout(), "Rounds:")
				for _, f := range entries {
					fmt.Fprint(cmd.OutOrStdout(), " "+f.Name())
				}
				fmt.Fprintln(cmd.OutOrStdout())
				return nil
			case "logs":
				p := filepath.Join(dir, "supervisor.log")
				if round > 0 {
					p = filepath.Join(dir, "rounds", fmt.Sprintf("%03d", round), "trace")
				}
				b, e := os.ReadFile(p)
				if e != nil {
					return e
				}
				_, e = cmd.OutOrStdout().Write(b)
				return e
			default:
				if e := dispatch.OrchestratorControl(name, action, to); e != nil {
					return e
				}
				if action == "wake" || action == "handoff" {
					if e := dispatch.LaunchOrchestrator(name); e != nil {
						return e
					}
				}
				fmt.Fprintln(cmd.OutOrStdout(), action, name)
				return nil
			}
		}}
		if action == "logs" {
			sub.Flags().IntVar(&round, "round", 0, "Round trace number")
		}
		if action == "handoff" {
			sub.Flags().StringVar(&to, "to", "", "New owner session")
			_ = sub.MarkFlagRequired("to")
		}
		c.AddCommand(sub)
	}
	c.AddCommand(&cobra.Command{Use: "supervise NAME", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return dispatch.SuperviseOrchestrator(cmd.Context(), args[0], dispatch.DefaultOrchestratorDriver())
	}})
	return c
}
