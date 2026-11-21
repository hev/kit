package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
)

// newDispatchClose is hev dispatch close: a terminal disposition for an ended
// run whose work was finished another way or superseded. It never launches a
// worker and keeps the run's log and reports.
func newDispatchClose(load func() (dispatch.Config, error), host *string, asJSON *bool) *cobra.Command {
	var reason, supersededBy string
	c := &cobra.Command{
		Use:   "close [HOST/]NAME [REASON...]",
		Short: "Close an ended run whose work finished elsewhere or was superseded",
		Long: "Close records a terminal closed state on a run that is not running (blocked, needs-review, halted, failed, timeout, done or died).\n" +
			"An open decision is settled without a rework. Logs, reports and the worker's own reason are kept. Pro compacts eligible worktrees.\n" +
			"A later rework, retry or amend starts a fresh attempt and reopens the run.",
		Args: cobra.MinimumNArgs(1),
	}
	c.Flags().StringVar(&reason, "reason", "", "Why the run is closed (positional words after NAME are joined to it)")
	c.Flags().StringVar(&supersededBy, "superseded-by", "", "Replacement run, [HOST/]NAME")
	c.RunE = func(cmd *cobra.Command, args []string) error {
		cfg, err := load()
		if err != nil {
			return err
		}
		hostName, name, err := dispatch.SplitRunAddress(args[0])
		if err != nil {
			return err
		}
		if hostName != "" && *host != "" && hostName != *host {
			return fmt.Errorf("run address names host %s but --host is %s", hostName, *host)
		}
		if hostName == "" {
			hostName = *host
		}
		text := strings.TrimSpace(strings.Join(append([]string{reason}, args[1:]...), " "))
		if supersededBy != "" {
			if _, _, err = dispatch.SplitRunAddress(supersededBy); err != nil {
				return fmt.Errorf("--superseded-by: %w", err)
			}
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
		defer cancel()
		r, hostName, err := closeDispatchRun(ctx, cfg, hostName, name, text, supersededBy)
		if err != nil {
			return err
		}
		if *asJSON {
			var result dispatch.Result
			_ = json.Unmarshal([]byte(r.Files["result.json"]), &result)
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
				"host": hostName, "name": name, "state": dispatch.State(&result, false), "message": r.Message,
				"closed_at": result.ClosedAt, "closed_by": result.ClosedBy, "closed_reason": result.ClosedReason,
				"superseded_by": result.SupersededBy, "reason": result.Reason, "outcome": result.Outcome,
			})
		}
		fmt.Fprintln(cmd.OutOrStdout(), r.Message)
		return nil
	}
	return c
}

// closeDispatchRun sends a close for name to hostName, or to the one host
// whose inventory lists the run when hostName is empty. It returns the host
// it closed the run on.
func closeDispatchRun(ctx context.Context, cfg dispatch.Config, hostName, name, reason, supersededBy string) (dispatch.Response, string, error) {
	hostName, h, err := locateDispatchRun(cfg, hostName, name)
	if err != nil {
		return dispatch.Response{}, hostName, err
	}
	r, err := dispatch.RPC(ctx, h, dispatch.Request{Action: "close", Name: name, Note: reason, SupersededBy: supersededBy})
	if err != nil && strings.Contains(err.Error(), "unknown action") {
		err = fmt.Errorf("%w: host %s runs an hev without dispatch close; update it first", err, hostName)
	}
	return r, hostName, err
}

// locateDispatchRun resolves the host a run lives on: the named host, or the
// one host whose inventory lists the run.
func locateDispatchRun(cfg dispatch.Config, hostName, name string) (string, dispatch.Host, error) {
	if hostName != "" {
		h, ok := cfg.Hosts[hostName]
		if !ok {
			return hostName, h, fmt.Errorf("unknown host %s", hostName)
		}
		return hostName, h, nil
	}
	inv := dispatch.Inventory(cfg, "")
	var matches, unreachable []string
	for n, r := range inv {
		if r.Error != "" {
			unreachable = append(unreachable, n)
			continue
		}
		for _, row := range r.Rows {
			if row.Name == name && row.Kind == "run" {
				matches = append(matches, n)
				break
			}
		}
	}
	sort.Strings(matches)
	sort.Strings(unreachable)
	switch {
	case len(matches) == 1:
		return matches[0], cfg.Hosts[matches[0]], nil
	case len(matches) > 1:
		return "", dispatch.Host{}, fmt.Errorf("run %s is on several hosts (%s); name one as HOST/%s or with --host", name, strings.Join(matches, ", "), name)
	case len(unreachable) > 0:
		return "", dispatch.Host{}, fmt.Errorf("run %s not found on reachable hosts; unreachable: %s", name, strings.Join(unreachable, ", "))
	}
	return "", dispatch.Host{}, fmt.Errorf("run %s not found", name)
}

// supersededRun is what --supersedes learns about the run it replaces.
type supersededRun struct {
	Host, Name string
	// TraceRef continues the old run's trace: its trace ID with its current
	// attempt span as the parent, when both are known.
	TraceRef string
}

// lookupSuperseded finds the run --supersedes names and reads its trace. A
// lookup failure is a warning: the replacement dispatch still goes ahead.
func lookupSuperseded(ctx context.Context, cfg dispatch.Config, ref string, warn io.Writer) supersededRun {
	hostName, name, _ := dispatch.SplitRunAddress(ref)
	s := supersededRun{Host: hostName, Name: name}
	hostName, h, err := locateDispatchRun(cfg, hostName, name)
	if err != nil {
		fmt.Fprintf(warn, "Warning: --supersedes %s: %v; the new run will not inherit its trace\n", ref, err)
		return s
	}
	s.Host = hostName
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	r, err := dispatch.RPC(ctx, h, dispatch.Request{Action: "show", Name: name})
	if err != nil {
		fmt.Fprintf(warn, "Warning: --supersedes %s: %v; the new run will not inherit its trace\n", ref, err)
		return s
	}
	var m dispatch.Meta
	_ = json.Unmarshal([]byte(r.Files["meta.json"]), &m)
	s.TraceRef = supersededTraceRef(m)
	return s
}

func supersededTraceRef(m dispatch.Meta) string {
	if !dispatch.ValidTraceID(m.TraceID) {
		return ""
	}
	if dispatch.ValidSpanID(m.SpanID) {
		return "00-" + m.TraceID + "-" + m.SpanID + "-01"
	}
	return m.TraceID
}

// closeSuperseded closes the replaced run once its replacement is admitted.
// Failing to close it only warns: the new run is already on its way.
func closeSuperseded(ctx context.Context, cfg dispatch.Config, old supersededRun, newHost, newName string, out, warn io.Writer) {
	by := newHost + "/" + newName
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	r, hostName, err := closeDispatchRun(ctx, cfg, old.Host, old.Name, dispatch.SupersededReason(by), by)
	if err != nil {
		where := old.Name
		if hostName != "" {
			where = hostName + "/" + old.Name
		}
		fmt.Fprintf(warn, "Warning: %s was admitted but closing superseded run %s failed: %v; close it later with: hev dispatch close %s --superseded-by %s\n", by, where, err, where, by)
		return
	}
	fmt.Fprintln(out, r.Message)
}
