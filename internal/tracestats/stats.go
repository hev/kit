package tracestats

import (
	"math"
	"sort"
	"strings"

	"github.com/hev/kit/internal/trace"
)

// Schema names the JSON report's shape. It changes only when a field is
// renamed, removed or changes meaning; adding a field keeps it.
const Schema = "hev.traces.stats/v1"

// LaunchWindowMS is how soon after launch a failed or blocked run counts
// as a launch failure.
const LaunchWindowMS = 2 * 60 * 1000

// Group dimensions for --by.
var Dimensions = []string{"kind", "harness", "host", "repo", "order"}

// Facts are one session's tool facts: from its row when it was captured
// with them (capture_version >= 1), else derived from its block rows.
type Facts struct {
	ToolResults     int64
	ToolErrors      int64
	ToolsBeforeEdit int64
	Commands        map[string]int
	Skills          []string
	Via             map[string]string
}

// FactsFromRow reads a row's capture facts; ok is false for a row captured
// before them.
func FactsFromRow(r trace.SessionRow) (Facts, bool) {
	if r.CaptureVersion < 1 {
		return Facts{}, false
	}
	f := Facts{ToolResults: r.ToolResults, ToolErrors: r.ToolErrors, ToolsBeforeEdit: r.ToolsBeforeEdit, Commands: map[string]int(r.CommandCounts), Skills: r.Skills, Via: map[string]string{}}
	for _, l := range r.SkillLoads {
		f.Via[l.Name] = l.Via
	}
	return f, true
}

// FactsFromSession reads facts derived by trace.SessionFacts.
func FactsFromSession(s trace.SessionRow) Facts {
	f, _ := FactsFromRow(s)
	return f
}

// Dist is a distribution's median and 90th percentile (nearest rank).
type Dist struct {
	Median float64 `json:"median"`
	P90    float64 `json:"p90"`
}

// Group is one --by group's aggregates. A pointer field is null when no
// session in the group had what it needs.
type Group struct {
	Key      string `json:"key"`
	Sessions int    `json:"sessions"`
	// ToolCalls is the distribution of tool calls per session.
	ToolCalls Dist `json:"tool_calls"`
	// FactsSessions is how many sessions had tool facts (captured or
	// derived from blocks); the shares, edit and error figures are over
	// them.
	FactsSessions int `json:"facts_sessions"`
	// EditedSessions edited or committed at least once;
	// ToolsBeforeEditMedian is the median tool calls before the first edit
	// or commit among them.
	EditedSessions        int      `json:"edited_sessions"`
	ToolsBeforeEditMedian *float64 `json:"tools_before_edit_median"`
	// Shares of sessions with at least one such command.
	BoardSearchShare  *float64 `json:"board_search_share"`
	TracesSearchShare *float64 `json:"traces_search_share"`
	BoardPostShare    *float64 `json:"board_post_share"`
	DispatchShare     *float64 `json:"dispatch_share"`
	// ToolResults and ToolErrors are summed; ToolErrorRate is their ratio.
	ToolResults   int64    `json:"tool_results"`
	ToolErrors    int64    `json:"tool_errors"`
	ToolErrorRate *float64 `json:"tool_error_rate"`
	// StartContextSessions recorded a starting context;
	// StartContextBytesMedian is its median size.
	StartContextSessions    int      `json:"start_context_sessions"`
	StartContextBytesMedian *float64 `json:"start_context_bytes_median"`
	// Tokens in (input, cache read and cache creation) and out, summed.
	TokensIn  int64 `json:"tokens_in"`
	TokensOut int64 `json:"tokens_out"`
}

// Skill is one skill's loads: sessions that loaded it, by run kind,
// harness and how it was loaded (when the row recorded that).
type Skill struct {
	Name      string         `json:"name"`
	Sessions  int            `json:"sessions"`
	ByKind    map[string]int `json:"by_kind"`
	ByHarness map[string]int `json:"by_harness"`
	ByVia     map[string]int `json:"by_via,omitempty"`
}

// Installed is --installed: the skills installed on this host, and those
// no session in the window loaded.
type Installed struct {
	Skills      []string `json:"skills"`
	NeverLoaded []string `json:"never_loaded"`
}

// HostDispatch is one host's dispatch launch health.
type HostDispatch struct {
	Host                  string `json:"host"`
	Runs                  int    `json:"runs"`
	LaunchFailures        int    `json:"launch_failures"`
	ValidationRuns        int    `json:"validation_runs"`
	Attempts              int    `json:"attempts"`
	AttemptLaunchFailures int    `json:"attempt_launch_failures"`
}

// Dispatch is launch health over the runs started in the window. A run is
// one RunKey, however many attempts (reworks, relaunches) it had; it
// counts in the window its first attempt started in, on that attempt's
// host. Runs and LaunchFailures cover work runs; validation runs (runs
// dispatched to validate another) are counted apart. Attempts counts every
// attempt of those runs.
type Dispatch struct {
	Runs                  int            `json:"runs"`
	LaunchFailures        int            `json:"launch_failures"`
	ValidationRuns        int            `json:"validation_runs"`
	Attempts              int            `json:"attempts"`
	AttemptLaunchFailures int            `json:"attempt_launch_failures"`
	ByHost                []HostDispatch `json:"by_host"`
}

// Capture says how complete the window's capture facts are.
type Capture struct {
	Sessions          int `json:"sessions"`
	WithCaptureFacts  int `json:"with_capture_facts"`
	DerivedFromBlocks int `json:"derived_from_blocks"`
	WithStartContext  int `json:"with_start_context"`
}

// Report is hev traces stats --json.
type Report struct {
	Schema    string         `json:"schema"`
	Since     string         `json:"since"`
	From      int64          `json:"from"`
	To        int64          `json:"to"`
	By        string         `json:"by"`
	Sessions  int            `json:"sessions"`
	Kinds     map[string]int `json:"kinds"`
	Capture   Capture        `json:"capture"`
	Groups    []Group        `json:"groups"`
	Skills    []Skill        `json:"skills"`
	Installed *Installed     `json:"installed,omitempty"`
	Dispatch  Dispatch       `json:"dispatch"`
}

// Input is what Build aggregates.
type Input struct {
	Since    string
	From, To int64
	By       string
	Rows     []trace.SessionRow
	// Derived holds facts derived from block rows, by session ID, for rows
	// captured before capture facts.
	Derived map[string]Facts
	Spans   []trace.SpanRow
	// Installed, when non-nil, lists this host's installed skills.
	Installed []string
}

// Build aggregates rows into a report.
func Build(in Input) Report {
	rep := Report{Schema: Schema, Since: in.Since, From: in.From, To: in.To, By: in.By, Sessions: len(in.Rows), Kinds: map[string]int{}, Groups: []Group{}, Skills: []Skill{}}
	repeats := Repeats(in.Rows)
	type acc struct {
		g                             Group
		calls, beforeEdit, startBytes []float64
		board, traces, post, dispatch int
	}
	groups := map[string]*acc{}
	skills := map[string]*Skill{}
	for _, r := range in.Rows {
		kind := RunKind(r, repeats)
		rep.Kinds[kind]++
		key := groupKey(in.By, r, kind)
		a := groups[key]
		if a == nil {
			a = &acc{g: Group{Key: key}}
			groups[key] = a
		}
		a.g.Sessions++
		a.calls = append(a.calls, float64(r.ToolCount))
		a.g.TokensIn += r.InputTokens + r.CacheReadTokens + r.CacheCreationTokens
		a.g.TokensOut += r.OutputTokens

		f, ok := FactsFromRow(r)
		if ok {
			rep.Capture.WithCaptureFacts++
		} else if d, derived := in.Derived[r.SessionID]; derived {
			f, ok = d, true
			rep.Capture.DerivedFromBlocks++
		}
		if r.StartContext != nil && r.StartContext.Source != "" && r.StartContext.Source != "none" {
			rep.Capture.WithStartContext++
			a.g.StartContextSessions++
			a.startBytes = append(a.startBytes, float64(r.StartContextBytes))
		}
		names := r.Skills
		if ok {
			a.g.FactsSessions++
			names = f.Skills
			if f.ToolsBeforeEdit >= 0 {
				a.g.EditedSessions++
				a.beforeEdit = append(a.beforeEdit, float64(f.ToolsBeforeEdit))
			}
			a.g.ToolResults += f.ToolResults
			a.g.ToolErrors += f.ToolErrors
			if f.Commands["board_search"] > 0 {
				a.board++
			}
			if f.Commands["traces_search"] > 0 {
				a.traces++
			}
			if f.Commands["board_post"] > 0 {
				a.post++
			}
			if f.Commands["dispatch"] > 0 {
				a.dispatch++
			}
		}
		seen := map[string]bool{}
		for _, raw := range names {
			name := trace.NormalizeSkillName(raw)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			s := skills[name]
			if s == nil {
				s = &Skill{Name: name, ByKind: map[string]int{}, ByHarness: map[string]int{}, ByVia: map[string]int{}}
				skills[name] = s
			}
			s.Sessions++
			s.ByKind[kind]++
			s.ByHarness[r.Harness]++
			if via := f.Via[name]; via != "" {
				s.ByVia[via]++
			}
		}
	}
	rep.Capture.Sessions = len(in.Rows)
	for _, a := range groups {
		g := a.g
		g.ToolCalls = Dist{Median: percentile(a.calls, 50), P90: percentile(a.calls, 90)}
		if len(a.beforeEdit) > 0 {
			g.ToolsBeforeEditMedian = ptr(percentile(a.beforeEdit, 50))
		}
		if g.FactsSessions > 0 {
			n := float64(g.FactsSessions)
			g.BoardSearchShare = ptr(round(float64(a.board) / n))
			g.TracesSearchShare = ptr(round(float64(a.traces) / n))
			g.BoardPostShare = ptr(round(float64(a.post) / n))
			g.DispatchShare = ptr(round(float64(a.dispatch) / n))
		}
		if g.ToolResults > 0 {
			g.ToolErrorRate = ptr(round(float64(g.ToolErrors) / float64(g.ToolResults)))
		}
		if len(a.startBytes) > 0 {
			g.StartContextBytesMedian = ptr(percentile(a.startBytes, 50))
		}
		rep.Groups = append(rep.Groups, g)
	}
	sort.Slice(rep.Groups, func(i, j int) bool {
		if rep.Groups[i].Sessions != rep.Groups[j].Sessions {
			return rep.Groups[i].Sessions > rep.Groups[j].Sessions
		}
		return rep.Groups[i].Key < rep.Groups[j].Key
	})
	loaded := map[string]bool{}
	for _, s := range skills {
		if len(s.ByVia) == 0 {
			s.ByVia = nil
		}
		loaded[s.Name] = true
		rep.Skills = append(rep.Skills, *s)
	}
	sort.Slice(rep.Skills, func(i, j int) bool {
		if rep.Skills[i].Sessions != rep.Skills[j].Sessions {
			return rep.Skills[i].Sessions > rep.Skills[j].Sessions
		}
		return rep.Skills[i].Name < rep.Skills[j].Name
	})
	if in.Installed != nil {
		inst := &Installed{Skills: []string{}, NeverLoaded: []string{}}
		seen := map[string]bool{}
		for _, raw := range in.Installed {
			name := trace.NormalizeSkillName(raw)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			inst.Skills = append(inst.Skills, name)
			if !loaded[name] {
				inst.NeverLoaded = append(inst.NeverLoaded, name)
			}
		}
		sort.Strings(inst.Skills)
		sort.Strings(inst.NeverLoaded)
		rep.Installed = inst
	}
	rep.Dispatch = dispatchHealth(in.Spans, in.From, in.To)
	return rep
}

// IsLaunchFailure reports whether a dispatch attempt ended failed (a
// non-zero exit or an unconfirmed launch) or blocked within LaunchWindowMS
// of its launch.
func IsLaunchFailure(s trace.SpanRow) bool {
	if s.Kind != "attempt" || s.Result == 0 || s.End == 0 {
		return false
	}
	launched := s.RunStart
	if launched == 0 {
		launched = s.Start
	}
	if s.End-launched > LaunchWindowMS {
		return false
	}
	return s.ExitCode != 0 || s.Unconfirmed != 0 || s.Outcome == "blocked"
}

// RunKey is the run an attempt belongs to: its RunKey, else its run
// directory on its publisher, else (with neither) the attempt itself.
func RunKey(s trace.SpanRow) string {
	switch {
	case s.RunKey != "":
		return s.RunKey
	case s.Run != "":
		return s.Publisher + "/" + s.Run
	}
	return "span:" + s.ID
}

func dispatchHealth(spans []trace.SpanRow, from, to int64) Dispatch {
	d := Dispatch{ByHost: []HostDispatch{}}
	type run struct {
		first      trace.SpanRow
		attempts   int
		failures   int
		validation bool
	}
	runs := map[string]*run{}
	var order []string
	seen := map[string]bool{}
	for _, s := range spans {
		if s.Kind != "attempt" || seen[s.ID] {
			continue
		}
		seen[s.ID] = true // one attempt published by several hosts
		k := RunKey(s)
		r := runs[k]
		if r == nil {
			r = &run{first: s}
			runs[k] = r
			order = append(order, k)
		}
		if s.Start < r.first.Start || (s.Start == r.first.Start && s.Attempt < r.first.Attempt) {
			r.first = s
		}
		r.attempts++
		if IsLaunchFailure(s) {
			r.failures++
		}
		r.validation = r.validation || s.Source != ""
	}
	hosts := map[string]*HostDispatch{}
	for _, k := range order {
		r := runs[k]
		if r.first.Start < from || (to > 0 && r.first.Start > to) {
			continue
		}
		host := r.first.Host
		if host == "" {
			host = r.first.Publisher
		}
		h := hosts[host]
		if h == nil {
			h = &HostDispatch{Host: host}
			hosts[host] = h
		}
		h.Attempts += r.attempts
		h.AttemptLaunchFailures += r.failures
		d.Attempts += r.attempts
		d.AttemptLaunchFailures += r.failures
		if r.validation {
			h.ValidationRuns++
			d.ValidationRuns++
			continue
		}
		h.Runs++
		d.Runs++
		if r.failures > 0 {
			h.LaunchFailures++
			d.LaunchFailures++
		}
	}
	for _, h := range hosts {
		d.ByHost = append(d.ByHost, *h)
	}
	sort.Slice(d.ByHost, func(i, j int) bool {
		if d.ByHost[i].Runs != d.ByHost[j].Runs {
			return d.ByHost[i].Runs > d.ByHost[j].Runs
		}
		return d.ByHost[i].Host < d.ByHost[j].Host
	})
	return d
}

func groupKey(by string, r trace.SessionRow, kind string) string {
	key := ""
	switch by {
	case "harness":
		key = r.Harness
	case "host":
		key = r.Host
	case "repo":
		key = RepoName(r.RepoURL)
	case "order":
		key = r.Order
	default:
		key = kind
	}
	if key == "" {
		return "(none)"
	}
	return key
}

// RepoName shortens a remote URL to owner/name.
func RepoName(url string) string {
	url = strings.TrimSuffix(strings.TrimSpace(url), ".git")
	if url == "" {
		return ""
	}
	url = strings.ReplaceAll(url, ":", "/")
	parts := strings.Split(strings.Trim(url, "/"), "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return parts[len(parts)-1]
}

// percentile is the nearest-rank percentile of values (0 when empty).
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	rank := int(math.Ceil(p / 100 * float64(len(v))))
	if rank < 1 {
		rank = 1
	}
	return v[rank-1]
}

func round(f float64) float64 { return math.Round(f*1000) / 1000 }

func ptr(f float64) *float64 { return &f }
