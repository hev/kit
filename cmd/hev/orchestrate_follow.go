package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/followthrough"
	"github.com/hev/kit/internal/peers"
	"github.com/hev/kit/internal/version"
	"github.com/hev/kit/skills"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var followInventory = dispatch.Inventory
var followRPC = dispatch.RPC
var followStart = startFollowOrchestrator
var followNow = time.Now
var followOwnAddress = func(ctx context.Context) (string, error) { return peers.New().OwnAddress(ctx) }

// `hev orchestrate follow` is registered by newOrchestrateCommand. The
// pre-rename `hev expedite` spelling stays as a hidden alias because scripts
// and skills on hosts still call it.
func init() { rootCmd.AddCommand(newExpediteAliasCommand()) }

func newExpediteAliasCommand() *cobra.Command {
	c := newFollowCommand()
	c.Use = "expedite"
	c.Hidden = true
	c.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		fmt.Fprintln(cmd.ErrOrStderr(), "hev expedite is deprecated; use hev orchestrate follow")
	}
	return c
}

func followPlan(text string) (followthrough.Plan, error) {
	var p followthrough.Plan
	raw, ok := dispatch.FollowPlanJSON(text)
	if !ok && raw == "" {
		return p, fmt.Errorf("not a follow-through orchestrator order")
	}
	if !ok {
		return p, fmt.Errorf("invalid follow-through snapshot")
	}
	err := json.Unmarshal([]byte(raw), &p)
	return p, err
}
func followOrder(o dispatch.Order, p followthrough.Plan) (dispatch.Order, string, error) {
	if p.Authority != "review" && p.Authority != "merge-on-green" {
		return o, "", fmt.Errorf("invalid authority")
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	tpl, err := skills.FS.ReadFile("hev-orchestrate-follow/order.md")
	if err != nil {
		return o, "", err
	}
	skill, err := skills.FS.ReadFile("hev-orchestrate-follow/SKILL.md")
	if err != nil {
		return o, "", err
	}
	// The compact copy under the legacy marker keeps the order readable by
	// clients from before the rename (their status and stop need it).
	compact, _ := json.Marshal(p)
	body := strings.NewReplacer("{{authority}}", p.Authority, "{{plan}}", string(b), "{{legacy_plan}}", string(compact), "{{version}}", version.Version, "{{skill}}", string(skill)).Replace(string(tpl))
	task, acceptance, _ := strings.Cut(body, "\n## Acceptance\n")
	var accepts []string
	for _, line := range strings.Split(acceptance, "\n") {
		if strings.HasPrefix(line, "- ") {
			accepts = append(accepts, strings.TrimPrefix(line, "- "))
		}
	}
	_, text, err := dispatch.Inline(o, task, accepts, map[string]bool{}, followNow())
	if err != nil {
		return o, "", err
	}
	text = strings.Replace(text, "---\n\n", "every: "+o.Every+"\nmax_lifetime: "+o.MaxLifetime+"\nuntil: all followed runs merged or abandoned\n---\n\n", 1)
	parsed, err := dispatch.Parse(text)
	return parsed, text, err
}
func newFollowCommand() *cobra.Command {
	var host, path, authority, every, lifetime, repo, evidence, harness, model string
	c := &cobra.Command{Use: "follow", Short: "Start a follow-through orchestrator for this session's open runs", Args: cobra.NoArgs}
	c.PersistentFlags().StringVar(&path, "config", "", "Dispatch configuration")
	c.PersistentFlags().StringVar(&host, "host", "", "Headless host (otherwise configured default)")
	c.Flags().StringVar(&harness, "harness", "", "Round harness (config or agy)")
	c.Flags().StringVar(&model, "model", "", "Round model (config or gemini-4-neon-medium)")
	c.Flags().StringVar(&authority, "authority", "review", "review or merge-on-green")
	c.Flags().StringVar(&every, "every", "10m", "Round interval")
	c.Flags().StringVar(&lifetime, "max-lifetime", "24h", "Maximum lifetime")
	c.Flags().StringVar(&repo, "repo", "", "Repository for the orchestrator worktree (defaults to first followed run)")
	load := func() (dispatch.Config, error) { path = dispatch.ConfigPath(path); return dispatch.Load(path) }
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		cfg, err := load()
		if err != nil {
			return err
		}
		reply, err := followOwnAddress(cmd.Context())
		if err != nil {
			return err
		}
		inv := followInventory(cfg, "")
		// PR merge state is authoritative; dispatch completion alone is not merging.
		for hn, r := range inv {
			for i, row := range r.Rows {
				if !followthrough.MatchesReply(row.Reply, reply) || row.PR == "" {
					continue
				}
				b, e := exec.CommandContext(cmd.Context(), "gh", "pr", "view", row.PR, "--json", "state").Output()
				if e != nil {
					return fmt.Errorf("snapshot PR state %s: %w", row.PR, e)
				}
				var pr followthrough.PR
				if e = json.Unmarshal(b, &pr); e != nil {
					return e
				}
				if pr.State == "MERGED" {
					r.Rows[i].State = "merged"
				}
			}
			inv[hn] = r
		}
		runs, err := followthrough.Collect(inv, reply)
		if err != nil {
			return err
		}
		if len(runs) == 0 {
			return fmt.Errorf("no open runs for %s", reply)
		}
		selected := host
		if selected == "" {
			selected = cfg.DefaultHost
		}
		h, ok := cfg.Hosts[selected]
		if !ok {
			return fmt.Errorf("set --host or default_host for a headless target")
		}
		used, err := inventoryNames(inv)
		if err != nil {
			return err
		}
		for _, r := range inv {
			for _, row := range r.Rows {
				if followthrough.MatchesReply(row.Reply, reply) && row.Until == "all followed runs merged or abandoned" && row.State != "stopped" && row.State != "complete" && row.State != "handed-off" && row.Schedule.End.IsZero() {
					return fmt.Errorf("follow-through orchestrator %s already exists; inspect or stop it first", row.Name)
				}
			}
		}
		// An owner on a build before the rename only recognizes expedite-*.
		name := dispatch.FollowNamePrefixFor(inv[selected]) + followNow().UTC().Format("20060102-150405")
		if used[name] {
			return fmt.Errorf("follow-through orchestrator name already exists")
		}
		if repo == "" {
			repo = runs[0].Repo
		}
		selectedHarness, selectedModel := followModel(cfg, harness, model)
		o, text, err := followOrder(dispatch.Order{Name: name, Host: selected, Repo: repo, Reply: reply, Every: every, MaxLifetime: lifetime, Harness: selectedHarness, Model: selectedModel}, followthrough.Plan{Name: name, Reply: reply, Authority: authority, Runs: runs})
		if err != nil {
			return err
		}
		return followStart(cmd, cfg, h, o, text, path)
	}
	for _, action := range []string{"status", "stop"} {
		action := action
		short := "Show this session's follow-through orchestrators and the runs they follow"
		if action == "stop" {
			short = "Stop this session's follow-through orchestrators and remove their admission scope"
		}
		sub := &cobra.Command{Use: action + " [NAME]", Short: short, Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := load()
			if err != nil {
				return err
			}
			reply := ""
			if len(args) == 0 {
				reply, err = followOwnAddress(cmd.Context())
				if err != nil {
					return err
				}
			}
			found := false
			var stopErrors []error
			inv := followInventory(cfg, host)
			for _, hn := range dispatch.SortedHosts(inv) {
				r := inv[hn]
				if r.Error != "" {
					return fmt.Errorf("%s: %s", hn, r.Error)
				}
				for _, row := range r.Rows {
					if row.Until != "all followed runs merged or abandoned" || (len(args) > 0 && row.Name != args[0] && row.Name != "o-"+args[0]) || (len(args) == 0 && !followthrough.MatchesReply(row.Reply, reply)) {
						continue
					}
					found = true
					h := cfg.Hosts[hn]
					shown, err := followRPC(cmd.Context(), h, dispatch.Request{Action: "show", Name: row.Name})
					if err != nil {
						return err
					}

					roundSpec, _ := dispatch.ParseOrchestrator(shown.Files["spec.md"])
					fmt.Fprintf(cmd.OutOrStdout(), "%s/%s %s harness=%s model=%s rounds=%d last=%s next=%s reason=%s\n", hn, row.Name, row.State, roundSpec.Harness, roundSpec.Model, row.Schedule.Rounds, roundTime(row.Schedule.Last), roundTime(row.Schedule.Next), row.Schedule.Reason)
					var state dispatch.OrchestratorState
					_ = json.Unmarshal([]byte(shown.Files["state.json"]), &state)
					for i, a := range state.Attempts {
						fmt.Fprintf(cmd.OutOrStdout(), "  attempt=%d harness=%s model=%s status=%s %s\n", i+1, a.Harness, a.Model, a.Status, a.Error)
					}
					if len(shown.Notices) > 0 {
						newest := shown.Notices[len(shown.Notices)-1]
						fmt.Fprintf(cmd.OutOrStdout(), "  undelivered=%d newest=%s error=%s\n", len(shown.Notices), newest.Message, newest.Error)
					}
					p, err := followPlan(shown.Files["order.md"])
					if err != nil {
						return err
					}
					for _, run := range p.Runs {
						fmt.Fprintf(cmd.OutOrStdout(), "  follows %s/%s\n", run.Host, run.Name)
					}
					if action == "stop" {
						if row.State == "stopped" || row.State == "complete" || row.State == "handed-off" {
							continue
						}
						if _, err = followRPC(cmd.Context(), h, dispatch.Request{Action: "stop", Name: row.Name}); err != nil {
							stopErrors = append(stopErrors, fmt.Errorf("stop %s/%s: %w", hn, row.Name, err))
							continue
						}
						// Remove admission registrations only after stopping their owner.
						seen := map[string]bool{hn: true}
						targets := append([]followthrough.Run{}, p.Runs...)
						if cfg.Validation.Host != "" {
							targets = append(targets, followthrough.Run{Host: cfg.Validation.Host})
						}
						for _, run := range targets {
							if !seen[run.Host] {
								seen[run.Host] = true
								if _, err = followRPC(cmd.Context(), cfg.Hosts[run.Host], dispatch.Request{Action: "orchestrator-scope-remove", Name: p.Name}); err != nil {
									stopErrors = append(stopErrors, fmt.Errorf("remove admission %s: %w", run.Host, err))
								}
							}
						}
					}
				}
			}
			if !found {
				return fmt.Errorf("no follow-through orchestrator found")
			}
			return errors.Join(stopErrors...)
		}}
		c.AddCommand(sub)
	}
	round := &cobra.Command{Use: "round ORDER.md", Short: "Apply reviewed evidence with fresh CI and authority checks", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := load()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		p, err := followPlan(string(b))
		if err != nil {
			return err
		}
		ev := map[string]followthrough.Evidence{}
		if evidence != "" {
			b, err = os.ReadFile(evidence)
			if err != nil {
				return err
			}
			if err = json.Unmarshal(b, &ev); err != nil {
				return err
			}
		}
		d := &followDriver{cfg: cfg, out: cmd.OutOrStdout(), reply: p.Reply}
		done, err := followthrough.Round(cmd.Context(), p, ev, d)
		if err != nil {
			return err
		}
		if done {
			fmt.Fprintln(cmd.OutOrStdout(), "All followed runs merged or abandoned; write ../until-met.json with evidence.")
		}
		return nil
	}}
	round.Flags().StringVar(&evidence, "evidence", "", "Review evidence JSON")
	c.AddCommand(round)
	return c
}

type followDriver struct {
	cfg   dispatch.Config
	out   io.Writer
	reply string
}

func (d *followDriver) Inbox(ctx context.Context) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	c := exec.CommandContext(ctx, exe, "peers", "inbox", "--pending", "--json")
	c.Stdout = d.out
	c.Stderr = d.out
	if err = c.Run(); err != nil {
		return err
	}
	c = exec.CommandContext(ctx, exe, "peers", "chat", "ls")
	c.Stdout = d.out
	c.Stderr = d.out
	return c.Run()
}
func (d *followDriver) Inspect(ctx context.Context, r followthrough.Run) (dispatch.Row, followthrough.PR, error) {
	var row dispatch.Row
	var pr followthrough.PR
	h, ok := d.cfg.Hosts[r.Host]
	if !ok {
		return row, pr, fmt.Errorf("unknown host %s", r.Host)
	}
	inv, err := followRPC(ctx, h, dispatch.Request{Action: "list"})
	if err != nil {
		return row, pr, err
	}
	found := false
	for _, v := range inv.Rows {
		if v.Name == r.Name {
			row = v
			found = true
			break
		}
	}
	if !found {
		return row, pr, fmt.Errorf("run missing; explicitly resolve abandonment")
	}
	if row.PR == "" {
		return row, pr, nil
	}
	b, err := exec.CommandContext(ctx, "gh", "pr", "view", row.PR, "--json", "headRefOid,state,statusCheckRollup").Output()
	if err != nil {
		return row, pr, err
	}
	err = json.Unmarshal(b, &pr)
	return row, pr, err
}
func (d *followDriver) Amend(ctx context.Context, r followthrough.Run, text string) error {
	o, err := dispatch.Parse(text)
	if err != nil {
		return err
	}
	if o.Name != r.Name || o.Repo != r.Repo || o.Host != r.Host {
		return fmt.Errorf("amend must preserve run identity")
	}
	q := dispatch.Request{Action: "amend", Name: r.Name, Order: text}
	if os.Getenv("HEV_ORCHESTRATOR") != "" {
		q.Action = "rework"
		q.Note = "Apply the reviewed amended order below within the existing worktree and branch:\n\n" + text
	}
	_, err = followRPC(ctx, d.cfg.Hosts[r.Host], q)
	return err
}
func (d *followDriver) Merge(ctx context.Context, r followthrough.Run, pr, head string) error {
	b, err := exec.CommandContext(ctx, "gh", "pr", "view", pr, "--json", "headRefOid,isDraft").Output()
	if err != nil {
		return err
	}
	var current struct {
		Head  string `json:"headRefOid"`
		Draft bool   `json:"isDraft"`
	}
	if err = json.Unmarshal(b, &current); err != nil {
		return err
	}
	if current.Head != head {
		return fmt.Errorf("PR head changed after review")
	}
	if current.Draft {
		if err = exec.CommandContext(ctx, "gh", "pr", "ready", pr).Run(); err != nil {
			return err
		}
	}
	// Marking a draft ready may schedule new checks. Re-read CI after that
	// transition; the earlier round observation alone cannot authorize merging.
	b, err = exec.CommandContext(ctx, "gh", "pr", "view", pr, "--json", "headRefOid,state,statusCheckRollup").Output()
	if err != nil {
		return err
	}
	var fresh followthrough.PR
	if err = json.Unmarshal(b, &fresh); err != nil {
		return err
	}
	if fresh.Head != head || fresh.State != "OPEN" || !fresh.Green() {
		return fmt.Errorf("PR is not green on the reviewed head after readiness transition")
	}
	return exec.CommandContext(ctx, "gh", "pr", "merge", pr, "--merge", "--match-head-commit", head).Run()
}
func (d *followDriver) Report(ctx context.Context, line string) error {
	fmt.Fprintln(d.out, line)
	if n, ok := followAttention(line, time.Now()); ok {
		emitFollowAttention(n)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	c := exec.CommandContext(ctx, exe, "peers", "send", d.reply, line)
	c.Stdout = d.out
	c.Stderr = d.out
	return c.Run()
}

// The legacy snapshot and evidence utility remain readable. New launches use
// the common runner, with the deliberately narrow follow-through preset authority.
func followModel(cfg dispatch.Config, harness, model string) (string, string) {
	if harness == "" {
		harness = cfg.FollowDefaults().Harness
	}
	if harness == "" {
		harness = dispatch.DefaultOrchestratorHarness
	}
	if model == "" {
		model = cfg.FollowDefaults().Model
	}
	if model == "" && harness == dispatch.DefaultOrchestratorHarness {
		model = dispatch.DefaultOrchestratorModel
	}
	return harness, model
}
func followPreset(o dispatch.Order, text string) dispatch.OrchestratorSpec {
	if o.Harness == "" {
		o.Harness = dispatch.DefaultOrchestratorHarness
	}
	if o.Model == "" && o.Harness == dispatch.DefaultOrchestratorHarness {
		o.Model = dispatch.DefaultOrchestratorModel
	}
	s := dispatch.OrchestratorSpec{Kind: "orchestrator", Name: o.Name, Goal: "Follow through on this session's open runs", Host: o.Host, Mode: "once", Until: "goal", Every: o.Every, MaxLifetime: o.MaxLifetime, Harness: o.Harness, Model: o.Model, Reply: o.Reply, Agent: "on", Idle: "skip", Authority: []string{"say", "rework", "notify", "validate"}}
	if p, err := followPlan(text); err == nil {
		if p.Authority == "merge-on-green" {
			s.Authority = append(s.Authority, "merge")
		}
		for _, r := range p.Runs {
			s.Manages.Runs.Names = append(s.Manages.Runs.Names, r.Name)
		}
	}
	s.Brief = "Follow the snapshot below. Read-only exact-head review with gh and reading reports/worktrees is always allowed. Use hev dispatch validate NAME --host HOST for independent verification; read validation.json and verdict.json next round and bind the verdict to the current head. Use say or rework for interventions; report merge-ready results to the owner. If validation is unavailable, write [needs you] with the reason once. Only for an open decision the owner must make, write [needs you] needs-decision KEY: QUESTION (KEY a short word); only that raises a needs-you notice. Write checkpoint.md before exiting. A run in state closed (hev dispatch close: finished elsewhere or superseded) is settled like an abandoned one; do not review, rework or report it as needing you. When all followed runs are merged, closed or explicitly abandoned, write goal-met.json containing {\"met\":true}.\n\n" + text
	return s
}
func startFollowOrchestrator(cmd *cobra.Command, cfg dispatch.Config, h dispatch.Host, o dispatch.Order, text, path string) error {
	s := followPreset(o, text)
	s.Fallbacks = cfg.Orchestrate.Follow.Fallbacks
	if s.Fallbacks == nil {
		s.Fallbacks = cfg.OrchestratorFallbacks()
	}
	if s.Fallbacks == nil {
		s.Fallbacks = dispatch.FallbackChain{{Harness: "codex"}}
	}
	b, e := yaml.Marshal(s)
	if e != nil {
		return e
	}
	spec := "---\n" + string(b) + "---\n" + s.Brief
	p, e := followPlan(text)
	if e != nil {
		return e
	}
	applied := []dispatch.Host{}
	rollback := func() {
		for _, target := range applied {
			_, _ = followRPC(cmd.Context(), target, dispatch.Request{Action: "orchestrator-scope-remove", Name: s.Name})
		}
	}
	owner := h
	if owner.Local {
		owner.Local = false
		owner.SSH = o.Host
	}
	seen := map[string]bool{o.Host: true}
	targets := append([]followthrough.Run{}, p.Runs...)
	if cfg.Validation.Host != "" {
		targets = append(targets, followthrough.Run{Host: cfg.Validation.Host})
	}
	for _, run := range targets {
		if seen[run.Host] {
			continue
		}
		seen[run.Host] = true
		target, ok := cfg.Hosts[run.Host]
		if !ok {
			rollback()
			return fmt.Errorf("admission host %s is not configured", run.Host)
		}
		registration, err := followRPC(cmd.Context(), target, dispatch.Request{Action: "orchestrator-apply", Name: s.Name, Order: spec, ScopeOwner: &owner, AdmissionOnly: true})
		if err == nil && !registration.ScopeRegistered {
			err = fmt.Errorf("host does not support admission-only registration; upgrade it first")
		}
		if err != nil {
			e = err
			rollback()
			return fmt.Errorf("admission host %s: %w", run.Host, e)
		}
		applied = append(applied, target)
	}
	_, e = followRPC(cmd.Context(), h, dispatch.Request{Action: "orchestrator-apply", Name: s.Name, Order: spec, Note: text})
	if e != nil {
		rollback()
	}
	if e == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "started", s.Name)
	}
	return e
}

// followAttention maps a round line "HOST/NAME: ... [needs you] TEXT" to an
// attention notice. Only an open decision ("[needs you] needs-decision KEY:
// QUESTION") raises needs-you, keyed by the run and the decision, so a round
// repeating the same question is one notice. Anything else after
// [needs you] (no PR, a merge to make, an error) is a record on the opt-in
// done kind, keyed by the line and the day.
func followAttention(line string, now time.Time) (attention.Notice, bool) {
	run, rest, ok := strings.Cut(line, ": ")
	_, text, needs := strings.Cut(rest, "[needs you]")
	if !ok || !needs {
		return attention.Notice{}, false
	}
	host, name, ok := strings.Cut(run, "/")
	if !ok {
		host, name = "", run
	}
	text = strings.TrimSpace(text)
	links := attention.Links{Run: "hev dispatch show " + name + " --host " + host}
	if key, question, ok := dispatch.NeedsDecision(text); ok {
		return attention.New(attention.KindNeedsYou, attention.SourceFollow, host, name, "decision|"+key+"|"+strings.Join(strings.Fields(question), " "), "needs-decision", text, links), true
	}
	return attention.New(attention.KindDone, attention.SourceFollow, host, name, line+"|"+now.UTC().Format("2006-01-02"), "reported", text, links), true
}

var emitFollowAttention = func(n attention.Notice) { _ = attention.Emit(n, "") }
