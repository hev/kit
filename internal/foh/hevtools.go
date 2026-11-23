package foh

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ExecHev runs this hev binary with args, a bounded timeout and capped output.
func ExecHev(ctx context.Context, args []string, stdin string) ([]byte, error) {
	exe, err := os.Executable()
	if err != nil {
		exe = "hev"
	}
	ctx, cancel := context.WithTimeout(ctx, defaultHevTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = toolEnv()
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out := &limitedBuffer{max: maxCapturedOutBytes}
	errOut := &limitedBuffer{max: 16 << 10}
	cmd.Stdout, cmd.Stderr = out, errOut
	setProcessGroup(cmd)
	cmd.WaitDelay = 2 * time.Second
	err = cmd.Run()
	if ctx.Err() != nil {
		return []byte(out.String()), fmt.Errorf("hev %s: timed out after %s", args[0], defaultHevTimeout)
	}
	if err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return []byte(out.String()), fmt.Errorf("hev %s: %s", strings.Join(args[:min(2, len(args))], " "), msg)
	}
	return []byte(out.String()), nil
}

type hevAction struct {
	// args builds the hev argv from the tool arguments; stdin is optional.
	args func(a hevArgs) (argv []string, stdin string, err error)
}

type hevArgs struct {
	Action  string `json:"action"`
	Name    string `json:"name"`
	Target  string `json:"target"`
	Message string `json:"message"`
	Query   string `json:"query"`
	ID      string `json:"id"`
	Board   string `json:"board"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Lines   int    `json:"lines"`
	Limit   int    `json:"limit"`
	// steer
	Level string `json:"level"`
	// dispatch send
	Order   string `json:"order"`
	Repo    string `json:"repo"`
	Host    string `json:"host"`
	Base    string `json:"base"`
	Harness string `json:"harness"`
	Model   string `json:"model"`
}

func need(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}

var orderName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)

// composeOrder builds an order file from fields when no full order is given.
func composeOrder(a hevArgs) (string, error) {
	if strings.TrimSpace(a.Order) != "" {
		return a.Order, nil
	}
	if !orderName.MatchString(a.Name) {
		return "", fmt.Errorf("name must be lowercase letters, digits and hyphens")
	}
	if err := need("repo", a.Repo); err != nil {
		return "", err
	}
	if err := need("body", a.Body); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("---\n")
	for _, kv := range [][2]string{{"name", a.Name}, {"host", a.Host}, {"repo", a.Repo}, {"base", a.Base}, {"harness", a.Harness}, {"model", a.Model}} {
		if v := strings.TrimSpace(kv[1]); v != "" {
			if strings.ContainsAny(v, "\n\r") {
				return "", fmt.Errorf("%s must be one line", kv[0])
			}
			fmt.Fprintf(&b, "%s: %s\n", kv[0], v)
		}
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(a.Body))
	b.WriteString("\n")
	return b.String(), nil
}

var hevToolDefs = []struct {
	name, desc, params string
	actions            map[string]hevAction
}{
	{"hev_dispatch", "Dispatch orders to worker hosts: ls runs, show NAME, list hosts, or send an order (full markdown in order, or name/repo/body with optional host/base/harness/model).",
		`{"type":"object","properties":{"action":{"type":"string","enum":["ls","show","hosts","send"]},"name":{"type":"string"},"order":{"type":"string"},"repo":{"type":"string"},"host":{"type":"string"},"base":{"type":"string"},"harness":{"type":"string"},"model":{"type":"string"},"body":{"type":"string"}},"required":["action"]}`,
		map[string]hevAction{
			"ls":    {func(hevArgs) ([]string, string, error) { return []string{"dispatch", "ls", "--json"}, "", nil }},
			"hosts": {func(hevArgs) ([]string, string, error) { return []string{"dispatch", "hosts", "--json"}, "", nil }},
			"show": {func(a hevArgs) ([]string, string, error) {
				if err := need("name", a.Name); err != nil {
					return nil, "", err
				}
				argv := []string{"dispatch", "show", a.Name, "--json"}
				if h := strings.TrimSpace(a.Host); h != "" {
					argv = append(argv, "--host", h)
				}
				return argv, "", nil
			}},
			"send": {func(a hevArgs) ([]string, string, error) {
				order, err := composeOrder(a)
				argv := []string{"dispatch", "-", "--json"}
				if h := strings.TrimSpace(a.Host); h != "" {
					argv = append(argv, "--host", h)
				}
				return argv, order, err
			}},
		}},
	{"hev_peers", "See and message live agent sessions: ls, read TARGET (last lines of its pane), send TARGET MESSAGE. Use the short session name (e.g. foh-kit-7) for local peers on this machine, and HOST/SESSION only for remote hosts.",
		`{"type":"object","properties":{"action":{"type":"string","enum":["ls","read","send"]},"target":{"type":"string"},"message":{"type":"string"},"lines":{"type":"integer"}},"required":["action"]}`,
		map[string]hevAction{
			"ls": {func(hevArgs) ([]string, string, error) { return []string{"peers", "ls", "--json"}, "", nil }},
			"read": {func(a hevArgs) ([]string, string, error) {
				n := a.Lines
				if n <= 0 || n > 400 {
					n = 60
				}
				return []string{"peers", "read", a.Target, "-n", fmt.Sprint(n)}, "", need("target", a.Target)
			}},
			"send": {func(a hevArgs) ([]string, string, error) {
				if err := need("target", a.Target); err != nil {
					return nil, "", err
				}
				return []string{"peers", "send", a.Target, "-"}, a.Message, need("message", a.Message)
			}},
		}},
	{"hev_steer", "Steer a BOH dispatch run or orchestrator (o-NAME): send TARGET MESSAGE with optional level (note, urgent, halt, redirect), or ls TARGET for receipts.",
		`{"type":"object","properties":{"action":{"type":"string","enum":["send","ls"]},"target":{"type":"string"},"message":{"type":"string"},"level":{"type":"string","enum":["note","urgent","halt","redirect"]}},"required":["action"]}`,
		map[string]hevAction{
			"send": {func(a hevArgs) ([]string, string, error) {
				if err := need("target", a.Target); err != nil {
					return nil, "", err
				}
				if err := need("message", a.Message); err != nil {
					return nil, "", err
				}
				argv := []string{"steer", a.Target, a.Message}
				if lvl := strings.TrimSpace(a.Level); lvl != "" {
					argv = append(argv, "--level", lvl)
				}
				return argv, "", nil
			}},
			"ls": {func(a hevArgs) ([]string, string, error) {
				if strings.TrimSpace(a.Target) != "" {
					return []string{"steer", "ls", a.Target}, "", nil
				}
				return []string{"steer", "ls"}, "", nil
			}},
		}},
	{"hev_board", "Shared findings board: search QUERY, read ID, post a finding (board, title, body).",
		`{"type":"object","properties":{"action":{"type":"string","enum":["search","read","post"]},"query":{"type":"string"},"id":{"type":"string"},"board":{"type":"string"},"title":{"type":"string"},"body":{"type":"string"}},"required":["action"]}`,
		map[string]hevAction{
			"search": {func(a hevArgs) ([]string, string, error) {
				return []string{"board", "search", a.Query}, "", need("query", a.Query)
			}},
			"read": {func(a hevArgs) ([]string, string, error) {
				return []string{"board", "read", a.ID}, "", need("id", a.ID)
			}},
			"post": {func(a hevArgs) ([]string, string, error) {
				for _, f := range [][2]string{{"board", a.Board}, {"title", a.Title}, {"body", a.Body}} {
					if err := need(f[0], f[1]); err != nil {
						return nil, "", err
					}
				}
				return []string{"board", "post", "--board", a.Board, "--title", a.Title}, a.Body, nil
			}},
		}},
	{"hev_traces", "Search past agent sessions across harnesses and hosts (search QUERY), or report usage stats.",
		`{"type":"object","properties":{"action":{"type":"string","enum":["search","stats"]},"query":{"type":"string"},"limit":{"type":"integer"}},"required":["action"]}`,
		map[string]hevAction{
			"search": {func(a hevArgs) ([]string, string, error) {
				argv := []string{"traces", "search", a.Query, "--json"}
				if a.Limit > 0 {
					argv = append(argv, "--limit", fmt.Sprint(min(a.Limit, 50)))
				}
				return argv, "", need("query", a.Query)
			}},
			"stats": {func(hevArgs) ([]string, string, error) { return []string{"traces", "stats", "--json"}, "", nil }},
		}},
	{"hev_status", "Workspace and cluster summary: peers, active orders, crons and daemon health.",
		`{"type":"object","properties":{}}`,
		map[string]hevAction{
			"": {func(hevArgs) ([]string, string, error) { return []string{"--json"}, "", nil }},
		}},
}

func (t *Toolset) hevTools() []Tool {
	var out []Tool
	for _, def := range hevToolDefs {
		def := def
		out = append(out, Tool{
			Spec:    FunctionSpec{Name: def.name, Description: def.desc, Parameters: schema(def.params)},
			Timeout: defaultHevTimeout + 5*time.Second,
			Run: func(ctx context.Context, raw json.RawMessage) ToolResult {
				var a hevArgs
				_ = json.Unmarshal(raw, &a)
				card := &Card{Tool: def.name, Action: a.Action}
				act, ok := def.actions[a.Action]
				if !ok {
					card.Summary = "unknown action " + a.Action
					return ToolResult{Content: fmt.Sprintf("%s: unknown action %q", def.name, a.Action), IsError: true, Card: card}
				}
				argv, stdin, err := act.args(a)
				if err != nil {
					card.Summary = err.Error()
					return ToolResult{Content: def.name + ": " + err.Error(), IsError: true, Card: card}
				}
				if len(argv) > 1 && argv[1] == "-" && argv[0] == "dispatch" {
					// hev dispatch reads an order file; stage it in a private temp dir.
					dir, err := os.MkdirTemp("", "hev-foh-order-")
					if err != nil {
						card.Summary = "could not stage order"
						return ToolResult{Content: "hev_dispatch: could not stage order", IsError: true, Card: card}
					}
					defer os.RemoveAll(dir)
					path := filepath.Join(dir, "order.md")
					if err := os.WriteFile(path, []byte(stdin), 0o600); err != nil {
						card.Summary = "could not stage order"
						return ToolResult{Content: "hev_dispatch: could not stage order", IsError: true, Card: card}
					}
					argv = append([]string{argv[0], path}, argv[2:]...)
					stdin = ""
				}
				out, err := t.Hev(ctx, argv, stdin)
				body := strings.TrimSpace(string(out))
				card.Body = formatCardBody(def.name, a.Action, body)
				if err != nil {
					card.Summary = "failed: " + err.Error()
					content := err.Error()
					if body != "" {
						content += "\n" + body
					}
					return ToolResult{Content: content, IsError: true, Card: card}
				}
				card.OK = true
				card.Summary = summarize(def.name, a.Action, body)
				if def.name == "hev_dispatch" && a.Action == "send" {
					if run, ok := extractDispatchedRun(string(raw), body); ok {
						card.Summary = "dispatched → " + run.Label()
					}
				} else if def.name == "hev_dispatch" && a.Action == "show" && a.Name != "" {
					run := watchedRun{Name: a.Name, Host: strings.TrimSpace(a.Host)}
					if wr, ok := parseShowResponse(run, []byte(body)); ok {
						card.Summary = run.Label() + " · " + wr.State
					} else {
						card.Summary = "show " + run.Label()
					}
				}
				if body == "" {
					body = "[no output]"
				}
				return ToolResult{Content: body, Card: card}
			},
		})
	}
	return out
}

// prettyBody indents JSON output for the card and leaves text alone.
func prettyBody(body string) string {
	var v any
	if json.Unmarshal([]byte(body), &v) != nil {
		return body
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return body
	}
	return string(b)
}

// summarize writes a one-line card headline from a hev result.
func summarize(tool, action, body string) string {
	label := strings.TrimPrefix(tool, "hev_")
	if action != "" {
		label += " " + action
	}
	var v any
	if json.Unmarshal([]byte(body), &v) == nil {
		switch x := v.(type) {
		case []any:
			return fmt.Sprintf("%s · %d item(s)", label, len(x))
		case map[string]any:
			for _, key := range []string{"runs", "hosts", "panes", "hits", "results", "items"} {
				if arr, ok := x[key].([]any); ok {
					return fmt.Sprintf("%s · %d %s", label, len(arr), key)
				}
			}
			if tool == "hev_status" {
				var parts []string
				keys := make([]string, 0, len(x))
				for k := range x {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					if m, ok := x[k].(map[string]any); ok {
						if s, ok := m["state"].(string); ok {
							parts = append(parts, k+" "+s)
						}
					}
				}
				if len(parts) > 0 {
					return label + " · " + strings.Join(parts, ", ")
				}
			}
			return fmt.Sprintf("%s · %d field(s)", label, len(x))
		}
	}
	first, _, _ := strings.Cut(body, "\n")
	if first == "" {
		return label + " · ok"
	}
	return label + " · " + truncateLine(first, 100)
}
