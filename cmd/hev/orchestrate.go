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
	c.AddCommand(newFollowCommand())
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
		id, e := dispatch.SayOrchestratorReceipt(args[0], strings.Join(args[1:], " "), "", "")
		if e != nil {
			return e
		}
		if e = dispatch.LaunchOrchestrator(args[0]); e != nil {
			return e
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s note queued for o-%s: inbox.md and the next round's first prompt\n", id, args[0])
		return nil
	}})
	var goal, runs, every, until, host, name, harness, model, effort, reply, lifetime string
	start := &cobra.Command{Use: "start --goal TEXT --runs SELECTOR", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if name == "" {
			name = fmt.Sprintf("adhoc-%d", time.Now().UnixNano())
		}
		s := dispatch.OrchestratorSpec{Kind: "orchestrator", Name: name, Goal: goal, Host: host, Mode: "once", Harness: harness, Model: model, Effort: effort, Reply: reply, Idle: "skip", Agent: "on", Authority: []string{"say", "notify"}}
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
	start.Flags().StringVar(&harness, "harness", "codex", "Fresh-session harness: codex, claude or agy")
	start.Flags().StringVar(&model, "model", "", "Model")
	start.Flags().StringVar(&effort, "effort", "", "Reasoning effort: low, medium, high, xhigh or max")
	start.Flags().StringVar(&reply, "reply", "", "Owner session (required for mine)")
	c.AddCommand(start)
	var asJSON, orphaned bool
	ls := &cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if orphaned {
			return printOrphans(cmd.OutOrStdout(), orphanRows(cmd.Context(), orphanInventory(), map[string]bool{"orch": true}), asJSON)
		}
		rows, e := dispatch.ListOrchestrators()
		if e != nil {
			return e
		}
		probe := newOwnerProber()
		owned := make([]orchestratorOwnerRow, len(rows))
		for i, r := range rows {
			label := r.Host
			if label == "" {
				label = selfHostLabel()
			}
			owned[i] = orchestratorOwnerRow{OrchestratorState: r, Owner: r.Reply, OwnerStatus: probe.status(cmd.Context(), r.Reply, label)}
		}
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(owned)
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
		defer w.Flush()
		fmt.Fprintln(w, "NAME\tHOST\tMODE\tSTATE\tOWNER\tROUNDS\tNEXT\tRUNS\tTODAY\tSPEND\tGOAL")
		for _, r := range owned {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%d\t%d\tunknown\t%s\n", r.Name, r.Host, r.Mode, r.State, ownerColumn(r.Owner, r.OwnerStatus), r.Rounds, roundTime(r.Next), r.ManagedRuns, r.RunsToday, r.Goal)
		}
		return nil
	}}
	ls.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	ls.Flags().BoolVar(&orphaned, "orphaned", false, "List live orchestrators on reachable hosts whose owner session is gone, with the adopt command")
	c.AddCommand(ls)
	for _, action := range []string{"show", "logs", "wake", "pause", "resume", "stop", "adopt", "retire", "handoff"} {
		c.AddCommand(newOrchestrateControl(action))
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
	var scheduled bool
	supervise := &cobra.Command{Use: "supervise NAME", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		// A scheduled-task launch (Windows) carries its environment in a file.
		done, err := dispatch.AdoptOrchestratorLaunch(args[0], scheduled)
		if err != nil {
			return err
		}
		// Only source plugins may see plugin-only values from here on.
		dispatch.HoldPluginOnlyEnv()
		err = dispatch.SuperviseOrchestrator(cmd.Context(), args[0], dispatch.DefaultOrchestratorDriver())
		done(err)
		return err
	}}
	supervise.Flags().BoolVar(&scheduled, "scheduled", false, "Started by its scheduled task (Windows)")
	c.AddCommand(supervise)
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
