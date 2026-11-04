package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/expedite"
	"github.com/hev/kit/internal/peers"
	"github.com/hev/kit/internal/version"
	"github.com/hev/kit/skills"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var expediteInventory = dispatch.Inventory
var expediteRPC = dispatch.RPC
var expediteStart = startExpediteOrchestrator
var expediteNow = time.Now
var expediteOwnAddress = func(ctx context.Context) (string, error) { return peers.New().OwnAddress(ctx) }

func init() { rootCmd.AddCommand(newExpediteCommand()) }

func expeditePlan(text string) (expedite.Plan, error) {
	var p expedite.Plan
	_, tail, ok := strings.Cut(text, "<!-- hev-expedite-plan\n")
	if !ok {
		return p, fmt.Errorf("not an expediter order")
	}
	raw, _, ok := strings.Cut(tail, "\n-->")
	if !ok {
		return p, fmt.Errorf("invalid expediter snapshot")
	}
	err := json.Unmarshal([]byte(raw), &p)
	return p, err
}
func expediteOrder(o dispatch.Order, p expedite.Plan) (dispatch.Order, string, error) {
	if p.Authority != "review" && p.Authority != "merge-on-green" {
		return o, "", fmt.Errorf("invalid authority")
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	tpl, err := skills.FS.ReadFile("hev-expedite/order.md")
	if err != nil {
		return o, "", err
	}
	skill, err := skills.FS.ReadFile("hev-expedite/SKILL.md")
	if err != nil {
		return o, "", err
	}
	body := strings.NewReplacer("{{authority}}", p.Authority, "{{plan}}", string(b), "{{version}}", version.Version, "{{skill}}", string(skill)).Replace(string(tpl))
	task, acceptance, _ := strings.Cut(body, "\n## Acceptance\n")
	var accepts []string
	for _, line := range strings.Split(acceptance, "\n") {
		if strings.HasPrefix(line, "- ") {
			accepts = append(accepts, strings.TrimPrefix(line, "- "))
		}
	}
	_, text, err := dispatch.Inline(o, task, accepts, map[string]bool{}, expediteNow())
	if err != nil {
		return o, "", err
	}
	text = strings.Replace(text, "---\n\n", "every: "+o.Every+"\nmax_lifetime: "+o.MaxLifetime+"\nuntil: all followed runs merged or abandoned\n---\n\n", 1)
	parsed, err := dispatch.Parse(text)
	return parsed, text, err
}
func newExpediteCommand() *cobra.Command {
	var host, path, authority, every, lifetime, repo, evidence string
	c := &cobra.Command{Use: "expedite", Short: "Opt in to repeating follow-through for this session's open runs", Args: cobra.NoArgs}
	c.PersistentFlags().StringVar(&path, "config", "", "Dispatch configuration")
	c.PersistentFlags().StringVar(&host, "host", "", "Headless host (otherwise configured default)")
	c.Flags().StringVar(&authority, "authority", "review", "review or merge-on-green")
	c.Flags().StringVar(&every, "every", "10m", "Round interval")
	c.Flags().StringVar(&lifetime, "max-lifetime", "24h", "Maximum lifetime")
	c.Flags().StringVar(&repo, "repo", "", "Repository for the expediter worktree (defaults to first followed run)")
	load := func() (dispatch.Config, error) { path = dispatch.ConfigPath(path); return dispatch.Load(path) }
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		cfg, err := load()
		if err != nil {
			return err
		}
		reply, err := expediteOwnAddress(cmd.Context())
		if err != nil {
			return err
		}
		inv := expediteInventory(cfg, "")
		// PR merge state is authoritative; dispatch completion alone is not merging.
		for hn, r := range inv {
			for i, row := range r.Rows {
				if row.Reply != reply || row.PR == "" {
					continue
				}
				b, e := exec.CommandContext(cmd.Context(), "gh", "pr", "view", row.PR, "--json", "state").Output()
				if e != nil {
					return fmt.Errorf("snapshot PR state %s: %w", row.PR, e)
				}
				var pr expedite.PR
				if e = json.Unmarshal(b, &pr); e != nil {
					return e
				}
				if pr.State == "MERGED" {
					r.Rows[i].State = "merged"
				}
			}
			inv[hn] = r
		}
		runs, err := expedite.Collect(inv, reply)
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
				if row.Reply == reply && row.Until == "all followed runs merged or abandoned" && row.State != "stopped" && row.State != "complete" && row.State != "handed-off" && row.Schedule.End.IsZero() {
					return fmt.Errorf("expediter %s already exists; inspect or stop it first", row.Name)
				}
			}
		}
		name := "expedite-" + expediteNow().UTC().Format("20060102-150405")
		if used[name] {
			return fmt.Errorf("expediter name already exists")
		}
		if repo == "" {
			repo = runs[0].Repo
		}
		o, text, err := expediteOrder(dispatch.Order{Name: name, Host: selected, Repo: repo, Reply: reply, Every: every, MaxLifetime: lifetime}, expedite.Plan{Name: name, Reply: reply, Authority: authority, Runs: runs})
		if err != nil {
			return err
		}
		return expediteStart(cmd, cfg, h, o, text, path)
	}
	for _, action := range []string{"status", "stop"} {
		action := action
		sub := &cobra.Command{Use: action + " [NAME]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := load()
			if err != nil {
				return err
			}
			reply := ""
			if len(args) == 0 {
				reply, err = expediteOwnAddress(cmd.Context())
				if err != nil {
					return err
				}
			}
			found := false
			inv := expediteInventory(cfg, host)
			for _, hn := range dispatch.SortedHosts(inv) {
				r := inv[hn]
				if r.Error != "" {
					return fmt.Errorf("%s: %s", hn, r.Error)
				}
				for _, row := range r.Rows {
					if row.Until != "all followed runs merged or abandoned" || (len(args) > 0 && row.Name != args[0] && row.Name != "o-"+args[0]) || (len(args) == 0 && row.Reply != reply) {
						continue
					}
					found = true
					h := cfg.Hosts[hn]
					shown, err := expediteRPC(cmd.Context(), h, dispatch.Request{Action: "show", Name: row.Name})
					if err != nil {
						return err
					}

					fmt.Fprintf(cmd.OutOrStdout(), "%s/%s %s rounds=%d last=%s next=%s reason=%s\n", hn, row.Name, row.State, row.Schedule.Rounds, roundTime(row.Schedule.Last), roundTime(row.Schedule.Next), row.Schedule.Reason)
					p, err := expeditePlan(shown.Files["order.md"])
					if err != nil {
						return err
					}
					for _, run := range p.Runs {
						fmt.Fprintf(cmd.OutOrStdout(), "  follows %s/%s\n", run.Host, run.Name)
					}
					if action == "stop" {
						if _, err = expediteRPC(cmd.Context(), h, dispatch.Request{Action: "stop", Name: row.Name}); err != nil {
							return err
						}
						// A remote stop request runs a handoff on the owning host, where board configuration lives.
					}
				}
			}
			if !found {
				return fmt.Errorf("no expediter found")
			}
			return nil
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
		p, err := expeditePlan(string(b))
		if err != nil {
			return err
		}
		ev := map[string]expedite.Evidence{}
		if evidence != "" {
			b, err = os.ReadFile(evidence)
			if err != nil {
				return err
			}
			if err = json.Unmarshal(b, &ev); err != nil {
				return err
			}
		}
		d := &expediteDriver{cfg: cfg, out: cmd.OutOrStdout(), reply: p.Reply}
		done, err := expedite.Round(cmd.Context(), p, ev, d)
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

type expediteDriver struct {
	cfg   dispatch.Config
	out   io.Writer
	reply string
}

func (d *expediteDriver) Inbox(ctx context.Context) error {
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
func (d *expediteDriver) Inspect(ctx context.Context, r expedite.Run) (dispatch.Row, expedite.PR, error) {
	var row dispatch.Row
	var pr expedite.PR
	h, ok := d.cfg.Hosts[r.Host]
	if !ok {
		return row, pr, fmt.Errorf("unknown host %s", r.Host)
	}
	inv, err := expediteRPC(ctx, h, dispatch.Request{Action: "list"})
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
func (d *expediteDriver) Amend(ctx context.Context, r expedite.Run, text string) error {
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
	_, err = expediteRPC(ctx, d.cfg.Hosts[r.Host], q)
	return err
}
func (d *expediteDriver) Merge(ctx context.Context, r expedite.Run, pr, head string) error {
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
	var fresh expedite.PR
	if err = json.Unmarshal(b, &fresh); err != nil {
		return err
	}
	if fresh.Head != head || fresh.State != "OPEN" || !fresh.Green() {
		return fmt.Errorf("PR is not green on the reviewed head after readiness transition")
	}
	return exec.CommandContext(ctx, "gh", "pr", "merge", pr, "--merge", "--match-head-commit", head).Run()
}
func (d *expediteDriver) Report(ctx context.Context, line string) error {
	fmt.Fprintln(d.out, line)
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
// the common runner, with the deliberately narrow expedite preset authority.
func expeditePreset(o dispatch.Order, text string) dispatch.OrchestratorSpec {
	s := dispatch.OrchestratorSpec{Kind: "orchestrator", Name: o.Name, Goal: "Follow through on this session's open runs", Host: o.Host, Mode: "once", Until: "goal", Every: o.Every, MaxLifetime: o.MaxLifetime, Harness: "codex", Reply: o.Reply, Agent: "on", Idle: "run", Authority: []string{"say", "rework"}}
	s.Manages.Runs.Selector = "mine"
	s.Brief = "Follow the snapshot below. Use only hev dispatch say or rework for interventions; report merge-ready results to the owner. Write checkpoint.md before exiting. When all followed runs are merged or explicitly abandoned, write goal-met.json containing {\"met\":true}.\n\n" + text
	return s
}
func startExpediteOrchestrator(cmd *cobra.Command, cfg dispatch.Config, h dispatch.Host, o dispatch.Order, text, path string) error {
	s := expeditePreset(o, text)
	b, e := yaml.Marshal(s)
	if e != nil {
		return e
	}
	_, e = expediteRPC(cmd.Context(), h, dispatch.Request{Action: "orchestrator-apply", Name: s.Name, Order: "---\n" + string(b) + "---\n" + s.Brief, Note: text})
	if e == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "started", s.Name)
	}
	return e
}
