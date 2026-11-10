// Package compat is a line-for-line port of the Python `peers` CLI (hev/skills
// peers/bin/peers). It shares the reference implementation's on-disk state
// (~/.local/state/peers-v2), wire formats and output so both can run side by
// side on one host. Keep names close to the reference to ease review.
package compat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	sep           = "\x1f"
	recentWindow  = 3600 // how far back "who messaged whom" looks
	stale         = 900  // a snapshot older than this is not shown as live
	relayTTL      = 1800 // a relayed message that can't be delivered for this long is dropped
	snapshotLines = 40   // screen lines per pane in a snapshot
)

var paneFields = []string{
	"#{pane_id}", "#{session_name}", "#{window_index}", "#{pane_index}", "#{pane_pid}",
	"#{pane_current_path}", "#{pane_title}", "#{session_attached}", "#{window_activity}",
	"#{@peers-role}", "#{pid}:#{session_id}:#{session_created}",
}

var (
	escRE = regexp.MustCompile(`\x1b\[[0-9;:?]*[A-Za-z]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][A-Za-z0-9]`)
	sgrRE = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)
)

// exitCode unwinds a command the way sys.exit does in the reference.
type exitCode int

// Refused is the reference's Refused exception.
type Refused struct{ msg string }

func (r *Refused) Error() string { return r.msg }
func refuse(format string, a ...any) error {
	if len(a) == 0 {
		return &Refused{format}
	}
	return &Refused{fmt.Sprintf(format, a...)}
}

const sandboxed = "can't reach tmux or ps: this looks like a sandboxed shell (Codex's default " +
	"sandbox blocks both). Rerun peers with escalated permissions, outside the sandbox"

// P is one invocation: the reference's module globals.
type P struct {
	home, stateDir                                string
	snapshots, outbox, pending, sentLog, relayLog string
	queueLog, transferLog, inboxDir, threads      string
	hostLabel                                     string
	stdinB64                                      *string
	in                                            io.Reader
	out, err                                      io.Writer
	procs                                         *procTable
	self                                          string // the executable remote() and hooks re-run
	sleep                                         func(time.Duration)
	exe                                           string
	tmuxQuoted                                    bool // tmux vis-encodes its output (3.4): use tmuxFields' q form
}

func newP(in io.Reader, out, errw io.Writer) *P {
	home, _ := os.UserHomeDir()
	p := &P{home: home, in: in, out: out, err: errw, sleep: time.Sleep}
	if v, ok := os.LookupEnv("PEERS_STDIN_B64"); ok {
		p.stdinB64 = &v
		os.Unsetenv("PEERS_STDIN_B64")
	}
	os.Setenv("PATH", home+"/.local/bin:/opt/homebrew/bin:/usr/local/bin:"+os.Getenv("PATH"))
	p.hostLabel = os.Getenv("PEERS_HOST_LABEL")
	// HEV_PEERS_STATE is the native engine's isolation override; honouring it
	// keeps compat and native (dispatch notifications) on one inbox tree.
	p.stateDir = os.Getenv("HEV_PEERS_STATE")
	if p.stateDir == "" {
		p.stateDir = filepath.Join(home, ".local/state/peers-v2")
	}
	p.snapshots = filepath.Join(p.stateDir, "hosts")
	p.outbox = filepath.Join(p.stateDir, "outbox")
	p.pending = filepath.Join(p.stateDir, "pending")
	p.sentLog = filepath.Join(p.stateDir, "sent.jsonl")
	p.relayLog = filepath.Join(p.stateDir, "relayed.jsonl")
	p.queueLog = filepath.Join(p.stateDir, "queued.jsonl")
	p.transferLog = filepath.Join(p.stateDir, "relay-transfers.jsonl")
	p.inboxDir = filepath.Join(p.stateDir, "inbox")
	p.threads = filepath.Join(p.stateDir, "threads")
	return p
}

func (p *P) die(message string, code ...int) {
	fmt.Fprintf(p.err, "peers: %s\n", message)
	c := 1
	if len(code) > 0 {
		c = code[0]
	}
	panic(exitCode(c))
}

func (p *P) exit(code int) { panic(exitCode(code)) }

type result struct {
	stdout, stderr string
	code           int
	timedOut       bool // runFor's deadline fired
}

// run is subprocess.run(capture_output=True, text=True). A missing executable
// reports exit 127 instead of raising, so optional tools degrade quietly.
func (p *P) run(argv []string, input *string, env ...string) result {
	return p.runFor(0, argv, input, env...)
}

// runFor is run with a hard deadline: past it the process is killed, its
// pipes are abandoned after a grace period, and the result is exit 255 with a
// "timed out" stderr. A zero limit means no deadline (run).
func (p *P) runFor(limit time.Duration, argv []string, input *string, env ...string) result {
	ctx := context.Background()
	if limit > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, limit)
		defer cancel()
	}
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	// A killed ssh can leave a child (ProxyCommand, a wrapper's sleep) holding
	// our pipes; don't wait on it past this.
	c.WaitDelay = 2 * time.Second
	if len(env) > 0 {
		c.Env = append(os.Environ(), env...)
	}
	if input != nil {
		c.Stdin = strings.NewReader(*input)
	}
	var o, e bytes.Buffer
	c.Stdout, c.Stderr = &o, &e
	err := c.Run()
	r := result{stdout: o.String(), stderr: e.String()}
	var exitErr *exec.ExitError
	switch {
	case limit > 0 && ctx.Err() == context.DeadlineExceeded:
		r.code, r.timedOut = 255, true
		r.stderr += fmt.Sprintf("%s: timed out after %s\n", argv[0], limit)
	case err == nil:
	case errors.As(err, &exitErr):
		r.code = exitErr.ExitCode()
	case errors.Is(err, os.ErrPermission):
		p.die(sandboxed)
	default:
		r.code = 127
		if r.stderr == "" {
			r.stderr = err.Error()
		}
	}
	return r
}

func (p *P) tmuxIn(input *string, check bool, args ...string) string {
	// -u: without a UTF-8 locale tmux prints the 0x1f separator (and any
	// non-ASCII) as "_". The reference gets UTF-8 there anyway, because
	// Python coerces a C locale to C.UTF-8 and exports LC_CTYPE to tmux.
	r := p.run(append([]string{"tmux", "-u"}, args...), input)
	if strings.Contains(r.stderr, "Operation not permitted") {
		p.die(sandboxed)
	}
	if check && r.code != 0 {
		msg := strings.TrimSpace(r.stderr)
		if msg == "" {
			msg = "tmux " + args[0] + " failed"
		}
		p.die(msg)
	}
	return r.stdout
}

func (p *P) tmux(args ...string) string  { return p.tmuxIn(nil, true, args...) }
func (p *P) tmuxQ(args ...string) string { return p.tmuxIn(nil, false, args...) }

// tmuxFields runs a tmux format command (list-panes, list-clients) whose
// -F format is fields joined by sep, and returns each output line's fields.
//
// tmux 3.6 prints the 0x1f separator as the byte. tmux 3.4 (Ubuntu 24.04)
// vis-encodes its output whatever the locale or -u: control bytes become
// \ooo, "$" becomes \$, and a backslash stays as it is, so the separator's
// \037 cannot be told from a field holding those four characters (the
// reference sees no panes at all there). When the output carries no
// separator byte, tmuxFields asks again with every #{...} wrapped in #{q:...},
// which backslash-escapes each backslash in a field; the escaped output then
// splits unambiguously (decodeTmuxQ). 3.6 output is used as it is. lines is
// the caller's line splitter, as in the reference (str.split vs splitlines).
func (p *P) tmuxFields(lines func(string) []string, fields []string, args ...string) [][]string {
	if !p.tmuxQuoted {
		out := p.tmuxQ(append(args, "-F", strings.Join(fields, sep))...)
		if strings.Contains(out, sep) || strings.TrimSpace(out) == "" || len(fields) < 2 {
			rows := [][]string{}
			for _, line := range lines(out) {
				if line != "" {
					rows = append(rows, strings.Split(line, sep))
				}
			}
			return rows
		}
		p.tmuxQuoted = true
	}
	quoted := make([]string, len(fields))
	for i, f := range fields {
		quoted[i] = strings.ReplaceAll(f, "#{", "#{q:")
	}
	rows := [][]string{}
	for _, line := range lines(p.tmuxQ(append(args, "-F", strings.Join(quoted, sep))...)) {
		if line != "" {
			rows = append(rows, decodeTmuxQ(line))
		}
	}
	return rows
}

// decodeTmuxQ splits one line of tmux 3.4 output for a #{q:...} format into
// fields. q escapes backslash and shell specials with a backslash; the output
// encoding then turns control bytes (the separator among them) into \ooo and
// q's \$ into \\$. So, reading left to right: \\$ is "$", \ooo is that byte
// (0x1f splits), and \X is X. A field's own backslash always arrives doubled,
// so its text can never be read as a separator. A raw 0x1f inside a field
// still splits, as it does on 3.6 and in the reference.
func decodeTmuxQ(line string) []string {
	fields := []string{}
	var b strings.Builder
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c != '\\' || i+1 == len(line) {
			b.WriteByte(c)
			continue
		}
		switch {
		case line[i+1] == '\\' && i+2 < len(line) && line[i+2] == '$':
			b.WriteByte('$')
			i += 2
		case i+3 < len(line) && line[i+1] <= '3' && isOctal(line[i+1]) && isOctal(line[i+2]) && isOctal(line[i+3]):
			v := (line[i+1]-'0')<<6 | (line[i+2]-'0')<<3 | (line[i+3] - '0')
			i += 3
			if v == sep[0] {
				fields = append(fields, b.String())
				b.Reset()
			} else {
				b.WriteByte(v)
			}
		default:
			b.WriteByte(line[i+1])
			i++
		}
	}
	return append(fields, b.String())
}

func splitNL(s string) []string { return strings.Split(s, "\n") }

func isOctal(c byte) bool { return c >= '0' && c <= '7' }

func (p *P) tilde(path string) string {
	if path == "" {
		return path
	}
	return strings.Replace(path, p.home, "~", 1)
}

func (p *P) hostname() string {
	r := p.run([]string{"hostname", "-s"}, nil)
	return strings.ToLower(strings.TrimSpace(r.stdout))
}

// selfLabel is what other machines call this one.
func (p *P) selfLabel() string {
	if v := os.Getenv("PEERS_SELF"); v != "" {
		return v
	}
	h := p.hostname()
	if strings.Contains(h, "mac-mini") {
		return "mini"
	}
	if strings.Contains(h, "macbook") {
		return "macbook"
	}
	return h
}

func (p *P) peerHosts() []string {
	me := map[string]bool{p.selfLabel(): true, p.hostname(): true, "localhost": true}
	hosts, ok := os.LookupEnv("PEERS_HOSTS")
	if !ok {
		hosts = "mini oven"
	}
	out := []string{}
	for _, h := range strings.Fields(hosts) {
		if !me[strings.ToLower(h)] {
			out = append(out, h)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// --- which panes hold an agent ------------------------------------------------

var piCLI = regexp.MustCompile(`(?:^|/)@earendil-works/pi-coding-agent/dist/(?:bundle/)?cli\.js$`)

func classify(command string) string {
	words := strings.Fields(command)
	if len(words) == 0 {
		return ""
	}
	exe := filepath.Base(words[0])
	entry := ""
	if len(words) > 1 {
		entry = words[1]
	}
	base := func(s string) string {
		if s == "" {
			return ""
		}
		return filepath.Base(s)
	}
	if len(words) == 1 && words[0] == "pi" {
		return "pi"
	}
	if exe == "node" && piCLI.MatchString(entry) {
		return "pi"
	}
	if exe == "claude" || strings.Contains(entry, "/@anthropic-ai/claude-code/") || base(entry) == "claude" {
		return "claude"
	}
	if exe == "codex" || base(entry) == "codex" || base(entry) == "codex.js" {
		return "codex"
	}
	if exe == "gemini" || exe == "opencode" || exe == "cursor-agent" || base(entry) == "gemini" || base(entry) == "opencode" {
		if exe == "node" {
			return base(entry)
		}
		return exe
	}
	return ""
}

func headless(agent, command string) bool {
	words := strings.Fields(command)
	switch agent {
	case "claude":
		return contains(words, "-p") || contains(words, "--print")
	case "codex":
		end := 3
		if len(words) < end {
			end = len(words)
		}
		return len(words) > 1 && contains(words[1:end], "exec")
	}
	return false
}

type procTable struct {
	cmd      map[int]string
	ppid     map[int]int
	children map[int][]int
	agents   map[int]string
	order    []int // agents in ps order, as the reference's dict iterates
}

func (p *P) ps() *procTable {
	if p.procs != nil {
		return p.procs
	}
	out := p.run([]string{"ps", "-axo", "pid=,ppid=,command="}, nil).stdout
	t := &procTable{cmd: map[int]string{}, ppid: map[int]int{}, children: map[int][]int{}, agents: map[int]string{}}
	var order []int
	for _, line := range strings.Split(out, "\n") {
		parts := splitN(strings.TrimSpace(line), 3)
		if len(parts) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(parts[0])
		ppid, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			continue
		}
		t.cmd[pid], t.ppid[pid] = parts[2], ppid
		t.children[ppid] = append(t.children[ppid], pid)
		order = append(order, pid)
	}
	for _, pid := range order {
		if a := classify(t.cmd[pid]); a != "" {
			if _, dup := t.agents[pid]; !dup {
				t.order = append(t.order, pid)
			}
			t.agents[pid] = a
		}
	}
	p.procs = t
	return t
}

// splitN is str.split(None, n-1).
func splitN(s string, n int) []string {
	out := []string{}
	s = strings.TrimLeft(s, " \t\n\r\v\f")
	for len(out) < n-1 && s != "" {
		i := strings.IndexAny(s, " \t\n\r\v\f")
		if i < 0 {
			break
		}
		out = append(out, s[:i])
		s = strings.TrimLeft(s[i:], " \t\n\r\v\f")
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

func (t *procTable) subtree(pid int) []int {
	stack, seen, mark := []int{pid}, []int{}, map[int]bool{}
	for len(stack) > 0 {
		q := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if mark[q] {
			continue
		}
		mark[q] = true
		seen = append(seen, q)
		stack = append(stack, t.children[q]...)
	}
	return seen
}

func (t *procTable) agentUnder(pid int) int {
	queue, seen := []int{pid}, map[int]bool{}
	for len(queue) > 0 {
		q := queue[0]
		queue = queue[1:]
		if seen[q] {
			continue
		}
		seen[q] = true
		if _, ok := t.agents[q]; ok {
			return q
		}
		queue = append(queue, t.children[q]...)
	}
	return 0
}

func (t *procTable) ancestors(pid int) []int {
	seen, mark := []int{}, map[int]bool{}
	for {
		_, ok := t.ppid[pid]
		if !ok || mark[pid] || pid <= 1 {
			return seen
		}
		mark[pid] = true
		seen = append(seen, pid)
		pid = t.ppid[pid]
	}
}

func intIn(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func dispatchRun(session string) string {
	if strings.HasPrefix(session, "d-") && len(session) > 2 {
		return session[2:]
	}
	return ""
}

var promptRE = regexp.MustCompile(`(?:^|\s)(?:-p|--print|exec)\s+([^-].*)`)

// spaceRE is Python's \s+ on str: everything str.isspace() accepts, where
// Go's \s is ASCII only and misses \v.
var spaceRE = regexp.MustCompile(`[\t-\r\x1c-\x1f\x{85}\p{Z}]+`)

func promptOf(command string) string {
	text := ""
	if m := promptRE.FindStringSubmatch(command); m != nil {
		text = m[1]
	}
	text = strings.ReplaceAll(text, "\\012", " ")
	return truncRunes(strings.TrimSpace(spaceRE.ReplaceAllString(text, " ")), 90)
}

// truncRunes is Python's s[:n]: it counts code points, a WTF-8 lone
// surrogate (or a stray non-UTF-8 byte) as one, and cuts the bytes as they
// are, so surrogates survive. ([]rune would turn them into U+FFFD.)
func truncRunes(s string, n int) string {
	at := 0
	for i := 0; i < n; i++ {
		if at == len(s) {
			return s
		}
		_, size := decodeWTF8(s[at:])
		at += size
	}
	return s[:at]
}

// pyLen is Python's len(s) for a WTF-8 string.
func pyLen(s string) int {
	n := 0
	for at := 0; at < len(s); n++ {
		_, size := decodeWTF8(s[at:])
		at += size
	}
	return n
}

func (p *P) cwds(pids []int) map[int]string {
	found := map[int]string{}
	if len(pids) == 0 {
		return found
	}
	list := make([]string, len(pids))
	for i, pid := range pids {
		list[i] = strconv.Itoa(pid)
	}
	r := p.run([]string{"lsof", "-a", "-d", "cwd", "-Fpn", "-p", strings.Join(list, ",")}, nil)
	pid := 0
	for _, line := range strings.Split(r.stdout, "\n") {
		if strings.HasPrefix(line, "p") {
			pid, _ = strconv.Atoi(line[1:])
		} else if strings.HasPrefix(line, "n") && pid != 0 {
			found[pid] = line[1:]
		}
	}
	if r.code == 127 { // no lsof (minimal Linux): /proc carries the same answer
		for _, pid := range pids {
			if dir, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid)); err == nil {
				found[pid] = dir
			}
		}
	}
	return found
}

// started is when pid started, in epoch seconds, from ps's absolute start time.
func (p *P) started(pid int) float64 {
	if pid == 0 {
		return 0
	}
	out := strings.TrimSpace(p.run([]string{"ps", "-o", "lstart=", "-p", strconv.Itoa(pid)}, nil, "LC_ALL=C").stdout)
	if out == "" {
		return 0
	}
	t, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(strings.Fields(out), " "), time.Local)
	if err != nil {
		return 0
	}
	return float64(t.Unix())
}

func now() float64 { return float64(time.Now().UnixNano()) / 1e9 }

func sortedGlob(pattern string) []string {
	m, _ := filepath.Glob(pattern)
	sort.Strings(m)
	return m
}

// glob is Python's glob.glob: directory order is unspecified; callers that
// care sort. Go's Glob is sorted, which is one valid order.
func glob(pattern string) []string { return sortedGlob(pattern) }
