package dispatch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/trace"
)

// RunKey identifies a run across hosts: its name and when its first attempt
// was queued. A dashboard prefers its own record of a run over the archive's.
func RunKey(r RunRecord) string {
	start := r.Meta.Start
	if len(r.History.Attempts) > 0 && !r.History.Attempts[0].Meta.Start.IsZero() {
		start = r.History.Attempts[0].Meta.Start
	}
	sum := sha256.Sum256([]byte("kit-run/v1/" + r.Name + "/" + start.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(sum[:8])
}

// UntracedAttemptSpanID names an attempt that recorded no span of its own.
func UntracedAttemptSpanID(runKey string, i int) string {
	sum := sha256.Sum256([]byte("kit-span/v1/attempt/" + runKey + "/" + strconv.Itoa(i)))
	return hex.EncodeToString(sum[:8])
}

func unixMs(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func fromMs(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

type publishedValidation struct {
	Name  string `json:"name"`
	Host  string `json:"host,omitempty"`
	State string `json:"state,omitempty"`
}

// TraceSpanRows turns run records into the trace facts a host publishes.
// scrub cleans free text (the ask text and the order title) before it
// leaves the host. Moved runs are published by the host they moved to.
func TraceSpanRows(records []RunRecord, publisher string, scrub func(string) string) []trace.SpanRow {
	if scrub == nil {
		scrub = func(s string) string { return s }
	}
	var out []trace.SpanRow
	for _, r := range records {
		if r.Meta.Forward != nil {
			continue
		}
		key := RunKey(r)
		trace_ := ""
		if ValidTraceID(r.Meta.TraceID) {
			trace_ = r.Meta.TraceID
		}
		asks := map[string]AskSpan{}
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
		if trace_ != "" {
			for _, a := range asks {
				if !ValidSpanID(a.SpanID) {
					continue
				}
				caller, _ := json.Marshal(a.Caller)
				out = append(out, trace.SpanRow{ID: a.SpanID, Kind: "ask", TraceID: trace_, Run: r.Name, RunKey: key, Publisher: publisher, Start: unixMs(a.Time), ParentSpanID: a.ParentSpanID, Action: a.Action, AskKind: a.Kind, Text: scrub(a.Text), Caller: string(caller)})
			}
		}
		title := ""
		if len(r.History.Revisions) > 0 {
			title, _, _ = strings.Cut(OrderAskText(r.History.Revisions[0].Order), "\n")
			title = scrub(strings.TrimSpace(title))
		}
		var validations string
		if len(r.Validations) > 0 {
			var vs []publishedValidation
			for _, v := range r.Validations {
				vs = append(vs, publishedValidation{Name: v.Name, Host: v.Host, State: v.State})
			}
			b, _ := json.Marshal(vs)
			validations = string(b)
		}
		used := map[int]bool{}
		for i, at := range r.History.Attempts {
			id := at.Meta.SpanID
			if trace_ == "" || !ValidSpanID(id) {
				id = UntracedAttemptSpanID(key, i)
			}
			row := trace.SpanRow{ID: id, Kind: "attempt", TraceID: trace_, Run: r.Name, RunKey: key, Publisher: publisher, Start: unixMs(at.Meta.Start), ParentSpanID: at.Meta.ParentSpanID,
				Attempt: int64(at.Number), Revision: int64(at.Revision), Revisions: int64(len(r.History.Revisions)), Source: r.Meta.Parent, Title: title, Repo: r.Meta.Order.Repo,
				Branch: at.Meta.Branch, Host: at.Meta.Host, Harness: at.Meta.Harness, Model: at.Meta.Model, Untraced: r.Meta.Untraced, RequestedBy: requester(r.Meta)}
			if trace_ == "" {
				row.ParentSpanID = ""
			}
			var res *Result
			for j := range r.Results {
				if !used[j] && r.Results[j].SpanID != "" && r.Results[j].SpanID == at.Meta.SpanID {
					res, used[j] = &r.Results[j], true
					break
				}
			}
			if res == nil && i < len(r.Results) && !used[i] && r.Results[i].SpanID == "" {
				res, used[i] = &r.Results[i], true
			}
			if res != nil {
				row.Result, row.RunStart, row.End, row.ExitCode = 1, unixMs(res.Start), unixMs(res.End), int64(res.ExitCode)
				if res.LaunchUnconfirmed {
					row.Unconfirmed = 1
				}
				row.Outcome, row.Validation, row.Head, row.PR, row.SessionID = res.Outcome, res.Validation, res.Head, res.PR, res.Session
				if res.Branch != "" {
					row.Branch = res.Branch
				}
			}
			if i == len(r.History.Attempts)-1 {
				row.Validations = validations
			}
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// RecordsFromSpanRows rebuilds run records from published trace facts, as
// far as the Asks view needs them. Remote names the publishing host; such
// records carry no order body, Acceptance or reports.
func RecordsFromSpanRows(rows []trace.SpanRow) []RunRecord {
	type run struct {
		rec      RunRecord
		attempts []trace.SpanRow
	}
	byKey := map[string]*run{}
	var keys []string
	for _, row := range rows {
		r := byKey[row.RunKey]
		if r == nil {
			r = &run{rec: RunRecord{Name: row.Run, Remote: row.Publisher}}
			byKey[row.RunKey] = r
			keys = append(keys, row.RunKey)
		}
		switch row.Kind {
		case "ask":
			var c Caller
			_ = json.Unmarshal([]byte(row.Caller), &c)
			r.rec.Meta.Asks = append(r.rec.Meta.Asks, AskSpan{SpanID: row.ID, ParentSpanID: row.ParentSpanID, Kind: row.AskKind, Action: row.Action, Text: row.Text, Caller: c, Time: fromMs(row.Start)})
			r.rec.Meta.TraceID = row.TraceID
		case "attempt":
			r.attempts = append(r.attempts, row)
		}
	}
	var out []RunRecord
	for _, key := range keys {
		r := byKey[key]
		if len(r.attempts) == 0 {
			// Asks recorded on a run whose attempts were not published (yet).
			continue
		}
		sort.Slice(r.attempts, func(i, j int) bool { return r.attempts[i].Attempt < r.attempts[j].Attempt })
		sort.Slice(r.rec.Meta.Asks, func(i, j int) bool { return r.rec.Meta.Asks[i].Time.Before(r.rec.Meta.Asks[j].Time) })
		last := r.attempts[len(r.attempts)-1]
		m := &r.rec.Meta
		m.TraceID, m.Parent, m.Untraced, m.Branch, m.RequestedBy = last.TraceID, last.Source, last.Untraced, last.Branch, last.RequestedBy
		m.Order.Name, m.Order.Repo = r.rec.Name, last.Repo
		for i := 0; i < int(max(last.Revisions, 1)); i++ {
			r.rec.History.Revisions = append(r.rec.History.Revisions, Revision{Number: i + 1, Order: last.Title})
		}
		for _, a := range r.attempts {
			am := Meta{TraceID: a.TraceID, ParentSpanID: a.ParentSpanID, Parent: a.Source, Branch: a.Branch, Start: fromMs(a.Start)}
			if a.TraceID != "" {
				am.SpanID = a.ID
			}
			am.Host, am.Harness, am.Model = a.Host, a.Harness, a.Model
			r.rec.History.Attempts = append(r.rec.History.Attempts, Attempt{Number: int(a.Attempt), Revision: int(a.Revision), Meta: am})
			if a.Result != 0 {
				res := Result{Start: fromMs(a.RunStart), End: fromMs(a.End), ExitCode: int(a.ExitCode), LaunchUnconfirmed: a.Unconfirmed != 0, Outcome: a.Outcome, Validation: a.Validation, Head: a.Head, PR: a.PR, Session: a.SessionID, Branch: a.Branch, TraceID: a.TraceID, SpanID: am.SpanID}
				r.rec.Results = append(r.rec.Results, res)
				r.rec.Reports = append(r.rec.Reports, "")
			}
		}
		first := r.rec.History.Attempts[0].Meta
		m.Start, m.SpanID, m.ParentSpanID = first.Start, r.rec.History.Attempts[len(r.rec.History.Attempts)-1].Meta.SpanID, first.ParentSpanID
		m.Host, m.Harness, m.Model = last.Host, last.Harness, last.Model
		if last.Validations != "" {
			var vs []publishedValidation
			if json.Unmarshal([]byte(last.Validations), &vs) == nil {
				for _, v := range vs {
					r.rec.Validations = append(r.rec.Validations, ValidationRecord{Name: v.Name, Host: v.Host, State: v.State})
				}
			}
		}
		out = append(out, r.rec)
	}
	return out
}
