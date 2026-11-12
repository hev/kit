package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/nodes"
	"github.com/hev/kit/internal/peers"
	"github.com/hev/kit/internal/version"
	"github.com/spf13/cobra"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"
)

func init() {
	dispatch.CallerSession = callerTmuxSession
	rootCmd.AddCommand(newDispatchCommand())
	// Keep the release-facing validation entry point on the same implementation.
	d := newDispatchCommand()
	v, _, _ := d.Find([]string{"validate"})
	d.RemoveCommand(v)
	v.Short = "Validate a completed browser dispatch order"
	v.Flags().AddFlagSet(d.PersistentFlags())
	rootCmd.AddCommand(v)
}
func newDispatchCommand() *cobra.Command {
	var configPath, host string
	var asJSON, all, follow, allowSame, fromValidation bool
	var resolve, traceRef string
	d := &cobra.Command{Use: "dispatch ORDER.md", Short: "Send agent work to configured machines", Args: cobra.ExactArgs(1)}
	d.Flags().StringVar(&traceRef, "trace", "", "Continue an existing trace (trace ID or W3C traceparent) instead of starting a new one")
	d.PersistentFlags().StringVar(&configPath, "config", "", "Dispatch TOML configuration")
	d.PersistentFlags().StringVar(&host, "host", "", "Select host")
	d.PersistentFlags().BoolVar(&asJSON, "json", false, "Print JSON")
	load := func() (dispatch.Config, error) {
		configPath = dispatch.ConfigPath(configPath)
		c, e := dispatch.Load(configPath)
		if e == nil && host != "" {
			if _, ok := c.Hosts[host]; !ok {
				return c, fmt.Errorf("unknown host %s", host)
			}
		}
		return c, e
	}
	emit := func(v any) error { return json.NewEncoder(d.OutOrStdout()).Encode(v) }
	d.RunE = func(cmd *cobra.Command, args []string) error {
		c, e := load()
		if e != nil {
			return e
		}
		b, e := os.ReadFile(args[0])
		if e != nil {
			return e
		}
		// A trace: key is the sender's; the target never sees it.
		text, ref, e := dispatch.TakeTraceKey(string(b))
		if e != nil {
			return e
		}
		if traceRef != "" {
			ref = traceRef
		}
		if ref != "" {
			if _, _, e = dispatch.ParseTraceRef(ref); e != nil {
				return e
			}
		}
		o, e := dispatch.Parse(text)
		if e != nil {
			return e
		}
		if host != "" {
			o.Host = host
		}
		if o.Host == "" {
			o.Host, e = dispatch.SelectQueueHost(c, dispatch.Inventory(c, ""))
			if e != nil {
				return e
			}
		}
		h, ok := c.Hosts[o.Host]
		if !ok {
			return fmt.Errorf("unknown or missing host %s", o.Host)
		}
		return startDispatch(cmd, c, h, o, text, configPath, ref)
	}
	d.AddCommand(newDispatchRun(load, &configPath, &host))
	// notices reports an unknown --host itself, naming where joined hosts are listed.
	d.AddCommand(newDispatchNoticesCommand(func() (dispatch.Config, error) { return dispatch.Load(dispatch.ConfigPath(configPath)) }, &host))
	for _, action := range []string{"say", "attach", "ls", "hosts", "show", "logs", "stop", "amend", "rework", "retry", "clean", "validate"} {
		action := action
		c := &cobra.Command{Use: action, Args: cobra.NoArgs}
		if action != "ls" && action != "hosts" {
			c.Use += " NAME"
			c.Args = cobra.ExactArgs(1)
		}
		if action == "amend" {
			c.Use += " [ORDER.md | -]"
			c.Args = cobra.RangeArgs(1, 2)
		}
		if action == "rework" || action == "say" {
			c.Use += " NOTE..."
			c.Args = func(cmd *cobra.Command, args []string) error {
				if action == "say" && len(args) < 2 {
					return fmt.Errorf("say requires NAME and a message")
				}
				if len(args) == 0 || (len(args) == 1 && !fromValidation) {
					return fmt.Errorf("rework requires a note or --from-validation")
				}
				return nil
			}
		}
		c.RunE = func(cmd *cobra.Command, args []string) (runErr error) {
			defer func() {
				if runErr != nil && (action == "rework" || action == "say") {
					runErr = fmt.Errorf("Error: %w", runErr)
				}
			}()
			if action == "rework" || action == "say" {
				cmd.SilenceErrors, cmd.SilenceUsage = true, true
			}
			cfg, e := load()
			if e != nil {
				return e
			}
			if action == "validate" {
				ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
				defer cancel()
				r, err := dispatch.Validate(ctx, cfg, args[0], host, allowSame)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), r.Message)
				return nil
			}
			inv := dispatch.Inventory(cfg, host)
			if action == "ls" || action == "hosts" {
				if asJSON {
					return emit(inv)
				}
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
				defer w.Flush()
				if action == "hosts" {
					fmt.Fprintln(w, "HOST\tCAP\tRUNNING\tFREE\tREACHABLE\tVERSION")
				} else {
					fmt.Fprintln(w, "NAME\tHOST\tSENDER\tPOSITION\tKIND\tORCH\tSTATE\tDECISION\tVALIDATION\tAGE\tPR\tTRACE\tROUNDS\tLAST\tNEXT\tERROR")
				}
				for _, n := range dispatch.SortedHosts(inv) {
					r := inv[n]
					if r.Error != "" {
						fmt.Fprintf(w, "%s: unreachable: %s\n", n, r.Error)
						continue
					}
					if action == "hosts" {
						v := r.Version
						if v != version.Version {
							v += " (version mismatch)"
						}
						fmt.Fprintf(w, "%s\t%d\t%d\t%d\tyes\t%s\n", n, r.Cap, r.Running, r.Free, v)
					} else {
						for _, row := range r.Rows {
							if all || dispatch.VisibleByDefault(row) {
								if row.SameHost {
									row.Validation += " (same-host)"
								}
								position := "-"
								if row.Position > 0 {
									position = fmt.Sprint(row.Position)
								}
								fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n", row.Name, n, row.Sender, position, row.Kind, row.Orchestrator, row.State, rowDecision(row), row.Validation, time.Since(row.Start).Round(time.Second), row.PR, row.Trace, row.Schedule.Rounds, roundTime(row.Schedule.Last), roundTime(row.Schedule.Next), strings.SplitN(row.Error, "\n", 2)[0])
							}
						}
					}
				}
				return nil
			}
			name := args[0]
			if !dispatch.ValidAddress(name) {
				return fmt.Errorf("invalid name")
			}
			matches := []string{}
			for n, r := range inv {
				for _, row := range r.Rows {
					if row.Name == name {
						matches = append(matches, n)
					}
				}
			}
			if len(matches) != 1 {
				return fmt.Errorf("order not found or ambiguous; select --host; inventory: %v", inv)
			}
			h := cfg.Hosts[matches[0]]
			if action == "attach" {
				for hops := 0; ; hops++ {
					if hops == 16 {
						return fmt.Errorf("too many attach forwards")
					}
					r, err := dispatch.RPC(cmd.Context(), h, dispatch.Request{Action: "attach-route", Name: name})
					if err != nil {
						return err
					}
					if r.AttachHost == nil {
						break
					}
					h = *r.AttachHost
				}
				return dispatchAttach(cmd, h, name)
			}

			q := dispatch.Request{Action: action, Name: name, Cap: h.Cap}
			if action == "rework" || action == "retry" || action == "say" || action == "amend" {
				caller := dispatch.DetectCaller()
				q.Caller = &caller
			}
			if action == "amend" {
				path := "-"
				if len(args) > 1 {
					path = args[1]
				}
				var b []byte
				if path == "-" {
					b, e = io.ReadAll(cmd.InOrStdin())
				} else {
					b, e = os.ReadFile(path)
				}
				if e != nil {
					return e
				}
				// An amend keeps the run's trace; a trace: key is dropped.
				text, _, err := dispatch.TakeTraceKey(string(b))
				if err != nil {
					return err
				}
				b = []byte(text)
				o, err := dispatch.Parse(text)
				if err != nil {
					return err
				}
				if o.Name != name {
					return fmt.Errorf("amend cannot change name")
				}
				_, ok := cfg.Hosts[o.Host]
				if !ok {
					return fmt.Errorf("unknown host %s", o.Host)
				}
				q.Order = string(b)
			}
			if action == "rework" || action == "say" {
				q.Note, e = dispatchMessage(cmd, args[1:])
				if e != nil {
					return e
				}
				q.FromValidation = fromValidation
			}
			if action == "say" && resolve != "" {
				if e = checkOpenDecision(cmd.Context(), h, name, resolve); e != nil {
					return e
				}
				q.Resolve = resolve
				q.Meta.Sender = dispatch.LocalSender()
			}

			for {
				ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
				r, e := dispatch.RPC(ctx, h, q)
				cancel()
				if e != nil {
					return e
				}
				if asJSON {
					return emit(r)
				}
				if action == "show" {
					if r.History != nil {
						for _, rev := range r.History.Revisions {
							fmt.Fprintf(cmd.OutOrStdout(), "Revision %d (%s)\n%s\n", rev.Number, rev.Time.Format(time.RFC3339), rev.Diff)
						}
						for _, attempt := range r.History.Attempts {
							fmt.Fprintf(cmd.OutOrStdout(), "Attempt %d: revision %d, %s/%s %s, branch %s%s\n", attempt.Number, attempt.Revision, attempt.Meta.Host, attempt.Meta.Harness, attempt.Meta.Model, attempt.Meta.Branch, attemptContext(attempt.Meta))
						}
					}
					showContext(cmd.OutOrStdout(), r.Files["meta.json"])
					for _, f := range []string{"meta.json", "status.md", "report.md", "result.json", "inbox.md", "validation.json", "verdict.json", "schedule.json", "round-notes.md", "decisions.json"} {
						fmt.Fprintf(cmd.OutOrStdout(), "--- %s ---\n%s\n", f, r.Files[f])
					}
					fmt.Fprintln(cmd.OutOrStdout(), r.Message)
				} else if action == "logs" {
					fmt.Fprint(cmd.OutOrStdout(), r.Log)
					q.Offset = r.Offset
					if follow && r.Active {
						select {
						case <-cmd.Context().Done():
							return cmd.Context().Err()
						case <-time.After(time.Second):
							continue
						}
					}
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), r.Message)
				}
				return nil
			}
		}
		if action == "validate" {
			c.Flags().BoolVar(&allowSame, "allow-same-model", false, "Allow the implementer's harness and model")
		}
		if action == "rework" {
			c.Flags().BoolVar(&fromValidation, "from-validation", false, "Use latest failed browser verdict")
		}
		if action == "say" {
			c.Flags().StringVar(&resolve, "resolve", "", "Answer the run's open decision KEY and rework it")
		}
		if action == "ls" {
			c.Flags().BoolVar(&all, "all", false, "Include completed runs")
		}
		if action == "logs" {
			c.Flags().BoolVarP(&follow, "follow", "f", false, "Follow worker output")
		}
		d.AddCommand(c)
	}
	d.AddCommand(&cobra.Command{Use: "sweep", Short: "Start waiting FIFO orders in free slots", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return dispatch.SweepQueue() }})
	d.AddCommand(&cobra.Command{Use: "notify-validation NAME", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return dispatch.NotifyValidation(args[0]) }})
	d.AddCommand(&cobra.Command{Use: "supervise NAME", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return dispatch.Supervise(args[0]) }})
	var nodeID string
	rpcCmd := &cobra.Command{Use: "rpc", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		var grant nodes.Access
		if nodeID != "" {
			var e error
			grant, e = nodes.Default().Grant(nodeID, os.Getenv("SSH_ORIGINAL_COMMAND"))
			if e != nil {
				return e
			}
		}
		var q dispatch.Request
		if e := json.NewDecoder(cmd.InOrStdin()).Decode(&q); e != nil {
			return e
		}
		var r dispatch.Response
		var e error
		if nodeID != "" {
			// Restricted keys reach peers only through HandleGrant (peer-send to
			// their own runs); the generic peers RPC is for unrestricted callers.
			r, e = dispatch.HandleGrant(q, grant.Name, grant.Allow)
		} else if q.Action == "peers" {
			r = peers.HandleRPC(cmd.Context(), q.Note)
			r.Version = version.Version
			return emit(r)
		} else {
			r, e = dispatch.Handle(q)
		}
		r.Version = version.Version
		if e != nil {
			r.Error = e.Error()
		}
		return emit(r)
	}}
	rpcCmd.Flags().StringVar(&nodeID, "node", "", "Host-owned SSH grant ID")
	d.AddCommand(rpcCmd)
	return d
}

func dispatchMessage(cmd *cobra.Command, args []string) (string, error) {
	if len(args) == 1 && args[0] == "-" {
		b, err := io.ReadAll(cmd.InOrStdin())
		return string(b), err
	}
	return strings.Join(args, " "), nil
}

func startDispatch(cmd *cobra.Command, c dispatch.Config, h dispatch.Host, o dispatch.Order, text, configPath, traceRef string) error {
	text = effectiveOrderHost(text, o.Host)
	var e error
	q := dispatch.Request{Action: "start", Name: o.Name, Order: text, Meta: dispatch.Meta{Order: o, Namespace: h.Namespace, HarnessWrapper: h.HarnessWrapper}, Cap: h.Cap, Workspace: h.Workspace}
	if e = dispatch.StampTrace(&q.Meta, dispatch.TraceStart{Ref: traceRef, Orchestrator: dispatch.CallerOrchestrator(), Caller: dispatch.DetectCaller(), Text: dispatch.OrderAskText(text)}); e != nil {
		return e
	}
	q.Meta.ConfigPath = h.ConfigPath
	if h.Local && q.Meta.ConfigPath == "" {
		if configPath != "" {
			q.Meta.ConfigPath, e = filepath.Abs(configPath)
			if e != nil {
				return e
			}
		}
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
	defer cancel()
	r, e := dispatch.RPC(ctx, h, q)
	if e != nil {
		if strings.Contains(e.Error(), "host full") {
			inventory := dispatch.Inventory(c, "")
			for _, n := range dispatch.SortedHosts(inventory) {
				info := inventory[n]
				if info.Error != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: unreachable: %s\n", n, info.Error)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %d free\n", n, dispatch.AvailableSlots(c.Hosts[n], info))
				}
			}
		}
		cmd.SilenceErrors, cmd.SilenceUsage = true, true
		return fmt.Errorf("Error: %w", e)
	}
	fmt.Fprintln(cmd.OutOrStdout(), r.Message)
	if o.Validation == "browser" {
		fmt.Fprintln(cmd.OutOrStdout(), "Browser validation requested; inspect with hev dispatch show "+o.Name)
	}
	return nil
}

func dispatchAttach(cmd *cobra.Command, h dispatch.Host, name string) error {
	if attachUsesWindows(h, runtime.GOOS) {
		fmt.Fprintf(cmd.OutOrStdout(), "Log: ~/dispatch/%s/log\nUse hev dispatch logs %s -f\n", name, name)
		return nil
	}
	c := attachCommand(cmd.Context(), h, name, os.Getenv("TMUX") != "")
	c.Stdin, c.Stdout, c.Stderr = cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
	return c.Run()
}

func attachCommand(ctx context.Context, h dispatch.Host, name string, nested bool) *exec.Cmd {
	if !h.Local {
		return exec.CommandContext(ctx, "ssh", "-t", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", h.SSH, dispatch.RemoteCommand(h, "tmux attach -t "+dispatch.Quote("d-"+name)))
	}
	action := "attach"
	if nested {
		action = "switch-client"
	}
	return exec.CommandContext(ctx, "tmux", action, "-t", "d-"+name)
}

func attachUsesWindows(h dispatch.Host, clientOS string) bool {
	return h.OS == "windows" || (h.Local && clientOS == "windows")
}

func effectiveOrderHost(text, host string) string {
	// Persist the effective host, including --host, in the saved order.
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		found := false
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				if !found {
					lines = append(lines[:i], append([]string{"host: " + host}, lines[i:]...)...)
				}
				break
			}
			if strings.HasPrefix(lines[i], "host:") {
				lines[i] = "host: " + host
				found = true
			}
		}
		text = strings.Join(lines, "\n")
	}

	return text
}

// rowDecision shows what a finished run is waiting on.
func rowDecision(row dispatch.Row) string {
	switch row.Outcome {
	case dispatch.OutcomeNeedsDecision:
		if row.State == "needs-review" {
			return row.Key + ": " + row.Question
		}
	case dispatch.OutcomeBlocked:
		return "blocked: " + row.Reason
	}
	return "-"
}

// checkOpenDecision refuses a resolve before it is sent. A host running an
// older hev ignores the resolve field and would rework without recording the
// answer; it never reports an open decision, so the resolve stops here.
func checkOpenDecision(ctx context.Context, h dispatch.Host, name, key string) error {
	if !dispatch.ValidDecisionKey(key) {
		return fmt.Errorf("invalid decision key %q", key)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	r, err := dispatch.RPC(ctx, h, dispatch.Request{Action: "show", Name: name})
	if err != nil {
		return err
	}
	var result dispatch.Result
	_ = json.Unmarshal([]byte(r.Files["result.json"]), &result)
	if r.Active {
		return fmt.Errorf("resolve requires an ended run; %s is running", name)
	}
	if !dispatch.OpenDecision(&result) {
		var ledger []dispatch.Decision
		_ = json.Unmarshal([]byte(r.Files["decisions.json"]), &ledger)
		for _, d := range ledger {
			if d.Key == key && !d.Resolved.IsZero() {
				return fmt.Errorf("decision %s already resolved: %s", key, d.Answer)
			}
		}
		return fmt.Errorf("unknown decision key %s; %s has no open decision", key, name)
	}
	if result.Key != key {
		return fmt.Errorf("unknown decision key %s; open decision is %s", key, result.Key)
	}
	return nil
}

func roundTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

// attemptContext names the context digest an attempt was given.
func attemptContext(m dispatch.Meta) string {
	if m.ContextDigest == "" {
		return ""
	}
	return ", context " + shortDigest(m.ContextDigest)
}

func shortDigest(d string) string {
	if len(d) > 12 {
		return d[:12]
	}
	return d
}

// showContext prints the current attempt's context recipe and bundle digest.
// Records written before context recipes have neither and print nothing.
func showContext(w io.Writer, metaJSON string) {
	var m dispatch.Meta
	if json.Unmarshal([]byte(metaJSON), &m) != nil || (m.Context == "" && m.ContextBundle == nil) {
		return
	}
	c := m.ContextBundle
	if c == nil {
		fmt.Fprintf(w, "Context: recipe %s (no bundle built for this attempt)\n", m.Context)
		return
	}
	fmt.Fprintf(w, "Context: digest %s\n  recipe %s sha256 %s at %s (origin/%s)\n  bundle %s, %d sources, delivered on %s\n  review: hev context review --run %s\n",
		c.Digest, c.Recipe, c.RecipeSHA256, shortDigest(c.RecipeCommit), c.Base, c.Bundle, len(c.Sources), c.Delivery, m.Name)
	if len(c.HarnessAlsoLoads) > 0 {
		fmt.Fprintf(w, "  harness may also load on its own: %s\n", strings.Join(c.HarnessAlsoLoads, ", "))
	}
}

// callerTmuxSession names the tmux session of the pane hev runs in, on the
// caller's own tmux server, for the ask span's caller.
func callerTmuxSession() string {
	if os.Getenv("TMUX") == "" {
		return ""
	}
	args := []string{"display-message", "-p"}
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		args = append(args, "-t", pane)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", append(args, "#S")...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
