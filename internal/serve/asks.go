package serve

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/trace"
)

// Ask-rooted traces, derived at read time. The dispatch run records on this
// host supply asks, orders, attempts and validation runs; Layer session rows
// supply worker sessions, joined by trace_id and by the session IDs the run
// results recorded. Nothing here is stored: there is no spans namespace.

// WithDispatchRoot reads run records from root instead of ~/dispatch.
func (s *Server) WithDispatchRoot(root string) *Server { s.dispatchRoot = root; return s }

// runRecords is this host's run records plus those other hosts published
// to the archive's spans namespace that match scope (a spans filter; nil
// for all). A run this host holds itself is read from disk, not the archive.
func (s *Server) runRecords(scope any) []dispatch.RunRecord {
	root := s.dispatchRoot
	if root == "" {
		root = dispatch.Root()
	}
	var out []dispatch.RunRecord
	local := map[string]bool{}
	for _, r := range dispatch.ReadRunRecords(root) {
		// A run moved to another host is read there, not from its stub here.
		if r.Meta.Forward == nil {
			out = append(out, r)
			local[dispatch.RunKey(r)] = true
		}
	}
	for _, r := range s.archiveRecords(scope) {
		if !local[dispatch.RunKey(r)] {
			out = append(out, r)
		}
	}
	return out
}

// noArchive scopes a read to this host's records only.
type noArchive struct{}

type spanLister interface {
	ListTraceSpanRows(filter any) ([]trace.SpanRow, error)
}

// archiveRecords reads published trace facts matching scope, widens them to
// whole traces (and whole untraced runs), and rebuilds their run records.
// Without an archive, or when it cannot be read, there are none: the view
// falls back to this host's own records.
func (s *Server) archiveRecords(scope any) []dispatch.RunRecord {
	l, ok := s.reader.(spanLister)
	if _, none := scope.(noArchive); !ok || none {
		return nil
	}
	rows, err := l.ListTraceSpanRows(scope)
	if err != nil || len(rows) == 0 {
		return nil
	}
	traces, runs := map[string]bool{}, map[string]bool{}
	for _, row := range rows {
		if row.TraceID != "" {
			traces[row.TraceID] = true
		} else {
			runs[row.Run] = true
		}
	}
	list := func(set map[string]bool) []any {
		keys := make([]string, 0, len(set))
		for k := range set {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]any, len(keys))
		for i, k := range keys {
			out[i] = k
		}
		return out
	}
	var or []any
	if len(traces) > 0 {
		or = append(or, []any{"trace_id", "In", list(traces)})
	}
	if len(runs) > 0 {
		or = append(or, []any{"run", "In", list(runs)})
	}
	filter := or[0]
	if len(or) > 1 {
		filter = []any{"Or", or}
	}
	if all, err := l.ListTraceSpanRows(filter); err == nil {
		rows = all
	}
	return dispatch.RecordsFromSpanRows(rows)
}

// traceScope is the spans filter for one trace page.
func traceScope(id string) any {
	if name, ok := strings.CutPrefix(id, "run:"); ok {
		return []any{"run", "Eq", name}
	}
	return []any{"trace_id", "Eq", id}
}

// Span is one row of the waterfall.
type Span struct {
	ID     string `json:"id"`
	Parent string `json:"parent,omitempty"`
	// Kind is ask, order, run, validation or session.
	Kind string `json:"kind"`
	Name string `json:"name"`
	Text string `json:"text,omitempty"`
	// Action is what an ask did (dispatch, continue, rework, amend, retry,
	// say, validate); for a run, the action of the ask that launched it.
	Action     string           `json:"action,omitempty"`
	LaunchedBy string           `json:"launched_by,omitempty"`
	Order      string           `json:"order,omitempty"`
	Attempt    int              `json:"attempt,omitempty"`
	Start      int64            `json:"start"`
	End        int64            `json:"end,omitempty"`
	State      string           `json:"state,omitempty"`
	Host       string           `json:"host,omitempty"`
	Harness    string           `json:"harness,omitempty"`
	Model      string           `json:"model,omitempty"`
	SessionID  string           `json:"session_id,omitempty"`
	Summary    string           `json:"summary,omitempty"`
	Indexed    bool             `json:"indexed,omitempty"`
	Cost       float64          `json:"cost,omitempty"`
	Tokens     int64            `json:"tokens,omitempty"`
	PR         string           `json:"pr,omitempty"`
	Caller     *dispatch.Caller `json:"caller,omitempty"`
	// Detail panel fields. Order: its intent (the body before Acceptance)
	// and Acceptance items from the latest revision. Run and validation:
	// when the attempt was queued, its branch and head, the worker's
	// outcome and report, and for validation the verdict.
	Intent     string            `json:"intent,omitempty"`
	Acceptance []string          `json:"acceptance,omitempty"`
	Revisions  int               `json:"revisions,omitempty"`
	Repo       string            `json:"repo,omitempty"`
	Queued     int64             `json:"queued,omitempty"`
	Branch     string            `json:"branch,omitempty"`
	Head       string            `json:"head,omitempty"`
	Outcome    string            `json:"outcome,omitempty"`
	Reason     string            `json:"reason,omitempty"`
	Error      string            `json:"error,omitempty"`
	ExitCode   int               `json:"exit_code,omitempty"`
	Report     string            `json:"report,omitempty"`
	Verdict    *dispatch.Verdict `json:"verdict,omitempty"`
	Evidence   string            `json:"evidence,omitempty"`
	// Remote names the host that published this order or attempt to the
	// archive. Its order body, Acceptance and reports stay on that host.
	Remote string `json:"remote,omitempty"`
	// ContextDigest is the context bundle a run attempt was given, and
	// ContextRecipe the recipe it was built from (see hev context).
	ContextDigest string `json:"context_digest,omitempty"`
	ContextRecipe string `json:"context_recipe,omitempty"`
}

// TraceSummary is one row of the asks list.
type TraceSummary struct {
	ID string `json:"id"`
	// Untraced marks a run (and its validators) with no trace of its own.
	// PreTrace narrows it: the record holds no trace fields and no reason,
	// so it was written before tracing (or by a hev that did not trace),
	// not left untraced on purpose. Reason says why a deliberately untraced
	// run has no trace.
	Untraced bool             `json:"untraced,omitempty"`
	PreTrace bool             `json:"pretrace,omitempty"`
	Reason   string           `json:"reason,omitempty"`
	Ask      string           `json:"ask"`
	RootHere bool             `json:"root_here"`
	Caller   *dispatch.Caller `json:"caller,omitempty"`
	Start    int64            `json:"start"`
	End      int64            `json:"end"`
	Duration int64            `json:"duration_ms"`
	// User is who asked: the requester recorded on the run the trace
	// started with (requested_by, else sender, such as "local:hev"). Users
	// lists everyone recorded across the trace's runs, User first.
	User  string   `json:"user,omitempty"`
	Users []string `json:"users"`
	// AskHosts are the hosts the asks were made on (from each ask's
	// caller), the first ask's host first. Hosts are where the work ran.
	AskHosts []string `json:"ask_hosts"`
	Hosts    []string `json:"hosts"`
	// Projects are the repositories of the trace's orders, by name.
	Projects []string `json:"projects"`
	Orders   []string `json:"orders"`
	Asks     int      `json:"asks"`
	Runs     int      `json:"runs"`
	Sessions int      `json:"sessions"`
	State    string   `json:"state"`
	Cost     float64  `json:"cost"`
}

type traceModel struct {
	TraceSummary
	runs  []dispatch.RunRecord
	spans []Span
}

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// derivedSpanID is a versioned, deterministic span ID for spans the records
// do not name themselves (orders, untraced attempts, sessions).
func derivedSpanID(parts ...string) string {
	sum := sha256.Sum256([]byte("kit-span/v1/" + strings.Join(parts, "/")))
	return hex.EncodeToString(sum[:8])
}

func traceKey(m dispatch.Meta, name string) string {
	if dispatch.ValidTraceID(m.TraceID) {
		return m.TraceID
	}
	return "run:" + name
}

// runState reads an attempt's state without probing live processes: an
// attempt with no end yet is running.
func runState(r *dispatch.Result) string {
	if r != nil && r.End.IsZero() && !r.LaunchUnconfirmed {
		return "running"
	}
	return dispatch.State(r, false)
}

var stateRank = map[string]int{"done": 0, "pass": 0, "running": 5, "queued": 4, "launch-unconfirmed": 3, "blocked": 6, "needs-review": 7, "timeout": 8, "failed": 8, "fail": 8, "died": 8}

// buildTraces groups run records into traces and joins sessions to them.
func buildTraces(records []dispatch.RunRecord, sessions map[string]trace.SessionRow) []*traceModel {
	byKey := map[string]*traceModel{}
	var order []string
	for _, r := range records {
		key := traceKey(r.Meta, r.Name)
		// An untraced validator belongs with its untraced source run.
		if r.Meta.Parent != "" && !dispatch.ValidTraceID(r.Meta.TraceID) {
			key = "run:" + r.Meta.Parent
		}
		t := byKey[key]
		if t == nil {
			t = &traceModel{TraceSummary: TraceSummary{ID: key, Untraced: strings.HasPrefix(key, "run:")}}
			byKey[key] = t
			order = append(order, key)
		}
		t.runs = append(t.runs, r)
	}
	out := make([]*traceModel, 0, len(order))
	for _, key := range order {
		t := byKey[key]
		t.derive(sessions)
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start > out[j].Start })
	return out
}

func (t *traceModel) derive(sessions map[string]trace.SessionRow) {
	asks := map[string]dispatch.AskSpan{}
	for _, r := range t.runs {
		for _, a := range r.Meta.Asks {
			asks[a.SpanID] = a
		}
		for _, at := range r.History.Attempts {
			for _, a := range at.Meta.Asks {
				if _, ok := asks[a.SpanID]; !ok {
					asks[a.SpanID] = a
				}
			}
		}
		if t.Untraced && r.Meta.Untraced != "" {
			t.Reason = r.Meta.Untraced
		}
	}
	var root *dispatch.AskSpan
	for _, a := range asks {
		c := a.Caller
		t.spans = append(t.spans, Span{ID: a.SpanID, Parent: a.ParentSpanID, Kind: "ask", Name: a.Action, Text: a.Text, Action: a.Action, Start: ms(a.Time), Caller: &c})
		if a.Kind == dispatch.AskIntake && (root == nil || a.Time.Before(root.Time)) {
			a := a
			root = &a
		}
	}
	// An ask's parent may live in a run on another host; keep it top level.
	for i := range t.spans {
		if _, ok := asks[t.spans[i].Parent]; !ok {
			t.spans[i].Parent = ""
		}
	}
	hosts := map[string]bool{}
	seenSessions := map[string]bool{}
	runSpans := map[string]string{}
	addSession := func(id, parent, order string, fallbackStart, fallbackEnd int64) {
		if id == "" || seenSessions[id] {
			return
		}
		seenSessions[id] = true
		sp := Span{ID: derivedSpanID("session", id), Parent: parent, Kind: "session", Name: id, SessionID: id, Order: order, Start: fallbackStart, End: fallbackEnd}
		if row, ok := sessions[id]; ok {
			sp.Indexed = true
			sp.Start, sp.End = row.Start, row.End
			sp.Host, sp.Harness, sp.Model = row.Host, row.Harness, row.Model
			sp.Cost, sp.Tokens = row.Cost, row.TotalTokens
			// Only a real summary: a dispatch worker's first prompt is the
			// same brief for every run and says nothing about this one.
			sp.Summary = strings.TrimSpace(row.Summary)
			if strings.HasPrefix(sp.Summary, dispatchPromptPrefix) {
				sp.Summary = ""
			}
			t.Cost += row.Cost
		}
		t.Sessions++
		t.spans = append(t.spans, sp)
	}
	for _, r := range t.runs {
		source := r.Name
		kind := "order"
		if r.Meta.Parent != "" {
			source, kind = r.Meta.Parent, "validation"
		}
		orderID := derivedSpanID("order", t.ID, source)
		if kind == "order" {
			first := r.History.Attempts[0].Meta
			parent := first.ParentSpanID
			if parent == "" && len(r.Meta.Asks) > 0 {
				parent = r.Meta.Asks[0].SpanID
			}
			if _, ok := asks[parent]; !ok {
				parent = ""
			}
			t.Orders = append(t.Orders, r.Name)
			sp := Span{ID: orderID, Parent: parent, Kind: "order", Name: r.Name, Order: r.Name, Text: dispatchOrderText(r), Start: ms(first.Start), Repo: r.Meta.Order.Repo, Branch: r.Meta.Branch, Revisions: len(r.History.Revisions)}
			sp.Remote = r.Remote
			if n := len(r.History.Revisions); n > 0 && r.Remote == "" {
				latest := r.History.Revisions[n-1].Order
				sp.Intent, sp.Acceptance = dispatch.OrderAskText(latest), dispatch.OrderAcceptance(latest)
			}
			t.spans = append(t.spans, sp)
		}
		used := map[int]bool{}
		for i, at := range r.History.Attempts {
			spanID := at.Meta.SpanID
			if !dispatch.ValidSpanID(spanID) {
				spanID = derivedSpanID("run", t.ID, r.Name, strconv.Itoa(i))
			}
			var res *dispatch.Result
			report := ""
			for j := range r.Results {
				if !used[j] && r.Results[j].SpanID != "" && r.Results[j].SpanID == at.Meta.SpanID {
					res, used[j] = &r.Results[j], true
					if j < len(r.Reports) {
						report = r.Reports[j]
					}
					break
				}
			}
			if res == nil && i < len(r.Results) && !used[i] && r.Results[i].SpanID == "" {
				res, used[i] = &r.Results[i], true
				if i < len(r.Reports) {
					report = r.Reports[i]
				}
			}
			sp := Span{ID: spanID, Parent: orderID, Kind: "run", Name: r.Name, Order: source, Attempt: at.Number, LaunchedBy: at.Meta.ParentSpanID, Start: ms(at.Meta.Start), Queued: ms(at.Meta.Start), Remote: r.Remote, Host: at.Meta.Host, Harness: at.Meta.Harness, Model: at.Meta.Model, State: "queued", Branch: at.Meta.Branch, Report: report,
				ContextDigest: at.Meta.ContextDigest, ContextRecipe: dispatch.ContextRecipeName(at.Meta)}
			if kind == "validation" {
				sp.Kind, sp.Name = "validation", r.Name
				for _, v := range validationsOf(t.runs, source) {
					if v.Name != r.Name {
						continue
					}
					if v.State == "pass" || v.State == "fail" {
						sp.State = v.State
					}
					if v.Verdict != nil {
						sp.Verdict = v.Verdict
					}
					sp.Evidence, sp.Error = v.Evidence, v.Error
				}
			}
			if a, ok := asks[sp.LaunchedBy]; ok {
				sp.Action = a.Action
			} else {
				sp.LaunchedBy = ""
			}
			if res != nil {
				if !res.Start.IsZero() {
					sp.Start = ms(res.Start)
				}
				sp.End = ms(res.End)
				if sp.State == "queued" || sp.Kind == "run" {
					sp.State = runState(res)
				}
				sp.SessionID, sp.PR = res.Session, res.PR
				if res.ContextDigest != "" {
					sp.ContextDigest = res.ContextDigest
				}
				if res.Branch != "" {
					sp.Branch = res.Branch
				}
				sp.Head, sp.Outcome, sp.ExitCode = res.Head, res.Outcome, res.ExitCode
				sp.Reason = firstNonEmpty(res.Reason, res.Question)
				if res.Error != "" {
					sp.Error = res.Error
				}
				if sp.Verdict == nil && res.Verdict != nil {
					sp.Verdict = res.Verdict
				}
			}
			if sp.Host != "" {
				hosts[sp.Host] = true
			}
			t.Runs++
			t.spans = append(t.spans, sp)
			runSpans[spanID] = spanID
			if res != nil {
				addSession(res.Session, spanID, source, sp.Start, sp.End)
			}
		}
		// Archived results whose attempt the history no longer names.
		for j := range r.Results {
			if !used[j] {
				addSession(r.Results[j].Session, orderID, source, ms(r.Results[j].Start), ms(r.Results[j].End))
			}
		}
	}
	// Sessions indexed into this trace that no local run result names.
	if !t.Untraced {
		var extra []trace.SessionRow
		for _, row := range sessions {
			if row.TraceID == t.ID && !seenSessions[row.SessionID] {
				extra = append(extra, row)
			}
		}
		sort.Slice(extra, func(i, j int) bool { return extra[i].Start < extra[j].Start })
		for _, row := range extra {
			addSession(row.SessionID, runSpans[row.ParentSpanID], row.Order, row.Start, row.End)
		}
	}
	// Order spans cover their attempts and validation runs.
	cover := map[string][2]int64{}
	for _, sp := range t.spans {
		if sp.Kind == "run" || sp.Kind == "validation" {
			id := derivedSpanID("order", t.ID, sp.Order)
			c := cover[id]
			if c[0] == 0 || (sp.Start != 0 && sp.Start < c[0]) {
				c[0] = sp.Start
			}
			if sp.End > c[1] {
				c[1] = sp.End
			}
			cover[id] = c
		}
	}
	worst := ""
	for i := range t.spans {
		sp := &t.spans[i]
		if sp.Kind == "order" {
			if c, ok := cover[sp.ID]; ok {
				if c[0] != 0 {
					sp.Start = c[0]
				}
				sp.End = c[1]
			}
			sp.State = t.orderState(sp.Name)
			if sp.State == "running" {
				sp.End = 0
			}
		}
		// A passing validation leaves the trace done; ties never depend on
		// the order records were read in.
		if st := sp.State; (sp.Kind == "order" || sp.Kind == "validation") && st != "pass" && (worst == "" || stateRank[st] > stateRank[worst]) {
			worst = st
		}
		if sp.Start != 0 && (t.Start == 0 || sp.Start < t.Start) {
			t.Start = sp.Start
		}
		if sp.End > t.End {
			t.End = sp.End
		}
	}
	if worst == "" {
		worst = "done"
	}
	t.State = worst
	if t.End > t.Start {
		t.Duration = t.End - t.Start
	}
	for h := range hosts {
		t.Hosts = append(t.Hosts, h)
	}
	sort.Strings(t.Hosts)
	t.Asks = len(asks)
	switch {
	case root != nil:
		t.Ask, t.RootHere, t.Caller = root.Text, true, &root.Caller
	case len(asks) > 0:
		// The root ask was recorded on another host's run.
		first := dispatch.AskSpan{}
		for _, a := range asks {
			if first.SpanID == "" || a.Time.Before(first.Time) {
				first = a
			}
		}
		t.Ask, t.Caller = first.Text, &first.Caller
	default:
		for _, r := range t.runs {
			if r.Meta.Parent == "" {
				t.Ask = dispatchOrderText(r)
				break
			}
		}
	}
	if t.Orders == nil {
		t.Orders = []string{}
	}
	if t.Hosts == nil {
		t.Hosts = []string{}
	}
	t.who(asks, root)
	sort.SliceStable(t.spans, func(i, j int) bool { return t.spans[i].Start < t.spans[j].Start })
}

// who fills in the trace's people and places: who asked, the hosts the
// asks were made on, the repositories, and whether its records predate
// tracing.
func (t *traceModel) who(asks map[string]dispatch.AskSpan, root *dispatch.AskSpan) {
	traced := false
	for _, r := range t.runs {
		metas := []dispatch.Meta{r.Meta}
		for _, at := range r.History.Attempts {
			metas = append(metas, at.Meta)
		}
		for _, m := range metas {
			if m.TraceID != "" || m.SpanID != "" || m.ParentSpanID != "" || len(m.Asks) > 0 || m.Untraced != "" {
				traced = true
			}
		}
		for _, res := range r.Results {
			if res.TraceID != "" || res.SpanID != "" {
				traced = true
			}
		}
	}
	t.PreTrace = t.Untraced && t.Reason == "" && !traced

	// The user is the requester the run records hold; an ask's caller
	// sender stands in only when no run names one (an archive row from a
	// publisher that did not carry it).
	// The trace's first run is the one holding the root ask, else its
	// earliest order (a validator only when there is nothing else).
	var first *dispatch.RunRecord
	for i := range t.runs {
		r := &t.runs[i]
		if ra := dispatch.RootAsk(r.Meta); root != nil && ra != nil && ra.SpanID == root.SpanID {
			first = r
			break
		}
		if first == nil || (r.Meta.Parent == "") != (first.Meta.Parent == "") && r.Meta.Parent == "" || (r.Meta.Parent == "") == (first.Meta.Parent == "") && runStart(*r).Before(runStart(*first)) {
			first = r
		}
	}
	users := map[string]bool{}
	for _, r := range t.runs {
		if u := requester(r.Meta); u != "" {
			users[u] = true
		}
	}
	if first != nil {
		t.User = requester(first.Meta)
	}
	if len(users) == 0 {
		if t.Caller != nil && t.Caller.Sender != "" {
			t.User = t.Caller.Sender
		}
		for _, a := range asks {
			if a.Caller.Sender != "" {
				users[a.Caller.Sender] = true
			}
		}
	}
	if t.User == "" {
		t.User = firstKey(users)
	}
	t.Users = leading(t.User, users)

	askHosts := map[string]bool{}
	for _, a := range asks {
		if a.Caller.Host != "" {
			askHosts[a.Caller.Host] = true
		}
	}
	lead := ""
	if t.Caller != nil {
		lead = t.Caller.Host
	}
	t.AskHosts = leading(lead, askHosts)

	projects := map[string]bool{}
	for _, r := range t.runs {
		if p := projectName(r.Meta.Order.Repo); p != "" {
			projects[p] = true
		}
	}
	t.Projects = leading("", projects)
}

// requester is who a run was dispatched for: requested_by, else sender.
func requester(m dispatch.Meta) string {
	return firstNonEmpty(m.RequestedBy, m.Sender)
}

func runStart(r dispatch.RunRecord) time.Time {
	if len(r.History.Attempts) > 0 && !r.History.Attempts[0].Meta.Start.IsZero() {
		return r.History.Attempts[0].Meta.Start
	}
	return r.Meta.Start
}

func firstKey(set map[string]bool) string {
	keys := leading("", set)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// leading lists a set sorted, with lead (if set) first.
func leading(lead string, set map[string]bool) []string {
	out := []string{}
	if lead != "" {
		out = append(out, lead)
	}
	rest := make([]string, 0, len(set))
	for k := range set {
		if k != lead {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// orderState is the latest attempt's state, refined by its validation.
func (t *traceModel) orderState(name string) string {
	for _, r := range t.runs {
		if r.Name != name {
			continue
		}
		if len(r.Results) == 0 {
			return "queued"
		}
		latest := r.Results[len(r.Results)-1]
		state := runState(&latest)
		if state == "done" && latest.Validation != "" && latest.Validation != "pass" && latest.Validation != "pending" && latest.Validation != "running" {
			state = latest.Validation
		}
		return state
	}
	return ""
}

func validationsOf(runs []dispatch.RunRecord, source string) []dispatch.ValidationRecord {
	for _, r := range runs {
		if r.Name == source {
			return r.Validations
		}
	}
	return nil
}

// dispatchOrderText is the order's ask text, from its first revision.
func dispatchOrderText(r dispatch.RunRecord) string {
	if len(r.History.Revisions) > 0 {
		if s := dispatch.OrderAskText(r.History.Revisions[0].Order); s != "" {
			return firstLine(s)
		}
	}
	return r.Name
}

// dispatchPromptPrefix starts every dispatch worker's first prompt.
var dispatchPromptPrefix = dispatch.Prompt[:min(len(dispatch.Prompt), 32)]

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}

// traceSessions loads the session rows the records name or that the indexer
// joined to these traces. The trace_id query is best effort: an archive
// written before tracing has no such attribute.
func (s *Server) traceSessions(records []dispatch.RunRecord) (map[string]trace.SessionRow, error) {
	var ids []any
	traces := map[string]bool{}
	for _, r := range records {
		for _, res := range r.Results {
			if res.Session != "" {
				ids = append(ids, res.Session)
			}
		}
		if dispatch.ValidTraceID(r.Meta.TraceID) {
			traces[r.Meta.TraceID] = true
		}
	}
	var rows []trace.SessionRow
	for start := 0; start < len(ids); start += 500 {
		end := min(start+500, len(ids))
		batch, err := s.listRows([]any{"session_id", "In", ids[start:end]})
		if err != nil {
			return nil, err
		}
		rows = append(rows, batch...)
	}
	if len(traces) > 0 {
		list := make([]any, 0, len(traces))
		for id := range traces {
			list = append(list, id)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].(string) < list[j].(string) })
		if batch, err := s.listRows([]any{"trace_id", "In", list}); err == nil {
			rows = append(rows, batch...)
		}
	}
	out := map[string]trace.SessionRow{}
	for _, row := range latestSessions(rows) {
		out[row.SessionID] = row
	}
	return out, nil
}

func (s *Server) traces(scope any) ([]*traceModel, error) {
	records := s.runRecords(scope)
	sessions, err := s.traceSessions(records)
	if err != nil {
		return nil, err
	}
	return buildTraces(records, sessions), nil
}

// AskFacet is one choice offered by an Asks list filter.
type AskFacet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// matchAny reports whether any value is among want (empty want matches).
func matchAny(want []string, values ...[]string) bool {
	if len(want) == 0 {
		return true
	}
	for _, vs := range values {
		for _, v := range vs {
			for _, w := range want {
				if v == w {
					return true
				}
			}
		}
	}
	return false
}

func facetList(counts map[string]int, keep []string) []AskFacet {
	for _, k := range keep {
		if _, ok := counts[k]; !ok {
			counts[k] = 0
		}
	}
	out := make([]AskFacet, 0, len(counts))
	for v, n := range counts {
		out = append(out, AskFacet{Value: v, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

// asks lists traces, newest first:
// GET /api/asks?window=&user=&host=&project=&pretrace=&q=.
//
// user matches anyone recorded across a trace's runs, host an asking or an
// execution host, project an order's repository. Repeated values of one
// filter are alternatives; different filters combine. Runs from before
// tracing are left out unless pretrace=1; the response counts them. Each
// filter's choices are counted over the window's traces with the other
// filters applied, so a choice never leads to an empty list.
func (s *Server) asks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter, window, err := windowFilter(q.Get("window"), time.Now())
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	since := int64(0)
	if f, ok := filter.([]any); ok && len(f) == 3 {
		since, _ = f[2].(int64)
	}
	var scope any
	if since > 0 {
		scope = []any{"start", "Gte", since}
	}
	all, err := s.traces(scope)
	if err != nil {
		writeError(w, err, http.StatusBadGateway)
		return
	}
	users, hosts, projects := nonEmpty(q["user"]), nonEmpty(q["host"]), nonEmpty(q["project"])
	pretrace := q.Get("pretrace") == "1" || q.Get("pretrace") == "true"
	text := strings.ToLower(strings.TrimSpace(q.Get("q")))
	match := func(t *traceModel, skip string) bool {
		return (skip == "user" || matchAny(users, t.Users)) &&
			(skip == "host" || matchAny(hosts, t.AskHosts, t.Hosts)) &&
			(skip == "project" || matchAny(projects, t.Projects))
	}
	out := []TraceSummary{}
	hidden := 0
	userN, hostN, projectN := map[string]int{}, map[string]int{}, map[string]int{}
	for _, t := range all {
		end := max(t.End, t.Start)
		if end != 0 && end < since {
			continue
		}
		if t.PreTrace && !pretrace {
			if match(t, "") {
				hidden++
			}
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(t.Ask+" "+strings.Join(t.Orders, " ")+" "+t.ID), text) {
			continue
		}
		if match(t, "user") {
			for _, u := range t.Users {
				userN[u]++
			}
		}
		if match(t, "host") {
			seen := map[string]bool{}
			for _, h := range append(append([]string{}, t.AskHosts...), t.Hosts...) {
				if !seen[h] {
					seen[h] = true
					hostN[h]++
				}
			}
		}
		if match(t, "project") {
			for _, p := range t.Projects {
				projectN[p]++
			}
		}
		if match(t, "") {
			out = append(out, t.TraceSummary)
		}
	}
	writeJSON(w, map[string]any{"window": window, "traces": out, "pretrace_hidden": hidden,
		"users": facetList(userN, users), "hosts": facetList(hostN, hosts), "projects": facetList(projectN, projects)})
}

func nonEmpty(v []string) []string {
	var out []string
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// askTrace returns one trace's waterfall: GET /api/asks/{id}.
func (s *Server) askTrace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	all, err := s.traces(traceScope(id))
	if err != nil {
		writeError(w, err, http.StatusBadGateway)
		return
	}
	for _, t := range all {
		if t.ID == id {
			writeJSON(w, map[string]any{"trace": t.TraceSummary, "spans": t.spans})
			return
		}
	}
	writeError(w, errTraceNotFound, http.StatusNotFound)
}

type traceError string

func (e traceError) Error() string { return string(e) }

const errTraceNotFound = traceError("trace not found on this host's dispatch records")

// TraceContext is the breadcrumb a worker session shows back to its trace.
type TraceContext struct {
	Trace    string `json:"trace"`
	Untraced bool   `json:"untraced,omitempty"`
	Ask      string `json:"ask"`
	// Launch is the ask that launched the session's run attempt.
	Launch     string `json:"launch,omitempty"`
	LaunchText string `json:"launch_text,omitempty"`
	LaunchSpan string `json:"launch_span,omitempty"`
	Order      string `json:"order,omitempty"`
	Run        string `json:"run,omitempty"`
	RunSpan    string `json:"run_span,omitempty"`
	Attempt    int    `json:"attempt,omitempty"`
	Kind       string `json:"kind,omitempty"`
	PreTrace   bool   `json:"pretrace,omitempty"`
}

// sessionTraceContext finds the trace a session belongs to, if any run
// record on this host names it or the row carries a trace.
func (s *Server) sessionTraceContext(row trace.SessionRow) *TraceContext {
	var scope any = noArchive{}
	switch {
	case row.TraceID != "":
		scope = []any{"trace_id", "Eq", row.TraceID}
	case row.Order != "":
		scope = []any{"run", "Eq", row.Order}
	}
	records := s.runRecords(scope)
	relevant := false
	for _, r := range records {
		if row.TraceID != "" && r.Meta.TraceID == row.TraceID {
			relevant = true
			break
		}
		for _, res := range r.Results {
			if res.Session == row.SessionID {
				relevant = true
			}
		}
	}
	if !relevant {
		return nil
	}
	for _, t := range buildTraces(records, map[string]trace.SessionRow{row.SessionID: row}) {
		byID := map[string]Span{}
		for _, sp := range t.spans {
			byID[sp.ID] = sp
		}
		for _, sp := range t.spans {
			if sp.Kind != "session" || sp.SessionID != row.SessionID {
				continue
			}
			c := &TraceContext{Trace: t.ID, Untraced: t.Untraced, PreTrace: t.PreTrace, Ask: t.Ask, Order: sp.Order}
			if run, ok := byID[sp.Parent]; ok {
				c.Run, c.RunSpan, c.Attempt, c.Kind = run.Name, run.ID, run.Attempt, run.Kind
				if a, ok := byID[run.LaunchedBy]; ok {
					c.Launch, c.LaunchText, c.LaunchSpan = a.Action, a.Text, a.ID
				}
			}
			return c
		}
	}
	return nil
}
