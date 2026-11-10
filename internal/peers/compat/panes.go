package compat

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Row is one pane, managed runner or headless run (the reference's row dict).
type Row struct {
	ID, Session, Window, Pane string
	PID                       int
	Cwd, Title                string
	Attached                  bool
	Activity                  any
	Agent                     string
	AgentPID                  int
	Headless                  bool
	Dispatch                  string
	Role                      string
	TmuxIdentity              string
	Managed                   *Obj
	inboxReceipt              *Obj
}

var peerRoles = []string{"foh", "boh", "offline"}

// peerRole: untagged existing sessions retain their prior coordinated-worker behavior.
func peerRole(role string) string {
	if role == "" {
		return "boh"
	}
	return role
}

func participates(role string) bool { r := peerRole(role); return r == "foh" || r == "boh" }

var managedName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

func (p *P) managedPath(name string) (string, error) {
	if !managedName.MatchString(name) {
		return "", refuse("invalid managed peer name")
	}
	return filepath.Join(p.stateDir, "managed", name+".json"), nil
}

func intOf(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), n == float64(int(n))
	}
	return 0, false
}

// managedRecord binds one runner process lifetime, not a reusable display name.
func (p *P) managedRecord(name string) *Obj {
	path, err := p.managedPath(name)
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	rec, err := LoadObj(b)
	if err != nil || !rec.Has("pid") {
		return nil
	}
	pid, ok := intOf(rec.Get("pid"))
	if !ok {
		return nil
	}
	if strings.TrimSpace(p.tmuxQ("display-message", "-p", "-t", "="+name+":", "#{session_id}")) != "" {
		return nil // a reused interactive name cannot inherit a managed binding
	}
	birth, _ := rec.Num("birth")
	uid, _ := rec.Num("uid")
	if !Equal(rec.Get("version"), 1) || rec.Str("name") != name || !rec.Has("uid") || int(uid) != os.Getuid() ||
		!Truthy(rec.Get("birth")) || p.started(pid) != birth || !rec.Has("command") {
		return nil
	}
	if cmd, ok := p.ps().cmd[pid]; !ok || !Equal(cmd, rec.Get("command")) {
		return nil
	}
	root, ok := rec.Get("root").(string)
	if !ok {
		return nil
	}
	sb, err := os.ReadFile(filepath.Join(root, "state.json"))
	if err != nil {
		return nil
	}
	state, err := LoadObj(sb)
	if err != nil {
		return nil
	}
	if !Equal(state.Get("pid"), pid) || Equal(state.Get("phase"), "stopped") {
		return nil
	}
	return rec.Merge(NewObj("checkpoint", state))
}

func (p *P) managedRows() []*Row {
	rows := []*Row{}
	for _, path := range glob(filepath.Join(p.stateDir, "managed", "*.json")) {
		rec := p.managedRecord(strings.TrimSuffix(filepath.Base(path), ".json"))
		if rec == nil {
			continue
		}
		state := rec.Obj("checkpoint")
		activity := state.Get("finished")
		if !state.Has("finished") {
			activity = state.Get("started")
			if !state.Has("started") {
				activity = 0
			}
		}
		pid, _ := intOf(rec.Get("pid"))
		rows = append(rows, &Row{ID: "managed:" + rec.Str("name"), Session: rec.Str("name"), PID: pid, Cwd: rec.Str("root"),
			Title: rec.Str("name"), Activity: activity, Agent: "claude", AgentPID: p.ps().agentUnder(pid), Headless: true,
			Role: "boh", Managed: rec})
	}
	return rows
}

func (p *P) managedScreen(r *Row) []string {
	state := r.Managed.Obj("checkpoint")
	o := NewObj()
	for _, k := range []string{"phase", "session", "shift", "turn", "result", "error"} {
		o.Set(k, state.Get(k))
	}
	return strings.Split(Dumps(o, 2), "\n")
}

func (p *P) panes(includeExcluded bool) []*Row {
	rows := p.managedRows()
	ps := p.ps()
	panePIDs := map[int]bool{}
	for _, r := range rows {
		panePIDs[r.PID] = true
	}
	for _, f := range p.tmuxFields(splitNL, paneFields, "list-panes", "-a") {
		if len(f) != 10 && len(f) != 11 {
			continue
		}
		pid, err := strconv.Atoi(f[4])
		if err != nil {
			continue
		}
		panePIDs[pid] = true
		apid := ps.agentUnder(pid)
		activity := 0
		if f[8] != "" {
			activity, _ = strconv.Atoi(f[8])
		}
		row := &Row{ID: f[0], Session: f[1], Window: f[2], Pane: f[3], PID: pid, Cwd: f[5], Title: f[6],
			Attached: f[7] != "0", Activity: activity, Agent: ps.agents[apid], AgentPID: apid,
			Headless: apid != 0 && headless(ps.agents[apid], ps.cmd[apid]), Role: peerRole(f[9])}
		if len(f) == 11 {
			row.TmuxIdentity = f[10]
		}
		rows = append(rows, row)
	}
	// hev dispatch workers: send reaches them through `hev dispatch say`.
	for _, r := range rows {
		if name := dispatchRun(r.Session); name != "" && r.Managed == nil {
			r.Dispatch, r.Headless = name, true
		}
	}
	// Headless runs outside tmux: top-level agent processes with no pane above them.
	orphans := []int{}
	for _, pid := range ps.order {
		if !headless(ps.agents[pid], ps.cmd[pid]) {
			continue
		}
		up := ps.ancestors(pid)
		if len(up) > 0 {
			up = up[1:]
		}
		covered := false
		for _, q := range up {
			if _, agent := ps.agents[q]; panePIDs[q] || agent {
				covered = true
				break
			}
		}
		if !covered {
			orphans = append(orphans, pid)
		}
	}
	where := p.cwds(orphans)
	for _, pid := range orphans {
		rows = append(rows, &Row{ID: fmt.Sprintf("pid:%d", pid), Session: fmt.Sprintf("pid:%d", pid), PID: pid,
			Cwd: where[pid], Title: promptOf(ps.cmd[pid]), Activity: 0, Agent: ps.agents[pid], AgentPID: pid, Headless: true})
	}
	if includeExcluded {
		return rows
	}
	out := []*Row{}
	for _, r := range rows {
		if participates(r.Role) {
			out = append(out, r)
		}
	}
	return out
}

// --- reading a pane's state ---------------------------------------------------

var workingRE = regexp.MustCompile(`^. ([\p{L}]+)… \((\d+)([smh])`)

func isWorking(line string) bool {
	if strings.Contains(strings.ToLower(line), "esc to interrupt") {
		return true
	}
	s := strings.TrimLeftFunc(line, unicode.IsSpace)
	if s == "" {
		return false
	}
	first := []rune(s)[0]
	if unicode.IsLetter(first) || unicode.IsDigit(first) || unicode.IsNumber(first) {
		return false
	}
	return workingRE.MatchString(s)
}

var dialogRE = regexp.MustCompile(`(?i)^\s*[❯›>]\s*1\.|Enter to confirm|Press enter to continue|Do you want to (proceed|make|create|allow)|Esc to cancel`)

// undimmed is a line's text with dim (SGR 2) runs removed.
func undimmed(raw string) string {
	var out strings.Builder
	dim, pos := false, 0
	for _, m := range sgrRE.FindAllStringSubmatchIndex(raw, -1) {
		if !dim {
			out.WriteString(raw[pos:m[0]])
		}
		codes := []string{}
		for _, c := range regexp.MustCompile("[;:]").Split(raw[m[2]:m[3]], -1) {
			if c != "" {
				codes = append(codes, c)
			}
		}
		if len(codes) == 0 {
			codes = []string{"0"}
		}
		for _, c := range codes {
			if c == "2" {
				dim = true
			} else if c == "0" || c == "22" {
				dim = false
			}
		}
		pos = m[1]
	}
	if !dim {
		out.WriteString(raw[pos:])
	}
	return escRE.ReplaceAllString(out.String(), "")
}

func (p *P) screen(paneID string, escapes bool) []string {
	args := []string{"capture-pane", "-p", "-t", paneID}
	if escapes {
		args = []string{"capture-pane", "-e", "-p", "-t", paneID}
	}
	return splitlines(p.tmux(args...))
}

// pyStrip is str.strip(): str.isspace() is unicode.IsSpace plus \x1c-\x1f.
func pyStrip(s string) string { return strings.TrimFunc(s, pyIsSpace) }

func pyIsSpace(r rune) bool { return unicode.IsSpace(r) || r >= 0x1c && r <= 0x1f }

func (p *P) state(r *Row) string {
	if r.Managed != nil {
		if Equal(r.Managed.Obj("checkpoint").Get("phase"), "running") {
			return "working"
		}
		return "idle"
	}
	if r.Agent == "" {
		return "-"
	}
	if r.Headless {
		if r.AgentPID != 0 {
			return "working"
		}
		return "idle"
	}
	if r.Agent != "claude" && r.Agent != "codex" {
		return "?"
	}
	raw := []string{}
	for _, l := range p.screen(r.ID, true) {
		if pyStrip(escRE.ReplaceAllString(l, "")) != "" {
			raw = append(raw, l)
		}
	}
	tail := lastN(raw, 20)
	plain := make([]string, len(tail))
	for i, l := range tail {
		plain[i] = strings.ReplaceAll(escRE.ReplaceAllString(l, ""), " ", " ")
	}
	for _, l := range lastN(plain, 12) {
		if dialogRE.MatchString(l) {
			return "prompt"
		}
	}
	for i := len(tail) - 1; i >= 0; i-- {
		s := strings.TrimLeftFunc(plain[i], unicode.IsSpace)
		if strings.HasPrefix(s, "❯") || strings.HasPrefix(s, "›") {
			typed := strings.TrimLeftFunc(strings.ReplaceAll(undimmed(tail[i]), " ", " "), unicode.IsSpace)
			if typed != "" {
				typed = string([]rune(typed)[1:])
			}
			if pyStrip(typed) != "" {
				return "draft"
			}
			break
		}
	}
	for _, l := range lastN(plain, 12) {
		if isWorking(l) {
			return "working"
		}
	}
	return "idle"
}

func lastN[T any](l []T, n int) []T {
	if n <= 0 {
		return l[:0]
	}
	if len(l) > n {
		return l[len(l)-n:]
	}
	return l
}

// --- headless transcripts -----------------------------------------------------

var slugRE = regexp.MustCompile(`[^A-Za-z0-9]`)

// transcript is the Claude session file a headless run is writing.
func (p *P) transcript(r *Row) string {
	if r.Agent != "claude" || r.Cwd == "" {
		return ""
	}
	slug := slugRE.ReplaceAllString(r.Cwd, "-")
	files := glob(filepath.Join(p.home, ".claude/projects", slug, "*.jsonl"))
	since := p.started(r.PID) - 5
	type born struct {
		t    float64
		path string
	}
	list := []born{}
	for _, f := range files {
		if t, ok := birthtime(f); ok && t >= since {
			list = append(list, born{t, f})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].t != list[j].t {
			return list[i].t < list[j].t
		}
		return list[i].path < list[j].path
	})
	if len(list) == 0 {
		return ""
	}
	return list[0].path
}

func (p *P) renderTranscript(path string, n int) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return renderEvents(strings.SplitAfter(string(b), "\n"), n)
}

// renderEvents is the last n lines of a Claude transcript / stream-json log or a
// Codex `exec --json` log, as a pane would read.
func renderEvents(raws []string, n int) []string {
	lines := []string{}
	for _, raw := range raws {
		v, err := Loads([]byte(raw))
		if err != nil {
			continue
		}
		entry, ok := v.(*Obj)
		if !ok {
			continue
		}
		if item := entry.Obj("item"); Equal(entry.Get("type"), "item.completed") && item != nil {
			switch item.Get("type") {
			case "agent_message":
				text := pyStr(item.Get("text"))
				if pyStrip(text) != "" {
					lines = append(lines, splitlines("⏺ "+pyStrip(text))...)
				}
			case "command_execution":
				lines = append(lines, "⏺ $ "+truncRunes(spaceRE.ReplaceAllString(pyStr(item.Get("command")), " "), 120))
				outLines := splitlines(pyStrip(pyStr(item.Get("aggregated_output"))))
				last := ""
				if len(outLines) > 0 {
					last = outLines[len(outLines)-1]
				}
				code := item.Get("exit_code")
				prefix := ""
				if code != nil && !Equal(code, 0) {
					prefix = "exit " + Dumps(code) + ": "
				}
				lines = append(lines, "  ⎿ "+prefix+truncRunes(last, 160))
			}
			continue
		}
		var content any
		if msg := entry.Obj("message"); msg != nil {
			content = msg.Get("content")
		}
		kind := entry.Get("type")
		if s, ok := content.(string); ok {
			content = []any{NewObj("type", "text", "text", s)}
		}
		blocks, _ := content.([]any)
		for _, bv := range blocks {
			block, ok := bv.(*Obj)
			if !ok {
				continue
			}
			switch block.Get("type") {
			case "text":
				text := block.Str("text")
				if pyStrip(text) != "" {
					mark := "⏺ "
					if Equal(kind, "user") {
						mark = "❯ "
					}
					lines = append(lines, splitlines(mark+pyStrip(text))...)
				}
			case "tool_use":
				input := block.Get("input")
				if !block.Has("input") {
					input = NewObj()
				}
				lines = append(lines, fmt.Sprintf("⏺ %s(%s)", pyStr(block.Get("name")), truncRunes(Dumps(input), 120)))
			case "tool_result":
				body := block.Get("content")
				if l, ok := body.([]any); ok {
					parts := []string{}
					for _, b := range l {
						if o, ok := b.(*Obj); ok {
							parts = append(parts, o.Str("text"))
						}
					}
					body = strings.Join(parts, " ")
				}
				text := ""
				if Truthy(body) {
					text = pyStr(body)
				}
				first := ""
				if ls := splitlines(pyStrip(text)); len(ls) > 0 {
					first = ls[0]
				}
				lines = append(lines, "  ⎿ "+truncRunes(first, 160))
			}
		}
	}
	return lastN(lines, n)
}

// pyStr is Python str(v) for JSON values (None prints as "None").
func pyStr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		return x
	case bool:
		if x {
			return "True"
		}
		return "False"
	case float64:
		return FloatRepr(x)
	default:
		return Dumps(x)
	}
}

// --- addressing ---------------------------------------------------------------

func splitHost(target string) (string, string) {
	if strings.Contains(target, "/") && !strings.HasPrefix(target, "/") {
		h, rest, _ := strings.Cut(target, "/")
		return h, rest
	}
	return "", target
}

var paneSuffix = regexp.MustCompile(`:\d+\.\d+$`)

func pyRepr(s string) string {
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"`
	}
	return "'" + strings.NewReplacer(`\`, `\\`, "'", `\'`, "\n", `\n`, "\t", `\t`, "\r", `\r`).Replace(s) + "'"
}

func find(target string, rows []*Row) (*Row, error) {
	var match []*Row
	switch {
	case strings.HasPrefix(target, "%") || strings.HasPrefix(target, "pid:") || strings.HasPrefix(target, "managed:"):
		for _, r := range rows {
			if r.ID == target {
				match = append(match, r)
			}
		}
	case paneSuffix.MatchString(target):
		i := strings.LastIndex(target, ":")
		s, wp := target[:i], target[i+1:]
		w, pn, _ := strings.Cut(wp, ".")
		for _, r := range rows {
			if r.Session == s && r.Window == w && r.Pane == pn {
				match = append(match, r)
			}
		}
	default:
		s, _, _ := strings.Cut(target, ":")
		for _, r := range rows {
			if r.Session == s {
				match = append(match, r)
			}
		}
		agents := []*Row{}
		for _, r := range match {
			if r.Agent != "" {
				agents = append(agents, r)
			}
		}
		if len(agents) > 1 {
			names := []string{}
			for _, r := range agents {
				names = append(names, fmt.Sprintf("%s (%s)", r.ID, r.Agent))
			}
			return nil, refuse("session %s has %d agent panes; pick one: %s", s, len(agents), strings.Join(names, ", "))
		}
		if len(agents) > 0 {
			match = agents
		} else if len(match) > 1 {
			match = match[:1]
		}
	}
	if len(match) == 0 {
		return nil, refuse("no pane matches %s (peers ls to see who's here)", pyRepr(target))
	}
	return match[0], nil
}

func (p *P) resolve(target string, rows []*Row) *Row {
	r, err := find(target, rows)
	if err != nil {
		p.die(err.Error())
	}
	return r
}

// address is the shortest name that reaches exactly this pane.
func (p *P) address(pane *Row, rows []*Row) string {
	same := 0
	for _, r := range rows {
		if r.Session == pane.Session && r.Agent != "" {
			same++
		}
	}
	name := pane.Session
	if same > 1 {
		name = pane.ID
	}
	if p.hostLabel != "" {
		return p.hostLabel + "/" + name
	}
	return name
}

// me is this process's own pane: --from / $PEERS_FROM, a managed runner above
// us, $TMUX_PANE, then the agent pane or run this process descends from.
func (p *P) me(rows []*Row) *Row {
	if os.Getenv("PEERS_NO_SELF") != "" {
		return nil
	}
	if named := os.Getenv("PEERS_FROM"); named != "" {
		row, err := find(named, rows)
		if err == nil && row.Managed != nil && !intIn(p.ps().ancestors(os.Getpid()), row.PID) {
			err = refuse("managed sender requires actual runner ancestry; --from cannot borrow it")
		}
		if err != nil {
			p.die(fmt.Sprintf("--from %s: %s", named, err))
		}
		return row
	}
	up := p.ps().ancestors(os.Getpid())
	for _, r := range rows {
		if r.Managed != nil && intIn(up, r.PID) {
			return r
		}
	}
	if mine, ok := os.LookupEnv("TMUX_PANE"); ok {
		for _, r := range rows {
			if r.ID == mine {
				return r
			}
		}
	}
	for _, r := range rows {
		if r.AgentPID != 0 && intIn(up, r.AgentPID) {
			return r
		}
	}
	for _, r := range rows {
		if intIn(up, r.PID) {
			return r
		}
	}
	return nil
}

const noSender = "can't tell which session you are (no $TMUX_PANE, and no agent pane or run above this process). " +
	"Every message names its sender, so rerun with --from YOUR_SESSION (your row in peers ls) or set PEERS_FROM"

type who struct {
	Address, Agent, Cwd string
	Replyable           bool
}

// whoami refuses rather than send a message nobody can answer or attribute.
func (p *P) whoami(rows []*Row) (who, error) {
	mine := p.me(rows)
	if mine == nil {
		return who{}, refuse(noSender)
	}
	if mine.Managed != nil && p.paneIncarnation(mine) == "" {
		return who{}, refuse("managed sender incarnation changed")
	}
	agent := mine.Agent
	if agent == "" {
		agent = myAgent()
	}
	return who{Address: p.address(mine, rows), Agent: agent, Cwd: p.tilde(mine.Cwd),
		Replyable: mine.Managed != nil || !mine.Headless || mine.Dispatch != ""}, nil
}

func messageHeader(origin, agent, cwd string, replyable bool, note string) string {
	reply := "It is a headless run with no input box, so it can't take a reply"
	if replyable {
		reply = fmt.Sprintf(`Reply with: peers send %s "..."`, origin)
	}
	return fmt.Sprintf("[peer message from %s (%s, %s)%s. %s]", origin, agent, cwd, note, reply)
}

func myAgent() string {
	if os.Getenv("CLAUDECODE") != "" || os.Getenv("CLAUDE_CODE_ENTRYPOINT") != "" {
		return "claude"
	}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "CODEX_") {
			return "codex"
		}
	}
	if v, ok := os.LookupEnv("PEERS_AGENT"); ok {
		return v
	}
	return "agent"
}
