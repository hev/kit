package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	tracepkg "github.com/hev/kit/internal/trace"
	"github.com/hev/kit/internal/tracestats"
	"github.com/spf13/cobra"
)

type statsOptions struct {
	since, until, by, namespace string
	json, installed             bool
	scanBlocks                  bool
	top                         int
}

// statsRuntime is what the command reads, injectable for tests.
type statsRuntime struct {
	Store func(namespace string) (traceStore, error)
	Spans func(store traceStore, from int64) ([]tracepkg.SpanRow, error)
	// RunHistory returns the attempts, whenever they started, of the runs
	// the given in-window attempts belong to (it may return more; the
	// caller keeps those whose tracestats.RunKey matches), so a run first
	// launched before the window is not mistaken for a new one when only
	// its rework falls inside. Nil falls back to Spans from time zero.
	RunHistory func(store traceStore, inWindow []tracepkg.SpanRow) ([]tracepkg.SpanRow, error)
	Installed  func() []string
	Now        func() time.Time
}

func defaultStatsRuntime() statsRuntime {
	return statsRuntime{
		Store: traces,
		Spans: func(store traceStore, from int64) ([]tracepkg.SpanRow, error) {
			if lc := layerOf(store); lc != nil {
				return lc.ListTraceSpanRows([]any{"start", "Gte", from})
			}
			// An archive in files is this host's: read its run records.
			w, err := currentWorkspace()
			if err != nil {
				return nil, err
			}
			return dispatch.TraceSpanRows(dispatch.WorkspaceRunRecords(dispatch.Root(), w.Name), layer.Hostname(), nil), nil
		},
		RunHistory: func(store traceStore, inWindow []tracepkg.SpanRow) ([]tracepkg.SpanRow, error) {
			if lc := layerOf(store); lc != nil {
				return layerRunHistory(lc, inWindow)
			}
			// The files path already returned every run record.
			return nil, nil
		},
		Installed: func() []string {
			home, _ := os.UserHomeDir()
			var names []string
			for _, s := range index.InstalledSkills(index.HarnessSkillRoots(home)) {
				names = append(names, s.Name)
			}
			return names
		},
		Now: time.Now,
	}
}

const tracesStatsLong = `Aggregate captured sessions: run kinds, skill loads, tool use, starting
context and dispatch launch health, for a daily review or a one-off look.

  hev traces stats                      last 5 days, grouped by run kind
  hev traces stats --by harness         group by harness (kind, harness, host, repo, order)
  hev traces stats --installed          also list installed skills no session loaded
  hev traces stats --json               the stable report (schema hev.traces.stats/v1)

Run kind is derived: worker (a dispatch run), orchestrator-round, loop (a
scheduled or self-rescheduling task, or a first prompt that recurs) or
interactive. Rows captured before the stats capture facts get their tool
facts from their block rows (--scan-blocks, on by default). See
https://hevkit.com/docs/traces for every field.`

func newTracesStatsCmd(rt statsRuntime) *cobra.Command {
	opt := statsOptions{}
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Aggregate sessions: run kinds, skills, tool use, dispatch health",
		Long:  tracesStatsLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rep, err := buildTracesStats(rt, opt, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			if opt.json {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rep)
			}
			renderTracesStats(cmd.OutOrStdout(), rep, opt.top)
			return nil
		},
	}
	cmd.Flags().StringVar(&opt.since, "since", "5d", "Sessions started within this window (e.g. 1d, 5d, 2w)")
	cmd.Flags().StringVar(&opt.until, "until", "", "End the window here instead of now (RFC3339 or YYYY-MM-DD), for a fixed window")
	cmd.Flags().StringVar(&opt.by, "by", "kind", "Group by kind, harness, host, repo or order")
	cmd.Flags().BoolVar(&opt.json, "json", false, "Print the report as JSON (schema hev.traces.stats/v1)")
	cmd.Flags().BoolVar(&opt.installed, "installed", false, "List the skills installed on this host that no session in the window loaded")
	cmd.Flags().BoolVar(&opt.scanBlocks, "scan-blocks", true, "Derive tool facts from block rows for sessions captured before them")
	cmd.Flags().IntVar(&opt.top, "top", 15, "Skill rows to print (0 for all; --json always has all)")
	cmd.Flags().StringVar(&opt.namespace, "namespace", "", "Layer namespace (default $HEVLAYER_NAMESPACE or hev-traces)")
	return cmd
}

func buildTracesStats(rt statsRuntime, opt statsOptions, errOut io.Writer) (tracestats.Report, error) {
	valid := false
	for _, d := range tracestats.Dimensions {
		valid = valid || d == opt.by
	}
	if !valid {
		return tracestats.Report{}, fmt.Errorf("--by must be one of %s", strings.Join(tracestats.Dimensions, ", "))
	}
	window, err := parseSinceDuration(opt.since)
	if err != nil {
		return tracestats.Report{}, err
	}
	store, err := rt.Store(opt.namespace)
	if err != nil {
		return tracestats.Report{}, err
	}
	now := rt.Now()
	if opt.until != "" {
		if now, err = parseUntil(opt.until); err != nil {
			return tracestats.Report{}, err
		}
	}
	from := now.Add(-window).UnixMilli()
	rows, err := store.ListSessionRows(-1, []any{"And", []any{[]any{"start", "Gte", from}, []any{"start", "Lte", now.UnixMilli()}}})
	if err != nil {
		return tracestats.Report{}, err
	}
	in := tracestats.Input{Since: opt.since, From: from, To: now.UnixMilli(), By: opt.by, Rows: rows}
	if opt.scanBlocks {
		in.Derived = deriveFacts(store, rows, errOut)
	}
	if rt.Spans != nil {
		spans, err := rt.Spans(store, from)
		if err != nil {
			fmt.Fprintf(errOut, "warning: dispatch spans unavailable: %v\n", err)
		} else if spans, err = withRunHistory(rt, store, spans, from); err != nil {
			fmt.Fprintf(errOut, "warning: dispatch run history unavailable, runs first launched before the window may count as new: %v\n", err)
		}
		in.Spans = spans
	}
	if opt.installed {
		in.Installed = []string{}
		if rt.Installed != nil {
			in.Installed = append(in.Installed, rt.Installed()...)
		}
	}
	return tracestats.Build(in), nil
}

// withRunHistory adds the earlier attempts of every run that has an attempt
// in the window. The span query filters on attempt start, so without them a
// run launched before the window and reworked inside it would look new.
func withRunHistory(rt statsRuntime, store traceStore, spans []tracepkg.SpanRow, from int64) ([]tracepkg.SpanRow, error) {
	want := map[string]bool{}
	var inWindow []tracepkg.SpanRow
	for _, s := range spans {
		if s.Kind == "attempt" && s.Start >= from {
			want[tracestats.RunKey(s)] = true
			inWindow = append(inWindow, s)
		}
	}
	if len(inWindow) == 0 {
		return spans, nil
	}
	var history []tracepkg.SpanRow
	var err error
	if rt.RunHistory != nil {
		history, err = rt.RunHistory(store, inWindow)
	} else {
		history, err = rt.Spans(store, 0)
	}
	if err != nil {
		return spans, err
	}
	out := append([]tracepkg.SpanRow(nil), spans...)
	for _, s := range history {
		if s.Kind == "attempt" && want[tracestats.RunKey(s)] {
			out = append(out, s)
		}
	}
	return out, nil
}

// layerRunHistory reads every attempt of the given runs from the hosted span
// namespace. Of the span attributes only trace_id, run and start are
// filterable, so it queries by run directory name (every attempt carries
// one) and keeps the rows whose run key is wanted; a name reused on another
// host or later is dropped there.
func layerRunHistory(lc *layer.Client, inWindow []tracepkg.SpanRow) ([]tracepkg.SpanRow, error) {
	seen := map[string]bool{}
	var names []string
	for _, s := range inWindow {
		if s.Run != "" && !seen[s.Run] {
			seen[s.Run] = true
			names = append(names, s.Run)
		}
	}
	var out []tracepkg.SpanRow
	for i := 0; i < len(names); i += 200 {
		rows, err := lc.ListTraceSpanRows([]any{"run", "In", names[i:min(i+200, len(names))]})
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

// deriveFacts reads the block rows of each session captured before the
// capture facts, eight at a time, and derives its facts. A session whose
// blocks cannot be read keeps no facts.
func deriveFacts(store traceStore, rows []tracepkg.SessionRow, errOut io.Writer) map[string]tracestats.Facts {
	out := map[string]tracestats.Facts{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	failed := 0
	for _, r := range rows {
		if r.CaptureVersion >= 1 {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(r tracepkg.SessionRow) {
			defer wg.Done()
			defer func() { <-sem }()
			blocks, err := store.ListBlockRows(r.SessionID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil || len(blocks) == 0 {
				failed++
				return
			}
			turns := tracepkg.TurnsFromBlocks(blocks, tracepkg.Turn{SessionID: r.SessionID, Harness: r.Harness, Workdir: r.Workdir})
			out[r.SessionID] = tracestats.FactsFromSession(tracepkg.SessionFacts(turns))
		}(r)
	}
	wg.Wait()
	if failed > 0 {
		fmt.Fprintf(errOut, "note: %d session(s) had no readable block rows; their tool facts are left out\n", failed)
	}
	return out
}

func renderTracesStats(w io.Writer, rep tracestats.Report, top int) {
	fmt.Fprintf(w, "%d sessions since %s", rep.Sessions, time.UnixMilli(rep.From).Local().Format("Jan 02 15:04"))
	var kinds []string
	for _, k := range tracestats.Kinds {
		if rep.Kinds[k] > 0 {
			kinds = append(kinds, fmt.Sprintf("%s %d", k, rep.Kinds[k]))
		}
	}
	if len(kinds) > 0 {
		fmt.Fprintf(w, " · %s", strings.Join(kinds, ", "))
	}
	fmt.Fprintln(w)
	c := rep.Capture
	fmt.Fprintf(w, "tool facts: %d captured, %d from block rows, %d missing · starting context on %d\n\n",
		c.WithCaptureFacts, c.DerivedFromBlocks, c.Sessions-c.WithCaptureFacts-c.DerivedFromBlocks, c.WithStartContext)

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\tSESSIONS\tTOOLS p50/p90\tBEFORE EDIT p50\tBOARD SEARCH\tTRACES SEARCH\tBOARD POST\tDISPATCH\tUNKNOWN CMD\tTOOL ERR\tCONTEXT p50\tTOKENS IN→OUT\n", strings.ToUpper(rep.By))
	for _, g := range rep.Groups {
		key := g.Key
		if rep.By == "harness" {
			key = formatHarness(key)
		}
		fmt.Fprintf(tw, "%s\t%d\t%s/%s\t%s\t%s\t%s\t%s\t%s\t%d/%d\t%s\t%s\t%s\n", key, g.Sessions,
			num(g.ToolCalls.Median), num(g.ToolCalls.P90), optNum(g.ToolsBeforeEditMedian),
			share(g.BoardSearchShare), share(g.TracesSearchShare), share(g.BoardPostShare), share(g.DispatchShare),
			g.UnknownCommandSessions, g.UnknownCommands,
			share(g.ToolErrorRate), optBytes(g.StartContextBytesMedian), formatTokenPair(g.TokensIn, g.TokensOut))
	}
	tw.Flush()

	fmt.Fprintln(w)
	tw = tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "SKILL\tSESSIONS")
	for _, k := range tracestats.Kinds {
		fmt.Fprintf(tw, "\t%s", strings.ToUpper(k))
	}
	fmt.Fprintf(tw, "\tHARNESS\n")
	for i, s := range rep.Skills {
		if top > 0 && i >= top {
			fmt.Fprintf(tw, "… %d more (--top 0 or --json for all)\n", len(rep.Skills)-top)
			break
		}
		fmt.Fprintf(tw, "%s\t%d", s.Name, s.Sessions)
		for _, k := range tracestats.Kinds {
			fmt.Fprintf(tw, "\t%d", s.ByKind[k])
		}
		var hs []string
		for h, n := range s.ByHarness {
			hs = append(hs, fmt.Sprintf("%s %d", formatHarness(h), n))
		}
		sort.Strings(hs)
		fmt.Fprintf(tw, "\t%s\n", strings.Join(hs, ", "))
	}
	tw.Flush()

	if rep.Installed != nil {
		fmt.Fprintf(w, "\ninstalled here: %d skills; never loaded in the window: %d\n", len(rep.Installed.Skills), len(rep.Installed.NeverLoaded))
		if len(rep.Installed.NeverLoaded) > 0 {
			fmt.Fprintf(w, "  %s\n", strings.Join(rep.Installed.NeverLoaded, ", "))
		}
	}

	d := rep.Dispatch
	fmt.Fprintf(w, "\ndispatch: %d runs started, %d launch failures (failed or blocked within 2 min); %d validation runs; %d attempts, %d failed at launch\n",
		d.Runs, d.LaunchFailures, d.ValidationRuns, d.Attempts, d.AttemptLaunchFailures)
	for _, h := range d.ByHost {
		fmt.Fprintf(w, "  %-24s %4d runs  %3d launch failures  %3d validation runs  %4d attempts\n", h.Host, h.Runs, h.LaunchFailures, h.ValidationRuns, h.Attempts)
	}
}

// parseUntil reads --until: an RFC3339 time, or a date meaning its end.
func parseUntil(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if d, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return d.AddDate(0, 0, 1), nil
	}
	return time.Time{}, fmt.Errorf("invalid --until %q: use RFC3339 (2026-11-16T23:40:00Z) or YYYY-MM-DD", s)
}

func num(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%.1f", f)
}

func optNum(f *float64) string {
	if f == nil {
		return "–"
	}
	return num(*f)
}

func share(f *float64) string {
	if f == nil {
		return "–"
	}
	return fmt.Sprintf("%.0f%%", *f*100)
}

func optBytes(f *float64) string {
	if f == nil {
		return "–"
	}
	return humanBytes(fmt.Sprintf("%d", int64(*f)))
}
