package foh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

const (
	// MaxToolOutput caps what one tool result sends back to the model.
	MaxToolOutput = 32 << 10
	// MaxCardBody caps the body a TUI card keeps.
	MaxCardBody         = 8 << 10
	defaultToolTimeout  = 60 * time.Second
	defaultBashTimeout  = 2 * time.Minute
	maxBashTimeout      = 10 * time.Minute
	defaultHevTimeout   = 60 * time.Second
	maxReadFile         = 256 << 10
	maxGrepFileSize     = 2 << 20
	maxGlobResults      = 1000
	maxGrepMatches      = 500
	maxWalkEntries      = 200_000
	maxEditFileSize     = 4 << 20
	maxCapturedOutBytes = 1 << 20
)

// Card is the structured, renderable result of a first-class hev tool.
type Card struct {
	Tool    string `json:"tool"`
	Action  string `json:"action"`
	Summary string `json:"summary"`
	Body    string `json:"body"`
	OK      bool   `json:"ok"`
}

// ToolResult is one tool execution's outcome.
type ToolResult struct {
	Content string
	IsError bool
	Card    *Card
}

// Tool is one callable tool.
type Tool struct {
	Spec    FunctionSpec
	Timeout time.Duration
	Run     func(ctx context.Context, args json.RawMessage) ToolResult
}

// HevRunner runs one hev subcommand and returns its stdout. Tests replace it.
type HevRunner func(ctx context.Context, args []string, stdin string) ([]byte, error)

// Toolset is the YOLO tool registry: no approval prompts, bounded timeouts
// and output caps on every call.
type Toolset struct {
	Cwd     string
	Hev     HevRunner
	Secrets []string
	tools   map[string]Tool
	order   []string
}

// NewToolset registers the local and hev tools rooted at cwd.
func NewToolset(cwd string, hev HevRunner, secrets ...string) *Toolset {
	if hev == nil {
		hev = ExecHev
	}
	t := &Toolset{Cwd: cwd, Hev: hev, Secrets: secrets, tools: map[string]Tool{}}
	for _, tool := range append(t.localTools(), t.hevTools()...) {
		t.Register(tool)
	}
	return t
}

// Register adds or replaces a tool.
func (t *Toolset) Register(tool Tool) {
	if _, ok := t.tools[tool.Spec.Name]; !ok {
		t.order = append(t.order, tool.Spec.Name)
	}
	t.tools[tool.Spec.Name] = tool
}

// Specs lists tools in registration order for the request.
func (t *Toolset) Specs() []ToolSpec {
	out := make([]ToolSpec, 0, len(t.order))
	for _, name := range t.order {
		out = append(out, ToolSpec{Type: "function", Function: t.tools[name].Spec})
	}
	return out
}

// Names lists registered tool names.
func (t *Toolset) Names() []string { return append([]string(nil), t.order...) }

// IsHevTool reports whether name is a first-class hev tool.
func IsHevTool(name string) bool { return strings.HasPrefix(name, "hev_") }

// Run executes call with its timeout, caps and scrubs the output.
func (t *Toolset) Run(ctx context.Context, call ToolCall) ToolResult {
	tool, ok := t.tools[call.Function.Name]
	if !ok {
		return ToolResult{Content: "unknown tool " + call.Function.Name, IsError: true}
	}
	timeout := tool.Timeout
	if timeout <= 0 {
		timeout = defaultToolTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := json.RawMessage(call.Function.Arguments)
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage("{}")
	}
	var res ToolResult
	if !json.Valid(args) {
		res = ToolResult{Content: "arguments are not valid JSON", IsError: true}
	} else {
		res = tool.Run(ctx, args)
	}
	res.Content = capText(Scrub(res.Content, t.Secrets...), MaxToolOutput)
	if res.Card != nil {
		res.Card.Summary = Scrub(res.Card.Summary, t.Secrets...)
		res.Card.Body = capText(Scrub(res.Card.Body, t.Secrets...), MaxCardBody)
	}
	return res
}

// capText keeps the head and tail of s within max bytes.
func capText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	marker := fmt.Sprintf("\n… [%d bytes truncated] …\n", len(s)-max)
	head := (max - len(marker)) * 2 / 3
	tail := max - len(marker) - head
	return s[:head] + marker + s[len(s)-tail:]
}

func errResult(format string, a ...any) ToolResult {
	return ToolResult{Content: fmt.Sprintf(format, a...), IsError: true}
}

func schema(s string) json.RawMessage { return json.RawMessage(s) }

func (t *Toolset) abs(p string) string {
	p = expandHome(p)
	if p == "" {
		return t.Cwd
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(t.Cwd, p)
	}
	return filepath.Clean(p)
}

func (t *Toolset) localTools() []Tool {
	return []Tool{
		{Spec: FunctionSpec{Name: "read_file", Description: "Read a text file. Optional 1-based offset and line limit.",
			Parameters: schema(`{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer"},"limit":{"type":"integer"}},"required":["path"]}`)},
			Run: t.readFile},
		{Spec: FunctionSpec{Name: "glob", Description: "List files matching a glob such as **/*.go under path (default cwd).",
			Parameters: schema(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern"]}`)},
			Run: t.glob},
		{Spec: FunctionSpec{Name: "grep", Description: "Search file contents with a Go regular expression under path (default cwd); optional glob filter.",
			Parameters: schema(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"},"glob":{"type":"string"}},"required":["pattern"]}`)},
			Run: t.grep},
		{Spec: FunctionSpec{Name: "bash", Description: "Run a shell command in the working directory. YOLO: no approval. timeout_seconds defaults to 120, max 600.",
			Parameters: schema(`{"type":"object","properties":{"command":{"type":"string"},"timeout_seconds":{"type":"integer"}},"required":["command"]}`)},
			Timeout: maxBashTimeout + 5*time.Second, Run: t.bash},
		{Spec: FunctionSpec{Name: "edit_file", Description: "Replace old_string with new_string in a file (must match once unless replace_all). Empty old_string writes new_string as the whole file.",
			Parameters: schema(`{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["path","new_string"]}`)},
			Run: t.editFile},
	}
}

func (t *Toolset) readFile(_ context.Context, raw json.RawMessage) ToolResult {
	var a struct {
		Path          string
		Offset, Limit int
	}
	if err := json.Unmarshal(raw, &a); err != nil || a.Path == "" {
		return errResult("read_file needs a path")
	}
	f, err := os.Open(t.abs(a.Path))
	if err != nil {
		return errResult("read_file: %v", err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxReadFile+1))
	if err != nil {
		return errResult("read_file: %v", err)
	}
	truncated := len(b) > maxReadFile
	if truncated {
		b = b[:maxReadFile]
	}
	text := string(b)
	if a.Offset > 0 || a.Limit > 0 {
		lines := strings.Split(text, "\n")
		start := max(a.Offset-1, 0)
		if start > len(lines) {
			start = len(lines)
		}
		end := len(lines)
		if a.Limit > 0 && start+a.Limit < end {
			end = start + a.Limit
		}
		text = strings.Join(lines[start:end], "\n")
	}
	if truncated {
		text += "\n… [file truncated at 256 KiB]"
	}
	return ToolResult{Content: text}
}

var skipDirs = []string{".git", "node_modules", ".hg", ".svn", "vendor", ".venv", "__pycache__"}

// globRegexp converts a glob with ** into an anchored regexp over slash paths.
func globRegexp(pattern string) (*regexp.Regexp, error) {
	pattern = filepath.ToSlash(pattern)
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '{':
			j := strings.IndexByte(pattern[i:], '}')
			if j < 0 {
				b.WriteString(regexp.QuoteMeta(string(c)))
				continue
			}
			alts := strings.Split(pattern[i+1:i+j], ",")
			for k := range alts {
				alts[k] = regexp.QuoteMeta(alts[k])
			}
			b.WriteString("(?:" + strings.Join(alts, "|") + ")")
			i += j
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// walk visits regular files under root with a bounded entry budget.
func walk(ctx context.Context, root string, visit func(rel, path string) bool) error {
	entries := 0
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		entries++
		if entries > maxWalkEntries {
			return fs.SkipAll
		}
		if d.IsDir() {
			if path != root && slices.Contains(skipDirs, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if !visit(filepath.ToSlash(rel), path) {
			return fs.SkipAll
		}
		return nil
	})
}

func (t *Toolset) glob(ctx context.Context, raw json.RawMessage) ToolResult {
	var a struct{ Pattern, Path string }
	if err := json.Unmarshal(raw, &a); err != nil || a.Pattern == "" {
		return errResult("glob needs a pattern")
	}
	re, err := globRegexp(a.Pattern)
	if err != nil {
		return errResult("glob: %v", err)
	}
	var out []string
	err = walk(ctx, t.abs(a.Path), func(rel, _ string) bool {
		if re.MatchString(rel) {
			out = append(out, rel)
		}
		return len(out) < maxGlobResults
	})
	if err != nil {
		return errResult("glob: %v", err)
	}
	if len(out) == 0 {
		return ToolResult{Content: "no matches"}
	}
	return ToolResult{Content: strings.Join(out, "\n")}
}

func (t *Toolset) grep(ctx context.Context, raw json.RawMessage) ToolResult {
	var a struct{ Pattern, Path, Glob string }
	if err := json.Unmarshal(raw, &a); err != nil || a.Pattern == "" {
		return errResult("grep needs a pattern")
	}
	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		return errResult("grep: %v", err)
	}
	var filter *regexp.Regexp
	if a.Glob != "" {
		if filter, err = globRegexp(a.Glob); err != nil {
			return errResult("grep: %v", err)
		}
	}
	root := t.abs(a.Path)
	var out []string
	match := func(rel, path string) bool {
		if filter != nil && !filter.MatchString(rel) && !filter.MatchString(filepath.Base(path)) {
			return true
		}
		if info, err := os.Stat(path); err != nil || info.Size() > maxGrepFileSize {
			return true
		}
		b, err := os.ReadFile(path)
		if err != nil || bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0 {
			return true
		}
		for n, line := range strings.Split(string(b), "\n") {
			if re.MatchString(line) {
				out = append(out, fmt.Sprintf("%s:%d:%s", rel, n+1, truncateLine(line, 300)))
				if len(out) >= maxGrepMatches {
					return false
				}
			}
		}
		return true
	}
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		match(filepath.Base(root), root)
	} else if err := walk(ctx, root, match); err != nil {
		return errResult("grep: %v", err)
	}
	if len(out) == 0 {
		return ToolResult{Content: "no matches"}
	}
	return ToolResult{Content: strings.Join(out, "\n")}
}

func truncateLine(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// limitedBuffer keeps the first max bytes written and counts the rest.
type limitedBuffer struct {
	buf     bytes.Buffer
	max     int
	dropped int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	room := l.max - l.buf.Len()
	if room > 0 {
		l.buf.Write(p[:min(room, len(p))])
	}
	if len(p) > room {
		l.dropped += len(p) - max(room, 0)
	}
	return len(p), nil
}

func (l *limitedBuffer) String() string {
	if l.dropped > 0 {
		return l.buf.String() + fmt.Sprintf("\n… [%d more bytes dropped]", l.dropped)
	}
	return l.buf.String()
}

// toolEnv is the environment for tool subprocesses: the parent's, minus the
// provider key.
func toolEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(secretEnv, name) {
			env = append(env, kv)
		}
	}
	return env
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("bash"); err == nil {
			return exec.CommandContext(ctx, p, "-c", command)
		}
		return exec.CommandContext(ctx, "cmd", "/C", command)
	}
	if p, err := exec.LookPath("bash"); err == nil {
		return exec.CommandContext(ctx, p, "-c", command)
	}
	return exec.CommandContext(ctx, "/bin/sh", "-c", command)
}

func (t *Toolset) bash(ctx context.Context, raw json.RawMessage) ToolResult {
	var a struct {
		Command        string
		TimeoutSeconds int `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(raw, &a); err != nil || strings.TrimSpace(a.Command) == "" {
		return errResult("bash needs a command")
	}
	timeout := defaultBashTimeout
	if a.TimeoutSeconds > 0 {
		timeout = min(time.Duration(a.TimeoutSeconds)*time.Second, maxBashTimeout)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := shellCommand(ctx, a.Command)
	cmd.Dir = t.Cwd
	cmd.Env = toolEnv()
	cmd.Stdin = nil
	out := &limitedBuffer{max: maxCapturedOutBytes}
	cmd.Stdout, cmd.Stderr = out, out
	setProcessGroup(cmd)
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	text := out.String()
	switch {
	case ctx.Err() != nil:
		return ToolResult{Content: text + fmt.Sprintf("\n[timed out after %s]", timeout), IsError: true}
	case err != nil:
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return ToolResult{Content: text + fmt.Sprintf("\n[exit %d]", exit.ExitCode()), IsError: true}
		}
		return errResult("bash: %v\n%s", err, text)
	}
	if text == "" {
		text = "[no output]"
	}
	return ToolResult{Content: text}
}

func (t *Toolset) editFile(_ context.Context, raw json.RawMessage) ToolResult {
	var a struct {
		Path       string
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(raw, &a); err != nil || a.Path == "" {
		return errResult("edit_file needs a path")
	}
	path := t.abs(a.Path)
	if a.OldString == "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return errResult("edit_file: %v", err)
		}
		if err := os.WriteFile(path, []byte(a.NewString), 0o644); err != nil {
			return errResult("edit_file: %v", err)
		}
		return ToolResult{Content: fmt.Sprintf("wrote %s (%d bytes)", a.Path, len(a.NewString))}
	}
	info, err := os.Stat(path)
	if err != nil {
		return errResult("edit_file: %v", err)
	}
	if info.Size() > maxEditFileSize {
		return errResult("edit_file: file over 4 MiB")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return errResult("edit_file: %v", err)
	}
	n := strings.Count(string(b), a.OldString)
	switch {
	case n == 0:
		return errResult("edit_file: old_string not found")
	case n > 1 && !a.ReplaceAll:
		return errResult("edit_file: old_string matches %d times; add context or set replace_all", n)
	}
	updated := strings.Replace(string(b), a.OldString, a.NewString, map[bool]int{true: -1, false: 1}[a.ReplaceAll])
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return errResult("edit_file: %v", err)
	}
	return ToolResult{Content: fmt.Sprintf("edited %s (%d replacement(s))", a.Path, map[bool]int{true: n, false: 1}[a.ReplaceAll])}
}
