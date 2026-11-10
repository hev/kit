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

var paneFormat = strings.Join([]string{
	"#{pane_id}", "#{session_name}", "#{window_index}", "#{pane_index}", "#{pane_pid}",
	"#{pane_current_path}", "#{pane_title}", "#{session_attached}", "#{window_activity}",
	"#{@peers-role}", "#{pid}:#{session_id}:#{session_created}",
}, sep)

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
		r.code = 255
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

// tmuxLine restores the field separator in one line of tmux format output.
// tmux 3.4 (Ubuntu 24.04) prints the 0x1f separator as the four characters
// \037 whatever the locale or -u, and the reference then sees no panes at
// all; 3.6 prints the byte.
func tmuxLine(line string) string {
	if !strings.Contains(line, sep) {
		return strings.ReplaceAll(line, `\037`, sep)
	}
	return line
}
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
var spaceRE = regexp.MustCompile(`\s+`)

func promptOf(command string) string {
	text := ""
	if m := promptRE.FindStringSubmatch(command); m != nil {
		text = m[1]
	}
	text = strings.ReplaceAll(text, "\\012", " ")
	return truncRunes(strings.TrimSpace(spaceRE.ReplaceAllString(text, " ")), 90)
}

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
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
