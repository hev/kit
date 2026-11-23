package foh

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// watchedRun tracks a dispatched run started from this harness session so the
// TUI can notify and auto-respond when it finishes.
type watchedRun struct {
	Name string
	Host string
}

func (w watchedRun) Label() string {
	if w.Host != "" {
		return w.Host + "/" + w.Name
	}
	return w.Name
}

type watchResult struct {
	Run     watchedRun
	State   string
	PR      string
	HeadSHA string
	Report  string
	Error   string
	Done    bool
}

type watchTickMsg struct{}
type watchPollMsg struct {
	results []watchResult
}

func watchInterval() time.Duration { return 8 * time.Second }

func isTerminalRunState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "done", "failed", "blocked", "needs-review", "closed", "error", "stopped", "cancelled", "canceled":
		return true
	}
	return false
}

// extractDispatchedRun parses the run name and host from a hev_dispatch send
// tool call's arguments and result JSON.
func extractDispatchedRun(argsJSON, resultJSON string) (watchedRun, bool) {
	var run watchedRun
	var res map[string]any
	if json.Unmarshal([]byte(resultJSON), &res) == nil {
		if s, _ := res["name"].(string); s != "" {
			run.Name = strings.TrimSpace(s)
		}
		if s, _ := res["host"].(string); s != "" {
			run.Host = strings.TrimSpace(s)
		}
	}
	var args hevArgs
	if json.Unmarshal([]byte(argsJSON), &args) == nil {
		if run.Name == "" {
			run.Name = strings.TrimSpace(args.Name)
		}
		if run.Host == "" {
			run.Host = strings.TrimSpace(args.Host)
		}
		// Fall back to YAML frontmatter in full order markdown.
		if (run.Name == "" || run.Host == "") && args.Order != "" {
			for _, line := range strings.Split(args.Order, "\n") {
				line = strings.TrimSpace(line)
				if run.Name == "" && strings.HasPrefix(line, "name:") {
					run.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				}
				if run.Host == "" && strings.HasPrefix(line, "host:") {
					run.Host = strings.TrimSpace(strings.TrimPrefix(line, "host:"))
				}
			}
		}
	}
	return run, run.Name != ""
}

var prURLRe = regexp.MustCompile(`https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/pull/[0-9]+`)

// parseShowResponse extracts terminal run state, PR, head SHA and report from
// either the real `hev dispatch show --json` envelope (`files["result.json"]`,
// `files["status.md"]`, `files["report.md"]`) or a flat JSON object.
func parseShowResponse(r watchedRun, raw []byte) (watchResult, bool) {
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return watchResult{}, false
	}
	var state, pr, headSHA, report, errText string

	// Real `hev dispatch show --json` shape: files.{result.json, status.md, report.md}
	if files, ok := doc["files"].(map[string]any); ok {
		if rj, ok := files["result.json"].(string); ok && strings.TrimSpace(rj) != "" {
			var res map[string]any
			if json.Unmarshal([]byte(rj), &res) == nil {
				state = pickStr(res, "outcome", "state", "status")
				headSHA = pickStr(res, "head_sha")
				pr = pickStr(res, "pr", "pr_url")
				errText = pickStr(res, "error")
				end := pickStr(res, "end")
				if state == "" && end != "" && !strings.HasPrefix(end, "0001-01-01") {
					if code, ok := res["exit_code"].(float64); ok && code == 0 {
						state = "done"
					} else {
						state = "failed"
					}
				}
			}
		}
		if state == "" {
			if sm, ok := files["status.md"].(string); ok {
				lines := strings.Split(strings.TrimSpace(sm), "\n")
				for i := len(lines) - 1; i >= 0; i-- {
					line := strings.TrimSpace(lines[i])
					if line == "" {
						continue
					}
					low := strings.ToLower(line)
					switch {
					case low == "done":
						state = "done"
					case strings.HasPrefix(low, "blocked:"):
						state = "blocked"
					case strings.HasPrefix(low, "needs-decision"):
						state = "needs-review"
					}
					break
				}
			}
		}
		if rep, ok := files["report.md"].(string); ok && strings.TrimSpace(rep) != "" {
			report = strings.TrimSpace(rep)
		}
	}

	// Flat JSON fallback (used in compact API responses and unit tests).
	if state == "" {
		state = pickStr(doc, "state", "status", "outcome")
	}
	if res, ok := doc["result"].(map[string]any); ok {
		if state == "" {
			state = pickStr(res, "outcome", "state")
		}
		if headSHA == "" {
			headSHA = pickStr(res, "head_sha")
		}
	}
	if pr == "" {
		pr = pickStr(doc, "pr", "pr_url")
	}
	if headSHA == "" {
		headSHA = pickStr(doc, "head_sha")
	}
	if report == "" {
		report = strings.TrimSpace(pickStr(doc, "report", "report_md"))
	}
	if errText == "" {
		errText = pickStr(doc, "error")
	}
	if pr == "" && report != "" {
		pr = prURLRe.FindString(report)
	}
	if !isTerminalRunState(state) {
		return watchResult{}, false
	}
	return watchResult{
		Run:     r,
		State:   state,
		PR:      pr,
		HeadSHA: headSHA,
		Report:  truncateLine(report, 1200),
		Error:   errText,
		Done:    true,
	}, true
}

// pollWatchedRuns checks each watched run via `hev dispatch show ... --json`
// with a bounded timeout so it never stalls the TUI.
func pollWatchedRuns(runs []watchedRun, execFn HevRunner) []watchResult {
	if execFn == nil {
		execFn = ExecHev
	}
	out := make([]watchResult, 0, len(runs))
	for _, r := range runs {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		argv := []string{"dispatch", "show", r.Name, "--json"}
		if r.Host != "" {
			argv = []string{"dispatch", "show", r.Name, "--host", r.Host, "--json"}
		}
		raw, err := execFn(ctx, argv, "")
		cancel()
		if err != nil {
			continue
		}
		if wr, ok := parseShowResponse(r, raw); ok {
			out = append(out, wr)
		}
	}
	return out
}

func pickStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func formatWatchPrompt(wr watchResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[dispatched run finished] %s ended with state=%s", wr.Run.Label(), wr.State)
	if wr.PR != "" {
		fmt.Fprintf(&b, " pr=%s", wr.PR)
	}
	if wr.HeadSHA != "" {
		fmt.Fprintf(&b, " head=%s", wr.HeadSHA)
	}
	if wr.Error != "" {
		fmt.Fprintf(&b, "\nerror: %s", wr.Error)
	}
	if wr.Report != "" {
		fmt.Fprintf(&b, "\nreport:\n%s", wr.Report)
	}
	b.WriteString("\n\nBriefly report the outcome to the user (what passed/failed, PR/head if any, and the next step). If you need the full report, call hev_dispatch show.")
	return b.String()
}
