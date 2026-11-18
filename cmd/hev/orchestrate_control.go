package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/steer"
	"github.com/spf13/cobra"
)

// orchestratorOwnerRow is an orchestrate ls row with its owner's liveness.
type orchestratorOwnerRow struct {
	dispatch.OrchestratorState
	Owner       string `json:"owner"`
	OwnerStatus string `json:"owner_status"`
}

// orchestrateLaunch starts a local supervisor; tests replace it.
var orchestrateLaunch = dispatch.LaunchOrchestrator

// orchestrateRPC reaches a remote host's orchestrators. Tests replace it with
// a fake remote.
var orchestrateRPC = dispatch.RPC

const handoffDeprecation = "handoff ends the orchestrator; to change who it replies to use adopt"

// orchestratorTarget is where an orchestrator's state lives.
type orchestratorTarget struct {
	local bool
	host  string
	h     dispatch.Host
}

// locateOrchestrator finds NAME: on --host when given; otherwise locally,
// then on every reachable configured dispatch host. A name on more than one
// remote host is an error naming them.
func locateOrchestrator(ctx context.Context, name, host string) (orchestratorTarget, error) {
	if !dispatch.ValidName(name) {
		return orchestratorTarget{}, fmt.Errorf("invalid orchestrator name")
	}
	if host != "" {
		cfg, err := dispatch.Load(dispatch.ConfigPath(""))
		if err != nil {
			return orchestratorTarget{}, err
		}
		h, ok := cfg.Hosts[host]
		if !ok {
			return orchestratorTarget{}, fmt.Errorf("unknown host %s", host)
		}
		if h.Local {
			return orchestratorTarget{local: true, host: host}, nil
		}
		return orchestratorTarget{host: host, h: h}, nil
	}
	_, localErr := dispatch.LoadOrchestrator(name)
	if localErr == nil && !scopeOnly(name) {
		return orchestratorTarget{local: true}, nil
	}
	cfg, err := dispatch.Load(dispatch.ConfigPath(""))
	if err != nil {
		if localErr != nil {
			return orchestratorTarget{}, localErr
		}
		return orchestratorTarget{}, err
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var found, unreachable []string
	for n, h := range cfg.Hosts {
		if h.Local {
			continue // Already looked.
		}
		wg.Add(1)
		go func(n string, h dispatch.Host) {
			defer wg.Done()
			c, cancel := context.WithTimeout(ctx, 12*time.Second)
			defer cancel()
			r, err := orchestrateRPC(c, h, dispatch.Request{Action: "list"})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				unreachable = append(unreachable, n)
				return
			}
			for _, row := range r.Rows {
				if row.Kind == "orch" && row.Name == "o-"+name {
					found = append(found, n)
					return
				}
			}
		}(n, h)
	}
	wg.Wait()
	sort.Strings(found)
	sort.Strings(unreachable)
	switch len(found) {
	case 1:
		return orchestratorTarget{host: found[0], h: cfg.Hosts[found[0]]}, nil
	case 0:
		if localErr == nil {
			return orchestratorTarget{local: true}, nil // An admission-only registration.
		}
		msg := fmt.Sprintf("orchestrator %s not found locally or on any reachable dispatch host", name)
		if len(unreachable) > 0 {
			msg += " (unreachable: " + strings.Join(unreachable, ", ") + ")"
		}
		return orchestratorTarget{}, fmt.Errorf("%s", msg)
	}
	return orchestratorTarget{}, fmt.Errorf("orchestrator %s is on more than one host (%s); select one with --host", name, strings.Join(found, ", "))
}

func scopeOnly(name string) bool {
	_, err := os.Stat(filepath.Join(dispatch.OrchestratorDir(name), "scope-owner.json"))
	return err == nil
}

func newOrchestrateControl(action string) *cobra.Command {
	var to, host string
	var yes bool
	var round int
	sub := &cobra.Command{Use: action + " NAME", Args: cobra.ExactArgs(1)}
	switch action {
	case "adopt":
		sub.Short = "Re-point a live orchestrator's owner without stopping it"
	case "retire":
		sub.Short = "End an orchestrator and hand its goal to another session (terminal)"
	case "handoff":
		sub.Short = "Deprecated: " + handoffDeprecation
	}
	sub.RunE = func(cmd *cobra.Command, args []string) error {
		name := args[0]
		out := cmd.OutOrStdout()
		switch action {
		case "handoff":
			fmt.Fprintln(cmd.ErrOrStderr(), handoffDeprecation+": hev orchestrate adopt "+name)
			if !yes {
				return fmt.Errorf("refusing handoff without --yes: %s (hev orchestrate adopt %s); to end it, use hev orchestrate retire %s --to %s --yes", handoffDeprecation, name, name, to)
			}
		case "retire":
			fmt.Fprintf(cmd.ErrOrStderr(), "retire ends orchestrator %s: it becomes handed-off (terminal) and its goal passes to %s. To change who it replies to and keep it running, use hev orchestrate adopt %s --to %s\n", name, to, name, to)
			if !yes {
				if !stdinIsTTY(cmd.InOrStdin()) {
					return fmt.Errorf("retire is terminal; pass --yes to confirm when stdin is not a terminal (to keep it running under a new owner, use adopt)")
				}
				fmt.Fprint(cmd.ErrOrStderr(), "End it? [y/N] ")
				answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
					return fmt.Errorf("retire cancelled")
				}
			}
		case "adopt":
			if to == "" {
				own, err := ownerOwnAddress(cmd.Context())
				if err != nil {
					return err
				}
				to = own
			}
		}
		t, err := locateOrchestrator(cmd.Context(), name, host)
		if err != nil {
			return err
		}
		verb := action
		if verb == "handoff" {
			verb = "retire"
		}
		if !t.local {
			return remoteOrchestrateControl(cmd, t, name, verb, to, round)
		}
		dir := dispatch.OrchestratorDir(name)
		switch verb {
		case "show":
			return showLocalOrchestrator(out, name, dir)
		case "logs":
			p := filepath.Join(dir, "supervisor.log")
			if round > 0 {
				p = filepath.Join(dir, "rounds", fmt.Sprintf("%03d", round), "trace")
			}
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			_, e = out.Write(b)
			return e
		case "adopt":
			res, e := dispatch.AdoptOrchestrator(name, to)
			if e != nil {
				return e
			}
			printAdopt(out, "orchestrator", res)
			return nil
		}
		if e := dispatch.OrchestratorControl(name, verb, to); e != nil {
			return e
		}
		if verb == "wake" || verb == "retire" {
			if e := orchestrateLaunch(name); e != nil {
				return e
			}
		}
		fmt.Fprintln(out, verb, name)
		return nil
	}
	sub.Flags().StringVar(&host, "host", "", "Dispatch host running the orchestrator (default: look locally, then on reachable hosts)")
	switch action {
	case "logs":
		sub.Flags().IntVar(&round, "round", 0, "Round trace number")
	case "adopt":
		sub.Flags().StringVar(&to, "to", "", "New owner HOST/SESSION (default: your own peers session)")
	case "retire", "handoff":
		sub.Flags().StringVar(&to, "to", "", "Session that takes over the goal")
		_ = sub.MarkFlagRequired("to")
		sub.Flags().BoolVar(&yes, "yes", false, "Confirm ending the orchestrator")
	}
	return sub
}

func printAdopt(w io.Writer, kind string, res dispatch.AdoptResult) {
	from := res.From
	if from == "" {
		from = "(none)"
	}
	fmt.Fprintf(w, "adopted %s %s: owner %s -> %s; %d undelivered notice(s) re-queued for %s\n", kind, res.Name, from, res.To, res.Redelivered, res.To)
	if res.Retried {
		if res.RetryError != "" {
			fmt.Fprintf(w, "retried the failed completion notice: failed again: %s\n", res.RetryError)
		} else {
			fmt.Fprintln(w, "retried the failed completion notice: delivered")
		}
	} else if res.RetryError != "" {
		fmt.Fprintln(w, "completion notice not retried: "+res.RetryError)
	}
}

func remoteOrchestrateControl(cmd *cobra.Command, t orchestratorTarget, name, verb, to string, round int) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
	defer cancel()
	r, err := orchestrateRPC(ctx, t.h, dispatch.Request{Action: verb, Name: "o-" + name, To: to, Round: round})
	if err != nil {
		return fmt.Errorf("%s on %s: %w", verb, t.host, err)
	}
	out := cmd.OutOrStdout()
	switch verb {
	case "show":
		fmt.Fprintf(out, "Host: %s\n", t.host)
		for _, f := range dispatch.OrchestratorShowFiles {
			fmt.Fprintf(out, "--- %s ---\n%s\n", f, r.Files[f])
		}
		fmt.Fprintln(out, "Rounds: "+strings.Join(r.Names, " "))
		if r.Message != "" {
			fmt.Fprintln(out, r.Message)
		}
		if s := r.Files["steer.json"]; s != "" && s != "[]" && s != "null" {
			fmt.Fprint(out, steerReceiptTable(s))
		}
	case "logs":
		fmt.Fprint(out, r.Log)
	case "adopt":
		if r.Adopt == nil {
			return fmt.Errorf("host %s did not confirm the adopt; upgrade hev there", t.host)
		}
		printAdopt(out, "orchestrator", *r.Adopt)
	default:
		fmt.Fprintln(out, verb, name, "on", t.host)
	}
	return nil
}

func showLocalOrchestrator(w io.Writer, name, dir string) error {
	for _, f := range dispatch.OrchestratorShowFiles {
		b, e := os.ReadFile(filepath.Join(dir, f))
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		fmt.Fprintf(w, "--- %s ---\n%s\n", f, b)
	}
	entries, e := os.ReadDir(filepath.Join(dir, "rounds"))
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	fmt.Fprint(w, "Rounds:")
	for _, f := range entries {
		fmt.Fprint(w, " "+f.Name())
	}
	fmt.Fprintln(w)
	if store := steer.OrchestratorStore(dispatch.Root(), name); store.Counts().Total() > 0 {
		fmt.Fprintln(w, store.Counts().Long())
		ms, _ := store.List()
		b, _ := json.Marshal(ms)
		fmt.Fprint(w, steerReceiptTable(string(b)))
	}
	var st dispatch.OrchestratorState
	if b, e := os.ReadFile(filepath.Join(dir, "state.json")); e == nil && json.Unmarshal(b, &st) == nil {
		if st.Deferred != "" {
			fmt.Fprintln(w, "Deferred: "+st.Deferred)
		}
		if sigs, e := dispatch.OrchestratorSignals(name); e == nil {
			st.Signals = sigs
		}
		if len(st.Signals) > 0 {
			fmt.Fprintln(w, "Intake signals:")
			for _, sig := range st.Signals {
				fmt.Fprintf(w, "  %s %s event=%s run=%s %s\n", sig.At.Format(time.RFC3339), sig.Kind, sig.Event, sig.Run, sig.Error)
			}
		}
	}
	return nil
}
