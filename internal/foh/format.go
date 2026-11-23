package foh

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// formatCardBody turns raw JSON from a `hev ... --json` tool invocation into
// a compact, human-readable table or summary for the TUI card (while the model
// still receives the full raw output in ToolResult.Content).
func formatCardBody(tool, action, body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}
	var raw any
	if json.Unmarshal([]byte(trimmed), &raw) != nil {
		return trimmed
	}
	switch tool {
	case "hev_dispatch":
		switch action {
		case "hosts":
			if s := formatDispatchHosts(raw); s != "" {
				return s
			}
		case "ls":
			if s := formatDispatchLS(raw); s != "" {
				return s
			}
		case "show":
			if s := formatDispatchShow(raw); s != "" {
				return s
			}
		case "send":
			if m, ok := raw.(map[string]any); ok {
				var lines []string
				for _, k := range []string{"host", "name", "state", "branch"} {
					if v := pickStr(m, k); v != "" {
						lines = append(lines, fmt.Sprintf("%-8s %s", k, v))
					}
				}
				if len(lines) > 0 {
					return strings.Join(lines, "\n")
				}
			}
		}
	case "hev_peers":
		if action == "ls" {
			if s := formatPeersLS(raw); s != "" {
				return s
			}
		}
	case "hev_board":
		if action == "search" {
			if s := formatBoardSearch(raw); s != "" {
				return s
			}
		}
	case "hev_traces":
		if action == "search" {
			if s := formatTracesSearch(raw); s != "" {
				return s
			}
		}
	case "hev_status":
		if s := formatHevStatus(raw); s != "" {
			return s
		}
	}
	return formatGenericCompact(raw)
}

func formatDispatchHosts(raw any) string {
	// Either []host or map[host]info.
	type hrow struct {
		name, cap, running, free, orch, version string
	}
	var rows []hrow
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			rows = append(rows, hrow{
				name:    pickStr(m, "host", "name"),
				cap:     fmtNum(m["cap"]),
				running: fmtNum(m["running"]),
				free:    fmtNum(m["free"]),
				orch:    fmtNum(m["orchestrators"], m["orch"]),
				version: pickStr(m, "version"),
			})
		}
	case map[string]any:
		keys := sortedKeys(v)
		for _, k := range keys {
			m, ok := v[k].(map[string]any)
			if !ok {
				continue
			}
			rows = append(rows, hrow{
				name:    k,
				cap:     fmtNum(m["cap"]),
				running: fmtNum(m["running"]),
				free:    fmtNum(m["free"]),
				orch:    fmtNum(m["orchestrators"], m["orch"]),
				version: pickStr(m, "version"),
			})
		}
	}
	if len(rows) == 0 {
		return ""
	}
	var out []string
	out = append(out, fmt.Sprintf("%-12s %4s %4s %4s %4s  %s", "HOST", "CAP", "RUN", "FREE", "ORCH", "VERSION"))
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%-12s %4s %4s %4s %4s  %s", r.name, r.cap, r.running, r.free, r.orch, r.version))
	}
	return strings.Join(out, "\n")
}

func formatDispatchLS(raw any) string {
	var lines []string
	switch v := raw.(type) {
	case map[string]any:
		if arr, ok := v["runs"].([]any); ok {
			for _, item := range arr {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				lines = append(lines, fmt.Sprintf("%-30s %-10s %-10s %s",
					truncateLine(pickStr(m, "name"), 30),
					pickStr(m, "host"),
					pickStr(m, "state", "status"),
					pickStr(m, "pr")))
			}
		} else {
			for _, host := range sortedKeys(v) {
				hm, ok := v[host].(map[string]any)
				if !ok {
					continue
				}
				reach := "offline"
				if b, _ := hm["reachable"].(bool); b {
					reach = "online"
				}
				names, _ := hm["names"].([]any)
				lines = append(lines, fmt.Sprintf("%-10s %-7s cap=%s free=%s runs=%d",
					host, reach, fmtNum(hm["cap"]), fmtNum(hm["free"]), len(names)))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func formatDispatchShow(raw any) string {
	b, err := json.Marshal(raw)
	if err != nil {
		return ""
	}
	if wr, ok := parseShowResponse(watchedRun{Name: "run"}, b); ok {
		var lines []string
		lines = append(lines, fmt.Sprintf("state    %s", wr.State))
		if wr.HeadSHA != "" {
			lines = append(lines, fmt.Sprintf("head     %s", truncateLine(wr.HeadSHA, 12)))
		}
		if wr.PR != "" {
			lines = append(lines, fmt.Sprintf("pr       %s", wr.PR))
		}
		if wr.Report != "" {
			lines = append(lines, "", wr.Report)
		}
		return strings.Join(lines, "\n")
	}
	return ""
}

func formatPeersLS(raw any) string {
	m, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	panes, _ := m["panes"].([]any)
	if len(panes) == 0 {
		return "no active peers"
	}
	var out []string
	for _, item := range panes {
		p, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, fmt.Sprintf("%-18s %-8s %-9s %s",
			truncateLine(pickStr(p, "session", "address"), 18),
			pickStr(p, "agent", "harness"),
			pickStr(p, "state", "status"),
			truncateLine(pickStr(p, "cwd", "dir"), 36)))
	}
	return strings.Join(out, "\n")
}

func formatBoardSearch(raw any) string {
	var arr []any
	switch v := raw.(type) {
	case []any:
		arr = v
	case map[string]any:
		for _, k := range []string{"hits", "results", "threads", "items"} {
			if a, ok := v[k].([]any); ok {
				arr = a
				break
			}
		}
	}
	if len(arr) == 0 {
		return ""
	}
	var out []string
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := fmtNum(m["id"])
		if id == "-" {
			id = pickStr(m, "id")
		}
		out = append(out, fmt.Sprintf("#%-5s %-10s %s", id, pickStr(m, "board"), truncateLine(pickStr(m, "title"), 60)))
	}
	return strings.Join(out, "\n")
}

func formatTracesSearch(raw any) string {
	var arr []any
	switch v := raw.(type) {
	case []any:
		arr = v
	case map[string]any:
		for _, k := range []string{"hits", "results", "items"} {
			if a, ok := v[k].([]any); ok {
				arr = a
				break
			}
		}
	}
	if len(arr) == 0 {
		return ""
	}
	var out []string
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, fmt.Sprintf("%-12s %-8s %s",
			truncateLine(pickStr(m, "session_id", "session", "id"), 12),
			pickStr(m, "harness"),
			truncateLine(pickStr(m, "snippet", "text", "summary"), 60)))
	}
	return strings.Join(out, "\n")
}

func formatHevStatus(raw any) string {
	m, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	var out []string
	for _, k := range sortedKeys(m) {
		switch sub := m[k].(type) {
		case map[string]any:
			state := pickStr(sub, "state", "status")
			detail := pickStr(sub, "detail", "summary", "scope")
			if state != "" {
				line := fmt.Sprintf("%-12s %s", k, state)
				if detail != "" {
					line += " · " + truncateLine(detail, 50)
				}
				out = append(out, line)
			}
		case string:
			if sub != "" {
				out = append(out, fmt.Sprintf("%-12s %s", k, truncateLine(sub, 60)))
			}
		}
	}
	return strings.Join(out, "\n")
}

func formatGenericCompact(raw any) string {
	switch v := raw.(type) {
	case map[string]any:
		var out []string
		for _, k := range sortedKeys(v) {
			val := v[k]
			switch x := val.(type) {
			case string:
				if x != "" {
					out = append(out, fmt.Sprintf("%-14s %s", k, truncateLine(x, 60)))
				}
			case float64, bool:
				out = append(out, fmt.Sprintf("%-14s %v", k, x))
			case []any:
				out = append(out, fmt.Sprintf("%-14s %d item(s)", k, len(x)))
			case map[string]any:
				out = append(out, fmt.Sprintf("%-14s %d field(s)", k, len(x)))
			}
		}
		if len(out) > 0 {
			return strings.Join(out, "\n")
		}
	}
	return prettyBody(fmt.Sprint(raw))
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fmtNum(vals ...any) string {
	for _, v := range vals {
		if f, ok := v.(float64); ok {
			return fmt.Sprintf("%d", int(f))
		}
	}
	return "-"
}
