package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/hostlabel"
	"github.com/hev/kit/internal/workspace"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var orchestrateApplyRPC = dispatch.RPC

func init() { rootCmd.AddCommand(newOrchestrateCommand()) }
func newOrchestrateCommand() *cobra.Command {
	c := &cobra.Command{Use: "orchestrate", Short: "Manage opt-in orchestrators with durable checkpoints"}
	c.AddCommand(newFollowCommand())
	var file string
	var dry bool
	var scopeOwner, seed string
	apply := &cobra.Command{Use: "apply -f FILE", Short: "Apply an orchestrator and register scope on managed hosts", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
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
		return applyOrchestratorFile(cmd, string(b), scopeOwner, seed, dry)
	}}
	apply.Flags().StringVarP(&file, "file", "f", "", "Markdown spec")
	_ = apply.MarkFlagRequired("file")
	apply.Flags().StringVar(&scopeOwner, "scope-only-owner", "", "Register admission scope only; configured owner host controls lifecycle")
	apply.Flags().BoolVar(&dry, "dry-run", false, "Validate without writing state or launching a session")
	apply.Flags().StringVar(&seed, "seed-checkpoint", "", "Record FILE as round 000, the checkpoint round 001 reads")
	c.AddCommand(apply)
	c.AddCommand(&cobra.Command{Use: "say NAME MSG", Args: cobra.MinimumNArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireLocalOrchestrator(args[0]); err != nil {
			return err
		}
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
		selected, err := currentWorkspace()
		if err != nil {
			return err
		}
		if err = selected.RequireHost(selfHostLabel()); err != nil {
			return err
		}
		if host != "" {
			if err = selected.RequireHost(host); err != nil {
				return err
			}
		}
		s := dispatch.OrchestratorSpec{WorkspaceName: scopedOrchestratorName(selected), Kind: "orchestrator", Name: name, Goal: goal, Host: host, Mode: "once", Harness: harness, Model: model, Effort: effort, Reply: reply, Idle: "skip", Agent: "on", Authority: []string{"say", "notify"}}
		if harness == dispatch.DefaultOrchestratorHarness && !cmd.Flags().Changed("model") {
			s.Model = dispatch.DefaultOrchestratorModel
		}
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
	start.Flags().StringVar(&harness, "harness", dispatch.DefaultOrchestratorHarness, "Fresh-session harness: codex, claude, agy or grok")
	start.Flags().StringVar(&model, "model", "", "Model")
	start.Flags().StringVar(&effort, "effort", "", "Reasoning effort: low, medium, high, xhigh or max")
	start.Flags().StringVar(&reply, "reply", "", "Owner session (required for mine)")
	c.AddCommand(start)
	var asJSON, orphaned bool
	ls := &cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		selected, e := currentWorkspace()
		if e != nil {
			return e
		}
		if orphaned {
			inv := orphanInventory()
			for name := range inv {
				if !selected.AllowsHost(name) {
					delete(inv, name)
				}
			}
			return printOrphans(cmd.OutOrStdout(), orphanRows(cmd.Context(), inv, map[string]bool{"orch": true}), asJSON)
		}
		rows, e := dispatch.ListOrchestrators()
		if e != nil {
			return e
		}
		probe := newOwnerProber()
		owned := make([]orchestratorOwnerRow, 0, len(rows))
		for _, r := range rows {
			label := r.Host
			if label == "" {
				label = selfHostLabel()
			}
			if !selected.AllowsHost(label) || !orchestratorWorkspaceMatches(selected, r.WorkspaceName) {
				continue
			}
			owned = append(owned, orchestratorOwnerRow{OrchestratorState: r, Owner: r.Reply, OwnerStatus: probe.status(cmd.Context(), r.Reply, label)})
		}
		if len(selected.Hosts) > 0 {
			if cfg, err := loadWorkspaceDispatch(dispatch.ConfigPath("")); err == nil {
				for name, h := range cfg.Hosts {
					if h.Local {
						delete(cfg.Hosts, name)
					}
				}
				inv := dispatch.Inventory(cfg, "")
				for _, name := range dispatch.SortedHosts(inv) {
					for _, row := range inv[name].Rows {
						if row.Kind != "orch" || !orchestratorWorkspaceMatches(selected, row.WorkspaceName) {
							continue
						}
						state := dispatch.OrchestratorState{Name: strings.TrimPrefix(row.Name, "o-"), Host: name, State: row.State, Rounds: row.Schedule.Rounds, Next: row.Schedule.Next, Reply: row.Reply, WorkspaceName: row.WorkspaceName}
						owned = append(owned, orchestratorOwnerRow{OrchestratorState: state, Owner: row.Reply, OwnerStatus: probe.status(cmd.Context(), row.Reply, name)})
					}
				}
			}
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
		if err := requireLocalOrchestrator(args[0]); err != nil {
			return err
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
	c.AddCommand(newSwarmCommand())
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
func applyOrchestratorFile(cmd *cobra.Command, spec, scopeOwner, seed string, dry bool) error {
	parsed, err := dispatch.ParseOrchestrator(spec)
	if err != nil {
		return err
	}
	selected, err := currentWorkspace()
	if err != nil {
		return err
	}
	if parsed.WorkspaceName != "" && parsed.WorkspaceName != selected.Name {
		return fmt.Errorf("workspace boundary: orchestrator spec belongs to %q", parsed.WorkspaceName)
	}
	if parsed.WorkspaceName == "" && scopedOrchestratorName(selected) != "" {
		spec = strings.Replace(strings.ReplaceAll(spec, "\r\n", "\n"), "---\n", "---\nworkspace: "+selected.Name+"\n", 1)
	}
	self := selfHostLabel()
	target := parsed.Host
	if target == "" || hostlabel.Same(target, self) || (scopeOwner != "" && hostlabel.Same(target, scopeOwner)) {
		target = self
	}
	hosts := parsed.ManagedRemoteHosts(target)
	if scopeOwner != "" {
		if hostlabel.Same(scopeOwner, target) {
			return fmt.Errorf("scope owner and target must be different hosts")
		}
		if err := selected.RequireHost(scopeOwner); err != nil {
			return err
		}
		hosts = nil
	}

	routes := append([]string{target}, hosts...)
	for _, host := range routes {
		if err := selected.RequireHost(host); err != nil {
			return err
		}
	}
	var cfg dispatch.Config
	if !hostlabel.Same(target, self) || len(hosts) > 0 {
		cfg, err = loadWorkspaceDispatch(dispatch.ConfigPath(""))
		if err != nil && !(os.IsNotExist(err) && dry && scopeOwner == "" && len(hosts) == 0) {
			return fmt.Errorf("dispatch configuration for %s: %w", strings.Join(routes, ", "), err)
		}
	}
	for _, host := range routes {
		if hostlabel.Same(host, self) {
			continue
		}
		if _, err := cfg.OrchestratorHost(host); err != nil {
			// Keep offline validation for owner-only specs, including
			// specs generated by swarm before dispatch is configured.
			if dry && scopeOwner == "" && len(hosts) == 0 {
				target = self
				continue
			}
			return err
		}
	}

	ctx := dispatch.WorkspaceContext(cmd.Context(), selected)
	applyRemote := func(host string, req dispatch.Request) (dispatch.Response, error) {
		route, err := cfg.OrchestratorHost(host)
		if err != nil {
			return dispatch.Response{}, err
		}
		bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return orchestrateApplyRPC(bounded, route, req)
	}
	register := func(host, owner string, dry bool) error {
		copy, err := dispatch.OrchestratorSpecForHost(spec, host)
		if err != nil {
			return err
		}
		if hostlabel.Same(host, self) {
			_, err = dispatch.RegisterOrchestratorScopeForOwner(copy, owner, dry)
		} else if !dry {
			var response dispatch.Response
			response, err = applyRemote(host, dispatch.Request{
				Action: "orchestrator-apply", Name: parsed.Name, Order: copy,
				AdmissionOnly: true, ScopeOwnerLabel: owner,
			})
			if err == nil && !response.ScopeRegistered {
				err = fmt.Errorf("host did not acknowledge scope registration; check its hev version")
			}
		}
		return err
	}

	switch {
	case scopeOwner != "":
		if err := register(target, scopeOwner, dry); err != nil {
			return fmt.Errorf("scope on %s (owner %s): %w", target, scopeOwner, err)
		}
	case hostlabel.Same(target, self):
		if _, err := dispatch.ApplyOrchestrator(spec, dry); err != nil {
			return err
		}
	default:
		if seed != "" {
			return fmt.Errorf("--seed-checkpoint is not supported for remote host %s; run on %s directly", target, target)
		}
		if !dry {
			if _, err := applyRemote(target, dispatch.Request{Action: "orchestrator-apply", Name: parsed.Name, Order: spec}); err != nil {
				return err
			}
		}
	}

	out := cmd.OutOrStdout()
	if dry {
		fmt.Fprintln(out, "valid", parsed.Name)
	}
	if scopeOwner != "" {
		if !dry {
			fmt.Fprintf(out, "registered scope on %s (owner %s)\n", target, scopeOwner)
		}
		return nil
	}
	sort.Strings(hosts)
	var failures []error
	for _, host := range hosts {
		if dry {
			fmt.Fprintf(out, "would register scope on %s (owner %s)\n", host, target)
		} else if err := register(host, target, false); err != nil {
			failures = append(failures, fmt.Errorf("scope registration on %s: %w", host, err))
		} else {
			fmt.Fprintf(out, "registered scope on %s (owner %s)\n", host, target)
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	if dry {
		return nil
	}
	if parsed.AllowNested {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: nesting enabled; maximum depth is 2")
	}
	if seed != "" {
		if err := dispatch.SeedOrchestratorCheckpoint(parsed.Name, seed); err != nil {
			return err
		}
	}
	if hostlabel.Same(target, self) {
		if err := dispatch.LaunchOrchestrator(parsed.Name); err != nil {
			return err
		}
		fmt.Fprintln(out, "applied", parsed.Name)
	} else {
		fmt.Fprintf(out, "applied %s on %s\n", parsed.Name, target)
	}
	return nil
}

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

func scopedOrchestratorName(w workspace.Selection) string {
	if w.Name == workspace.Default && len(w.Hosts) == 0 {
		return ""
	}
	return w.Name
}

func orchestratorWorkspaceMatches(w workspace.Selection, name string) bool {
	if name == "" {
		name = workspace.Default
	}
	return w.Name == name
}

func requireLocalOrchestrator(name string) error {
	selected, err := currentWorkspace()
	if err != nil {
		return err
	}
	if err = selected.RequireHost(selfHostLabel()); err != nil {
		return err
	}
	spec, err := dispatch.LoadOrchestrator(name)
	if err != nil {
		return err
	}
	if !orchestratorWorkspaceMatches(selected, spec.WorkspaceName) {
		return fmt.Errorf("workspace boundary: orchestrator is outside workspace %q", selected.Name)
	}
	return nil
}
