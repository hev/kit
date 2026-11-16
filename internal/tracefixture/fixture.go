// Package tracefixture writes a small, self-contained dispatch history with
// worker transcripts: one ask-rooted trace that spans two orders, a rework and
// a browser validation run, asked by local:hev from the laptop on the docs
// repo; a second single-order trace a teammate (alex) asked from another
// host (studio) on the kit repo; a run recorded before tracing existed; and
// a run an orchestrator launched untraced on purpose, which says why. Tests index it, and `hev serve
// --demo-traces` serves it so the Traces view can be checked in a browser.
package tracefixture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/dispatch"
)

// Info names what the fixture wrote.
type Info struct {
	Trace, OtherTrace string
	// Sessions maps each worker session ID to its run name.
	Sessions map[string]string
	// TraceSessions are the worker sessions inside Trace.
	TraceSessions []string
	// Untraced and PreTrace both name the run recorded before tracing;
	// Deliberate names the run recorded as untraced on purpose.
	Untraced    string
	PreTrace    string
	Deliberate  string
	Transcripts string
}

const (
	OrderNav     = "docs-nav-refresh"
	OrderSearch  = "docs-search-index"
	Validation   = OrderNav + "-v1"
	OrderOther   = "kit-flaky-queue-test"
	OrderLegacy  = "legacy-cleanup"
	OrderNightly = "deps-nightly-bump"
	// NightlyReason is why the orchestrator's run carries no trace.
	NightlyReason = "orchestrator nightly-deps launched this run without trace context"
)

// Write creates the fixture under home: run records in home/dispatch and
// Claude transcripts in home/.claude/projects. IDs are fixed so a fixture
// read twice describes the same trace.
func Write(home string, now time.Time) (Info, error) {
	root := filepath.Join(home, "dispatch")
	projects := filepath.Join(home, ".claude", "projects", "fixture")
	info := Info{
		Trace:       "4bf92f3577b34da6a3ce929d0e0e4736",
		OtherTrace:  "0af7651916cd43dd8448eb211c80319c",
		Sessions:    map[string]string{},
		Transcripts: filepath.Join(home, ".claude", "projects"),
	}
	t0 := now.Add(-26 * time.Hour).UTC().Truncate(time.Second)
	foh := dispatch.Caller{Session: "foh-hev-3", Harness: "claude", HarnessSession: "6f1c3c1e-foh0-4a8e-9a51-000000000003", Host: "laptop", Sender: "local:hev"}

	intake := dispatch.AskSpan{SpanID: "a1a1a1a1a1a1a1a1", Kind: dispatch.AskIntake, Action: "dispatch", Text: "Refresh the docs navigation so every guide is reachable in two clicks, and make search index the new pages.", Caller: foh, Time: t0}
	rework := dispatch.AskSpan{SpanID: "a2a2a2a2a2a2a2a2", ParentSpanID: intake.SpanID, Kind: dispatch.AskFollowUp, Action: "rework", Text: "The sidebar still hides the API guides. Keep them expanded by default.", Caller: foh, Time: t0.Add(95 * time.Minute)}
	validate := dispatch.AskSpan{SpanID: "a3a3a3a3a3a3a3a3", ParentSpanID: intake.SpanID, Kind: dispatch.AskFollowUp, Action: "validate", Text: "Validate " + OrderNav + " in a browser.", Caller: foh, Time: t0.Add(150 * time.Minute)}
	alex := dispatch.Caller{Session: "alex-main", Harness: "codex", Host: "studio", Sender: "local:alex"}
	search := dispatch.AskSpan{SpanID: "a4a4a4a4a4a4a4a4", ParentSpanID: intake.SpanID, Kind: dispatch.AskFollowUp, Action: "continue", Text: "Search has to index the new guide pages too.", Caller: foh, Time: t0.Add(30 * time.Minute)}

	type attempt struct {
		span, parent, session, host, harness, model string
		start, end                                  time.Time
		exit                                        int
		pr                                          string
		// title is the session's harness title (its summary); report is
		// the worker's report.md for this attempt.
		title, report string
	}
	type run struct {
		name, order, branch, parent, host string
		// repo is the order's repository (docs when empty), sender who
		// the run was dispatched for (local:hev when empty), untraced the
		// reason a run carries no trace on purpose.
		repo, sender, untraced string
		trace, parentSpan      string
		asks                   []dispatch.AskSpan
		attempts               []attempt
		validation             string
		acceptance             []string
	}
	navOrder := "Refresh the docs navigation.\n\nEvery guide should be reachable from the sidebar in two clicks. Group the guides by task (Start, Build, Operate, Reference) instead of by product area, and keep the current URLs."
	navAcceptance := []string{"Every guide page is reachable from the sidebar in at most two clicks.", "Existing guide URLs still resolve; no redirects are needed.", "The sidebar groups are Start, Build, Operate and Reference."}
	nav := run{name: OrderNav, order: navOrder, acceptance: navAcceptance, host: "oven", trace: info.Trace, asks: []dispatch.AskSpan{intake, rework, validate},
		attempts: []attempt{
			{span: "b1b1b1b1b1b1b1b1", parent: intake.SpanID, session: "11111111-0000-4000-8000-000000000001", host: "oven", harness: "claude", model: "claude-opus-5-5", start: t0.Add(2 * time.Minute), end: t0.Add(48 * time.Minute),
				title: "Regroup docs sidebar by task", report: "# docs-nav-refresh: report\n\nOutcome: done.\n\nRegrouped the sidebar into Start, Build, Operate and Reference. Every guide is now two clicks from the root. URLs are unchanged.\n\n- `nav.go`: new group table\n- `go test ./...` passes\n\nLeft out: the API guides stay collapsed under Reference.\n"},
			{span: "b2b2b2b2b2b2b2b2", parent: rework.SpanID, session: "11111111-0000-4000-8000-000000000002", host: "oven", harness: "claude", model: "claude-opus-5-5", start: t0.Add(97 * time.Minute), end: t0.Add(140 * time.Minute), pr: "https://github.com/example/docs/pull/41",
				title: "Expand API guides by default", report: "# docs-nav-refresh: report (rework)\n\nOutcome: done. PR: https://github.com/example/docs/pull/41\n\nThe API guides under Reference now start expanded, so each one is two clicks from the root.\n\n- `nav.go`: `Expanded: true` on the API group\n- New test `TestAPIGuidesExpanded`\n"},
		}, validation: "pass"}
	val := run{name: Validation, parent: OrderNav, order: navOrder, acceptance: navAcceptance, host: "mini", trace: info.Trace, parentSpan: validate.SpanID,
		attempts: []attempt{{span: "c1c1c1c1c1c1c1c1", parent: validate.SpanID, session: "11111111-0000-4000-8000-000000000003", host: "mini", harness: "claude", model: "claude-sonnet-5-5", start: t0.Add(152 * time.Minute), end: t0.Add(171 * time.Minute),
			title: "Browser check of the docs sidebar", report: "# Validation of docs-nav-refresh\n\nVerdict: pass, on head 0000…0002 in Chromium at 1440px and 390px.\n\nClicked through all 38 guides from the root; the deepest took two clicks. Old URLs load without redirects.\n"}}}
	srch := run{name: OrderSearch, order: "Index the new guide pages.\n\nThe search index is built from a fixed page list. Build it from the sidebar instead, so pages added by docs-nav-refresh are found.", acceptance: []string{"Searching for a guide title finds the guide.", "The index is built from the sidebar, not a separate list."}, host: "oven", trace: info.Trace, asks: []dispatch.AskSpan{search},
		attempts: []attempt{{span: "d1d1d1d1d1d1d1d1", parent: search.SpanID, session: "11111111-0000-4000-8000-000000000004", host: "oven", harness: "codex", model: "gpt-5.3-codex", start: t0.Add(33 * time.Minute), end: t0.Add(88 * time.Minute), pr: "https://github.com/example/docs/pull/42",
			report: "# docs-search-index: report\n\nOutcome: done. PR: https://github.com/example/docs/pull/42\n\nThe indexer now walks the sidebar tree. All 38 guide titles are found by search.\n"}}}
	otherAsk := dispatch.AskSpan{SpanID: "e0e0e0e0e0e0e0e0", Kind: dispatch.AskIntake, Action: "dispatch", Text: "Find and fix the flaky queue test in kit.", Caller: alex, Time: t0.Add(5 * time.Hour)}
	other := run{name: OrderOther, order: "Fix the flaky queue test.", host: "oven", repo: "kit", sender: "alex", trace: info.OtherTrace, asks: []dispatch.AskSpan{otherAsk},
		attempts: []attempt{{span: "e1e1e1e1e1e1e1e1", parent: otherAsk.SpanID, session: "11111111-0000-4000-8000-000000000005", host: "oven", harness: "claude", model: "claude-opus-5-5", start: t0.Add(5*time.Hour + time.Minute), end: t0.Add(5*time.Hour + 21*time.Minute), exit: 1}}}
	legacy := run{name: OrderLegacy, order: "Remove the legacy cleanup job.", host: "oven",
		attempts: []attempt{{session: "11111111-0000-4000-8000-000000000006", host: "oven", harness: "claude", model: "claude-opus-5-5", start: t0.Add(-3 * time.Hour), end: t0.Add(-2*time.Hour - 30*time.Minute)}}}
	nightly := run{name: OrderNightly, order: "Bump the nightly dependency set and run the tests.", host: "mini", repo: "kit", untraced: NightlyReason,
		attempts: []attempt{{session: "11111111-0000-4000-8000-000000000007", host: "mini", harness: "codex", model: "gpt-5.3-codex", start: t0.Add(8 * time.Hour), end: t0.Add(8*time.Hour + 12*time.Minute)}}}
	info.Untraced, info.PreTrace, info.Deliberate = OrderLegacy, OrderLegacy, OrderNightly

	for _, r := range []run{nav, val, srch, other, legacy, nightly} {
		dir := filepath.Join(root, r.name)
		if err := os.MkdirAll(filepath.Join(dir, "wt"), 0o700); err != nil {
			return info, err
		}
		harness := r.attempts[0].harness
		acceptance := r.acceptance
		if len(acceptance) == 0 {
			acceptance = []string{"Done"}
		}
		repo, sender := r.repo, r.sender
		if repo == "" {
			repo = "docs"
		}
		if sender == "" {
			sender = "local:hev"
		}
		front := fmt.Sprintf("---\nname: %s\nhost: %s\nrepo: %s\nharness: %s\nvalidation: %s\n---\n%s\n\n## Acceptance\n- %s\n", r.name, r.host, repo, harness, map[bool]string{true: "browser", false: "none"}[r.name == OrderNav], r.order, strings.Join(acceptance, "\n- "))
		if err := os.WriteFile(filepath.Join(dir, "order.md"), []byte(front), 0o600); err != nil {
			return info, err
		}
		o, err := dispatch.Parse(front)
		if err != nil {
			return info, err
		}
		branch := "d/" + r.name
		if r.parent != "" {
			branch = "d/" + r.parent
		}
		var h dispatch.History
		h.Revisions = []dispatch.Revision{{Number: 1, Time: r.attempts[0].start, Order: front}}
		var m dispatch.Meta
		for i, a := range r.attempts {
			m = dispatch.Meta{Sender: sender, RequestedBy: sender, Parent: r.parent, Order: o, Branch: branch, Start: a.start, TraceID: r.trace, Asks: r.asks, Untraced: r.untraced}
			if r.trace != "" {
				m.SpanID, m.ParentSpanID = a.span, a.parent
			} else {
				m.Asks = nil
			}
			m.Model = a.model
			h.Attempts = append(h.Attempts, dispatch.Attempt{Number: i + 1, Revision: 1, Meta: m})
			res := dispatch.Result{Start: a.start, End: a.end, ExitCode: a.exit, Branch: branch, Head: fmt.Sprintf("%040d", i+1), PR: a.pr, Session: a.session, TraceID: m.TraceID, SpanID: m.SpanID, Outcome: "done"}
			if a.exit != 0 {
				res.Outcome, res.Error = "", "worker exited 1 before writing a report"
			}
			if r.validation != "" && i == len(r.attempts)-1 {
				res.Validation = r.validation
			}
			// Earlier attempts are archived the way rework archives them:
			// result.json and report.md renamed with nearby stamps.
			name, report := "result.json", "report.md"
			if i < len(r.attempts)-1 {
				name = fmt.Sprintf("result.json.%d", a.end.UnixNano())
				report = fmt.Sprintf("report.md.%d", a.end.UnixNano()+1234)
			}
			if err := dispatch.WriteJSON(filepath.Join(dir, name), res); err != nil {
				return info, err
			}
			if a.report != "" {
				if err := os.WriteFile(filepath.Join(dir, report), []byte(a.report), 0o600); err != nil {
					return info, err
				}
			}
			info.Sessions[a.session] = r.name
			if r.trace == info.Trace {
				info.TraceSessions = append(info.TraceSessions, a.session)
			}
			if err := writeTranscript(projects, a.session, filepath.Join(dir, "wt"), a.model, branch, firstLine(r.order), a.title, a.start, a.end); err != nil {
				return info, err
			}
		}
		if err := dispatch.WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
			return info, err
		}
		if err := dispatch.WriteJSON(filepath.Join(dir, "history.json"), h); err != nil {
			return info, err
		}
		if r.name == OrderNav {
			verdict := &dispatch.Verdict{Verdict: "pass"}
			for _, item := range navAcceptance {
				verdict.Items = append(verdict.Items, dispatch.VerdictItem{Item: item, Result: "pass", Evidence: []string{"sidebar-1440.png", "sidebar-390.png"}, Note: "Checked in Chromium at 1440px and 390px."})
			}
			v := []dispatch.ValidationRecord{{Name: Validation, Host: "mini", State: "pass", Revision: fmt.Sprintf("%040d", 2), Verdict: verdict, Evidence: "~/jobs/" + Validation + "/"}}
			if err := dispatch.WriteJSON(filepath.Join(dir, "validation.json"), v); err != nil {
				return info, err
			}
		}
	}
	return info, nil
}

// writeTranscript writes a short Claude-format worker transcript: the order
// prompt, a few tool calls with results, and a closing message.
func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}

func writeTranscript(dir, session, cwd, model, branch, task, title string, start, end time.Time) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	step := end.Sub(start) / 8
	var lines []string
	add := func(i int, v map[string]any) {
		v["sessionId"], v["cwd"], v["gitBranch"] = session, cwd, branch
		v["timestamp"] = start.Add(time.Duration(i) * step).UTC().Format(time.RFC3339Nano)
		v["uuid"] = fmt.Sprintf("%s-%02d", session[len(session)-4:], i)
		if i > 0 {
			v["parentUuid"] = fmt.Sprintf("%s-%02d", session[len(session)-4:], i-1)
		}
		b, _ := json.Marshal(v)
		lines = append(lines, string(b))
	}
	usage := map[string]any{"input_tokens": 1200, "output_tokens": 640, "cache_read_input_tokens": 42000, "cache_creation_input_tokens": 3100}
	assistant := func(content ...map[string]any) map[string]any {
		return map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "model": model, "id": "msg-" + session + fmt.Sprint(len(lines)), "content": content, "usage": usage}}
	}
	toolResult := func(id, text string) map[string]any {
		return map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": id, "content": text}}}}
	}
	add(0, map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": dispatch.Prompt + " Task: " + task}})
	add(1, assistant(map[string]any{"type": "text", "text": "Reading the order and the inbox."}, map[string]any{"type": "tool_use", "id": "tool-1-" + session, "name": "Read", "input": map[string]any{"file_path": "../order.md"}}))
	add(2, toolResult("tool-1-"+session, task))
	add(3, assistant(map[string]any{"type": "tool_use", "id": "tool-2-" + session, "name": "Bash", "input": map[string]any{"command": "go test ./..."}}))
	add(4, toolResult("tool-2-"+session, "ok  \tgithub.com/example/docs\t1.2s"))
	add(5, assistant(map[string]any{"type": "tool_use", "id": "tool-3-" + session, "name": "Edit", "input": map[string]any{"file_path": "nav.go"}}))
	add(6, toolResult("tool-3-"+session, "edited"))
	add(7, assistant(map[string]any{"type": "text", "text": "Done: " + strings.TrimSuffix(task, ".") + ". Report written to ../report.md."}))
	if title != "" {
		b, _ := json.Marshal(map[string]any{"type": "ai-title", "sessionId": session, "aiTitle": title})
		lines = append(lines, string(b))
	}
	return os.WriteFile(filepath.Join(dir, session+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}
