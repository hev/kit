package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	var scopeOwner, seed string
	apply := &cobra.Command{Use: "apply -f FILE", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		b, e := os.ReadFile(file)
		if e != nil {
			return e
		}
		if b, e = resolveSpecPaths(file, b); e != nil {
			return e
		}
		if seed != "" {
			if scopeOwner != "" {
				return fmt.Errorf("--seed-checkpoint needs an owner apply")
			}
			raw, err := os.ReadFile(seed)
			if err != nil {
				return err
			}
			if err = dispatch.ValidateSeedCheckpoint(raw); err != nil {
				return err
			}
		}
		var s dispatch.OrchestratorSpec
		if scopeOwner != "" {
			cfg, err := dispatch.Load(dispatch.ConfigPath(""))
			if err != nil {
				return err
			}
			owner, ok := cfg.Hosts[scopeOwner]
			if !ok || owner.Local {
				return fmt.Errorf("scope owner must name a configured remote host")
			}
			s, e = dispatch.RegisterOrchestratorScope(string(b), owner, dry)
		} else {
			s, e = dispatch.ApplyOrchestrator(string(b), dry)
		}
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
		if seed != "" {
			if e = dispatch.SeedOrchestratorCheckpoint(s.Name, seed); e != nil {
				return e
			}
		}
		if scopeOwner == "" {
			if e = dispatch.LaunchOrchestrator(s.Name); e != nil {
				return e
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), "applied", s.Name)
		return nil
	}}
	apply.Flags().StringVarP(&file, "file", "f", "", "Markdown spec")
	_ = apply.MarkFlagRequired("file")
	apply.Flags().StringVar(&scopeOwner, "scope-only-owner", "", "Register admission scope only; configured owner host controls lifecycle")
	apply.Flags().BoolVar(&dry, "dry-run", false, "Validate without writing state or launching a session")
	apply.Flags().StringVar(&seed, "seed-checkpoint", "", "Record FILE as round 000, the checkpoint round 001 reads")
	c.AddCommand(apply)
	c.AddCommand(&cobra.Command{Use: "say NAME MSG", Args: cobra.MinimumNArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if e := dispatch.SayOrchestrator(args[0], strings.Join(args[1:], " ")); e != nil {
			return e
		}
		return dispatch.LaunchOrchestrator(args[0])
	}})
	var goal, runs, every, until, host, name, harness, model, reply, lifetime string
	start := &cobra.Command{Use: "start --goal TEXT --runs SELECTOR", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if name == "" {
			name = fmt.Sprintf("adhoc-%d", time.Now().UnixNano())
		}
		s := dispatch.OrchestratorSpec{Kind: "orchestrator", Name: name, Goal: goal, Host: host, Mode: "once", Harness: harness, Model: model, Reply: reply, Idle: "skip", Agent: "on", Authority: []string{"say", "notify"}}
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
		fmt.Fprintln(w, "NAME\tHOST\tMODE\tSTATE\tROUNDS\tNEXT\tRUNS\tTODAY\tSPEND\tNOTICES\tGOAL")
		for _, r := range rows {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%d\t%d\tunknown\t%s\t%s\n", r.Name, r.Host, r.Mode, r.State, r.Rounds, roundTime(r.Next), r.ManagedRuns, r.RunsToday, backlogText(r.Backlog), r.Goal)
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
				for _, f := range []string{"spec.md", "checkpoint.md", "state.json", "budget.json", "budget-owner.json", "last-notice.json"} {
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
				spec, _ := dispatch.LoadOrchestrator(name)
				backlog, e := dispatch.OrchestratorBacklog(name)
				if e != nil {
					return e
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Owner notices: route %s; undelivered %s\n", dispatch.OrchestratorNoticeRoute(spec), backlogText(&backlog))
				if rows, e := dispatch.ListOrchestrators(); e == nil {
					for _, r := range rows {
						if r.Name == name && strings.HasPrefix(r.State, "blocked:") {
							fmt.Fprintln(cmd.OutOrStdout(), "State: "+r.State)
						}
					}
				}
				var st dispatch.OrchestratorState
				if b, e := os.ReadFile(filepath.Join(dir, "state.json")); e == nil && json.Unmarshal(b, &st) == nil {
					if st.Deferred != "" {
						fmt.Fprintln(cmd.OutOrStdout(), "Deferred: "+st.Deferred)
					}
					if sigs, e := dispatch.OrchestratorSignals(name); e == nil {
						st.Signals = sigs
					}
					if len(st.Signals) > 0 {
						fmt.Fprintln(cmd.OutOrStdout(), "Intake signals:")
						for _, sig := range st.Signals {
							fmt.Fprintf(cmd.OutOrStdout(), "  %s %s event=%s run=%s %s\n", sig.At.Format(time.RFC3339), sig.Kind, sig.Event, sig.Run, sig.Error)
						}
					}
				}
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
	var event string
	var admitted, notAdmitted bool
	reconcile := &cobra.Command{Use: "reconcile NAME --event ID --admitted|--not-admitted", Short: "Resolve an uncertain source receipt after inspecting the target", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if admitted == notAdmitted {
			return fmt.Errorf("choose exactly one of --admitted or --not-admitted")
		}
		n, e := dispatch.ReconcileSourceReceipt(args[0], event, admitted)
		if e != nil {
			return e
		}
		outcome := "live"
		if notAdmitted {
			outcome = "rejected (retry on next round)"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "reconciled %d receipt(s) for %s: %s\n", n, event, outcome)
		return nil
	}}
	reconcile.Flags().StringVar(&event, "event", "", "Source event ID")
	_ = reconcile.MarkFlagRequired("event")
	reconcile.Flags().BoolVar(&admitted, "admitted", false, "The run was admitted on the target")
	reconcile.Flags().BoolVar(&notAdmitted, "not-admitted", false, "The run was not admitted; retry the event")
	c.AddCommand(reconcile)
	var eventFile string
	render := &cobra.Command{Use: "render NAME --event FILE|-", Short: "Preview source rule orders without writing receipts", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var b []byte
		var e error
		if eventFile == "-" {
			b, e = io.ReadAll(cmd.InOrStdin())
		} else {
			b, e = os.ReadFile(eventFile)
		}
		if e != nil {
			return e
		}
		events, e := parseSourceEvents(b)
		if e != nil {
			return e
		}
		previews, e := dispatch.RenderSourcePreview(args[0], events)
		if e != nil {
			return e
		}
		if len(previews) == 0 {
			return fmt.Errorf("no source rule matches the given events")
		}
		failed := 0
		for _, p := range previews {
			fmt.Fprintf(cmd.OutOrStdout(), "--- %s (status %s, %s, events %s) ---\n", p.Run, p.Status, p.Mode, strings.Join(p.Events, ","))
			if p.Error != "" {
				failed++
				fmt.Fprintf(cmd.OutOrStdout(), "error: %s\n", p.Error)
				continue
			}
			fmt.Fprint(cmd.OutOrStdout(), p.Order)
		}
		if failed > 0 {
			return fmt.Errorf("%d of %d units failed to render", failed, len(previews))
		}
		return nil
	}}
	render.Flags().StringVar(&eventFile, "event", "", "Event JSON object, array or JSON lines; - for stdin")
	_ = render.MarkFlagRequired("event")
	c.AddCommand(render)
	c.AddCommand(newOrchestrateNoticesCommand())
	c.AddCommand(&cobra.Command{Use: "supervise NAME", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		// Only source plugins may see plugin-only values from here on.
		dispatch.HoldPluginOnlyEnv()
		return dispatch.SuperviseOrchestrator(cmd.Context(), args[0], dispatch.DefaultOrchestratorDriver())
	}})
	return c
}

// parseSourceEvents accepts one event object, an array, or JSON lines.
func parseSourceEvents(b []byte) ([]dispatch.SourceEvent, error) {
	trimmed := bytes.TrimSpace(b)
	if bytes.HasPrefix(trimmed, []byte("[")) {
		var events []dispatch.SourceEvent
		return events, json.Unmarshal(trimmed, &events)
	}
	var events []dispatch.SourceEvent
	scan := bufio.NewScanner(bytes.NewReader(trimmed))
	scan.Buffer(make([]byte, 4096), 4*1024*1024)
	for scan.Scan() {
		if len(bytes.TrimSpace(scan.Bytes())) == 0 {
			continue
		}
		var event dispatch.SourceEvent
		if e := json.Unmarshal(scan.Bytes(), &event); e != nil {
			// A pretty-printed single object spans several lines.
			if e2 := json.Unmarshal(trimmed, &event); e2 == nil {
				return []dispatch.SourceEvent{event}, nil
			}
			return nil, e
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no events")
	}
	return events, scan.Err()
}

// resolveSpecPaths stores relative templates and pause_file as absolute
// paths beside the input spec, so the CLI and API resolve the same files.
func resolveSpecPaths(file string, b []byte) ([]byte, error) {
	parsed, e := dispatch.ParseOrchestrator(string(b))
	if e != nil {
		return nil, e
	}
	relativePause := parsed.PauseFile != "" && !filepath.IsAbs(parsed.PauseFile)
	if parsed.Source.Plugin == "" && !relativePause {
		return b, nil
	}
	beside := func(p string) (string, error) {
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(file), p)
		}
		return filepath.Abs(p)
	}
	if relativePause {
		if parsed.PauseFile, e = beside(parsed.PauseFile); e != nil {
			return nil, e
		}
	}
	for i := range parsed.Source.Rules {
		p, e := beside(parsed.Source.Rules[i].Template)
		if e != nil {
			return nil, e
		}
		if _, e = os.ReadFile(p); e != nil {
			return nil, e
		}
		parsed.Source.Rules[i].Template = p
	}
	front, e := yaml.Marshal(parsed)
	if e != nil {
		return nil, e
	}
	return []byte("---\n" + string(front) + "---\n" + parsed.Brief + "\n"), nil
}
