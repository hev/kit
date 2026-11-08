package compat

import (
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// --- remote -------------------------------------------------------------------

// remote runs peers on HOST over ssh. The reference ships its own source and
// runs it under python3; a compiled peers cannot, so this runs the peers the
// remote host has installed ($PEERS_REMOTE_PEERS, default `peers` on the
// remote PATH with ~/.local/bin first). The environment, argv, stdin and the
// hidden _recv/_chat/_chatlog wire formats are the reference's, so either
// implementation answers. Stdin travels raw: the receiving peers reads it as
// its own stdin, exactly as the reference does when PEERS_STDIN_B64 is unset.
func (p *P) remote(host string, argv []string, stdinText string, sender *string) result {
	w := who{Replyable: true}
	known := false
	if sender == nil {
		rows := p.panes(false)
		if me, err := p.whoami(rows); err == nil {
			w, known = me, true
		}
		s := w.Address
		sender = &s
	}
	origin := p.selfLabel()
	if *sender != "" {
		origin = p.selfLabel() + "/" + *sender
	}
	agent := myAgent()
	if known {
		agent = w.Agent
	}
	replyable := ""
	if !known || w.Replyable {
		replyable = "1"
	}
	env := [][2]string{{"PEERS_HOST_LABEL", host}, {"PEERS_REMOTE_FROM", origin}, {"PEERS_REMOTE_AGENT", agent},
		{"PEERS_REMOTE_CWD", w.Cwd}, {"PEERS_REMOTE_REPLYABLE", replyable}}
	parts := []string{}
	for _, kv := range env {
		parts = append(parts, kv[0]+"="+shlexQuote(kv[1]))
	}
	bin := os.Getenv("PEERS_REMOTE_PEERS")
	if bin == "" {
		bin = "peers"
	}
	cmd := strings.Join(parts, " ") + " PATH=$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:$PATH " + shlexQuote(bin)
	for _, a := range argv {
		cmd += " " + shlexQuote(a)
	}
	return p.run([]string{"ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", host, cmd}, &stdinText)
}

func empty() *string { s := ""; return &s }

func (p *P) runRemoteOrExit(target string, argv []string, stdinText string) string {
	host, rest := splitHost(target)
	if host == "" {
		return rest
	}
	for i, a := range argv {
		if a == target {
			argv = append(append(append([]string{}, argv[:i]...), rest), argv[i+1:]...)
			break
		}
	}
	if p.snapshot(host) != nil && !contains(p.peerHosts(), host) {
		// A machine that pushes to us but that we can't ssh to.
		switch argv[0] {
		case "read":
			p.snapshotRead(host, argv[1:])
		case "send":
			p.snapshotSend(host, argv[1:], stdinText)
		case "wait":
			p.snapshotWait(host, argv[1:])
		}
		p.exit(0)
	}
	r := p.remote(host, argv, stdinText, nil)
	fmt.Fprint(p.out, r.stdout)
	fmt.Fprint(p.err, strings.ReplaceAll(r.stderr, "peers:", "peers["+host+"]:"))
	p.exit(r.code)
	return ""
}

func (p *P) readStdinArg(value string) string {
	if value != "-" {
		return value
	}
	if p.stdinB64 != nil {
		b, err := base64.StdEncoding.DecodeString(*p.stdinB64)
		if err != nil {
			panic(err)
		}
		return string(b)
	}
	b, err := readAll(p.in)
	if err != nil {
		panic(err)
	}
	return b
}

// --- commands -----------------------------------------------------------------

func ago(epoch any) string {
	f, ok := num(epoch)
	if !ok || f == 0 {
		return ""
	}
	s := int64(math.Floor(now())) - int64(f)
	if s < 0 {
		s = 0
	}
	for _, u := range []struct {
		unit string
		n    int64
	}{{"d", 86400}, {"h", 3600}, {"m", 60}} {
		if s >= u.n {
			return fmt.Sprintf("%d%s", s/u.n, u.unit)
		}
	}
	return fmt.Sprintf("%ds", s)
}

func (p *P) branch(cwd string) string {
	if cwd == "" {
		return ""
	}
	r := p.run([]string{"git", "-C", cwd, "branch", "--show-current"}, nil)
	if r.code != 0 {
		return ""
	}
	return strings.TrimSpace(r.stdout)
}

var titleLead = regexp.MustCompile(`^[^\p{L}\p{N}\p{Mn}_\s\v\p{Z}]\s*`)

func (p *P) cleanTitle(r *Row) string {
	title := pyStrip(titleLead.ReplaceAllString(r.Title, ""))
	head := strings.Split(strings.ToLower(title), ".")[0]
	if head == p.hostname() || head == p.selfLabel() || head == strings.ToLower(p.hostLabel) || head == "" || title == r.Cwd {
		return ""
	}
	return title
}

func kind(r *Row) string {
	switch {
	case r.Managed != nil:
		return "managed, inbox"
	case r.Dispatch != "":
		return "dispatch " + r.Dispatch
	case r.Headless:
		return "headless, no input"
	}
	return ""
}

func (p *P) inboundNotes() map[string]string {
	notes := map[string]string{}
	rs := recent(p.sentLog, recentWindow)
	for i := len(rs) - 1; i >= 0; i-- {
		rec := rs[i]
		to, ok := rec.Get("to").(string)
		if !ok {
			continue
		}
		if _, seen := notes[to]; !seen {
			notes[to] = fmt.Sprintf("← %s %s ago", pyStr(rec.Get("from")), ago(rec.Get("ts")))
		}
	}
	return notes
}

func (p *P) sshClients() map[string]bool {
	ps := p.ps()
	out := map[string]bool{}
	for _, line := range splitlines(p.tmuxQ("list-clients", "-F", "#{client_pid}"+sep+"#{client_session}")) {
		pid, session, _ := strings.Cut(line, sep)
		if !isDigits(pid) {
			continue
		}
		n, _ := strconv.Atoi(pid)
		for _, a := range ps.ancestors(n) {
			if strings.Contains(ps.cmd[a], "sshd") {
				out[session] = true
				break
			}
		}
	}
	return out
}

func pad(s string, width int) string { return fmt.Sprintf("%-*s", width, s) }

func (p *P) printRow(width int, addr, agent, st, where, title, flags string, notes []string, you string) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s  %s  %s  %s", pad(addr, width), pad(agent, 6), pad(st, 7), where))
	blank := fmt.Sprintf("\n%s  %s  %s  ", pad("", width), pad("", 6), pad("", 7))
	if title != "" {
		b.WriteString(blank + "“" + title + "”")
	}
	if pyStrip(flags) != "" {
		b.WriteString("  (" + pyStrip(flags) + ")")
	}
	b.WriteString(you)
	for _, n := range notes {
		b.WriteString(blank + n)
	}
	fmt.Fprintln(p.out, b.String())
}

func (p *P) sessionID(r *Row) any {
	if r.Managed != nil {
		return r.Managed.Obj("checkpoint").Get("session")
	}
	if r.Agent != "claude" || r.AgentPID == 0 {
		return nil
	}
	o, err := readObj(filepath.Join(p.home, ".claude/sessions", strconv.Itoa(r.AgentPID)+".json"))
	if err != nil {
		return nil
	}
	return o.Get("sessionId")
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func orNone(v any) any {
	if !Truthy(v) {
		return nil
	}
	return v
}

func (p *P) agentStarted(pid int) any {
	if pid == 0 {
		return nil
	}
	return int(math.RoundToEven(p.started(pid)))
}

func (p *P) inboxStatus(r *Row) (bool, *Obj) {
	if (r.Managed != nil || !r.Headless) && p.inboxOn(r.Session) {
		expected := r.TmuxIdentity
		if r.Managed != nil {
			expected = r.Managed.Str("incarnation")
		}
		return true, p.inboxSummary(r.Session, expected)
	}
	return false, nil
}

// lsJSON is the same inventory for programs; version bumps on any incompatible change.
func (p *P) lsJSON(rows []*Row, showAll, everywhere bool) *Obj {
	mine := p.me(rows)
	host := p.hostLabel
	if host == "" {
		host = p.selfLabel()
	}
	out := []any{}
	for _, r := range rows {
		if !participates(r.Role) || !(r.Agent != "" || showAll) {
			continue
		}
		addr := p.address(r, rows)
		if i := strings.Index(addr, "/"); i >= 0 {
			addr = addr[i+1:]
		}
		on, summary := p.inboxStatus(r)
		var status any
		if on {
			status = summary
		}
		out = append(out, NewObj(
			"address", host+"/"+addr, "pane", r.ID, "session", r.Session, "agent", nullable(r.Agent), "agent_pid", nullInt(r.AgentPID),
			"agent_started", p.agentStarted(r.AgentPID), "session_id", p.sessionID(r), "state", p.state(r), "cwd", r.Cwd,
			"branch", p.branch(r.Cwd), "title", p.cleanTitle(r), "dispatch", nullable(r.Dispatch), "headless", r.Headless,
			"role", peerRole(r.Role), "tmux_identity", nullable(r.TmuxIdentity), "recipient_incarnation", nullable(p.paneIncarnation(r)),
			"inbox_status", status, "attached", r.Attached, "inbox", r.Agent != "" && on, "activity", orNone(r.Activity),
			"you", mine != nil && r.ID == mine.ID))
	}
	doc := NewObj("version", 1, "host", host, "at", now(), "panes", out, "hosts", []any{})
	if p.hostLabel != "" {
		return doc
	}
	hosts := []any{}
	if everywhere {
		for _, h := range p.peerHosts() {
			args := []string{"ls", "--json"}
			if showAll {
				args = append(args, "-a")
			}
			r := p.remote(h, args, "", empty())
			remote, err := LoadObj([]byte(r.stdout))
			if err == nil && remote.Has("at") && remote.Has("panes") {
				hosts = append(hosts, NewObj("host", h, "source", "ssh", "at", remote.Get("at"), "stale", false, "panes", remote.Get("panes")))
			} else {
				hosts = append(hosts, NewObj("host", h, "source", "ssh", "error", lastLine(r.stderr, "ssh failed")))
			}
		}
	}
	for _, snap := range p.snapshotList() {
		if everywhere && contains(p.peerHosts(), snap.Str("host")) {
			continue
		}
		panesOut := []any{}
		for _, v := range snap.List("panes") {
			sp := v.(*Obj)
			branch := sp.Get("branch")
			if !sp.Has("branch") {
				branch = ""
			}
			title := sp.Get("title")
			if !sp.Has("title") {
				title = ""
			}
			k := sp.Get("kind")
			if !sp.Has("kind") {
				k = ""
			}
			panesOut = append(panesOut, NewObj(
				"address", snap.Str("host")+"/"+pyStr(sp.Get("address")), "pane", sp.Get("id"), "agent", sp.Get("agent"),
				"agent_pid", sp.Get("agent_pid"), "agent_started", sp.Get("agent_started"), "session_id", sp.Get("session_id"),
				"state", sp.Get("state"), "role", peerRole(sp.Str("role")), "cwd", sp.Get("cwd"), "branch", branch, "title", title,
				"kind", k, "attached", sp.Get("attached"), "activity", orNone(sp.Get("activity"))))
		}
		at, _ := snap.Num("at")
		hosts = append(hosts, NewObj("host", snap.Get("host"), "source", "snapshot", "at", snap.Get("at"),
			"stale", now()-at > stale, "panes", panesOut))
	}
	doc.Set("hosts", hosts)
	return doc
}

func lastLine(s, fallback string) string {
	lines := splitlines(pyStrip(s))
	if len(lines) == 0 {
		return fallback
	}
	return lines[len(lines)-1]
}

func (p *P) cmdLs(args []string) {
	showAll := contains(args, "-a") || contains(args, "--all")
	everywhere := contains(args, "--everywhere") || contains(args, "-e")
	rows := p.panes(false)
	if contains(args, "--json") {
		fmt.Fprintln(p.out, Dumps(p.lsJSON(rows, showAll, everywhere), 1))
		return
	}
	mine := p.me(rows)
	listed := []*Row{}
	for _, r := range rows {
		if r.Agent != "" || showAll {
			listed = append(listed, r)
		}
	}
	if len(listed) == 0 {
		host := p.hostLabel
		if host == "" {
			host = p.selfLabel()
		}
		suffix := " (-a for every pane)"
		if showAll {
			suffix = ""
		}
		fmt.Fprintln(p.out, host+": no agent panes"+suffix)
	}
	notes := p.inboundNotes()
	overSSH := p.sshClients()
	width := 7
	for _, r := range listed {
		if n := len([]rune(p.address(r, rows))); n > width {
			width = n
		}
	}
	for _, r := range listed {
		where := p.tilde(r.Cwd)
		if b := p.branch(r.Cwd); b != "" {
			where = fmt.Sprintf("%s [%s]", p.tilde(r.Cwd), b)
		}
		attached := ""
		if overSSH[r.Session] {
			attached = "attached over ssh "
		} else if r.Attached {
			attached = "attached "
		}
		inbox := ""
		if r.Agent != "" {
			if on, summary := p.inboxStatus(r); on {
				inbox = fmt.Sprintf("inbox: %s unread, %s awaiting ACK", pyStr(summary.Get("unread")), pyStr(summary.Get("unacknowledged")))
			}
		}
		activity := ""
		if Truthy(r.Activity) {
			activity = ago(r.Activity) + " ago"
		}
		flags := joinNonEmpty(", ", strings.ToUpper(peerRole(r.Role)), kind(r), inbox, pyStrip(attached+activity))
		addr := p.address(r, rows)
		short := addr
		if i := strings.Index(addr, "/"); i >= 0 {
			short = addr[i+1:]
		}
		extra := []string{}
		if n, ok := notes[short]; ok {
			extra = append(extra, n)
		}
		you := ""
		if mine != nil && r.ID == mine.ID {
			you = "  ← you"
		}
		agent := r.Agent
		if agent == "" {
			agent = "shell"
		}
		p.printRow(width, addr, agent, p.state(r), where, p.cleanTitle(r), flags, extra, you)
	}
	if everywhere && p.hostLabel == "" {
		for _, host := range p.peerHosts() {
			args := []string{"ls"}
			if showAll {
				args = append(args, "-a")
			}
			sender := ""
			if mine != nil {
				sender = p.address(mine, rows)
			}
			r := p.remote(host, args, "", &sender)
			if r.code == 0 {
				fmt.Fprint(p.out, r.stdout)
			} else {
				fmt.Fprintf(p.out, "%s/: unreachable (%s)\n", host, lastLine(r.stderr, "ssh failed"))
			}
		}
	}
	if p.hostLabel == "" {
		for _, snap := range p.snapshotList() {
			if contains(p.peerHosts(), snap.Str("host")) && everywhere {
				continue // already listed live
			}
			p.printSnapshot(snap)
		}
	}
}

func joinNonEmpty(sep string, parts ...string) string {
	kept := []string{}
	for _, x := range parts {
		if x != "" {
			kept = append(kept, x)
		}
	}
	return strings.Join(kept, sep)
}

func (p *P) printSnapshot(snap *Obj) {
	host := snap.Str("host")
	at, _ := snap.Num("at")
	panes := snap.List("panes")
	if now()-at > stale {
		fmt.Fprintf(p.out, "%s/: last checked in %s ago (asleep or offline); %d agents then. Messages to it wait until it's back\n",
			host, ago(snap.Get("at")), len(panes))
		return
	}
	fmt.Fprintf(p.out, "%s/  (pushed %s ago; can't be reached directly, sends are relayed within a minute)\n", host, ago(snap.Get("at")))
	if len(panes) == 0 {
		fmt.Fprintf(p.out, "%s/: no agent panes\n", host)
	}
	width := 7
	for _, v := range panes {
		if n := len([]rune(host)) + 1 + len([]rune(pyStr(v.(*Obj).Get("address")))); n > width {
			width = n
		}
	}
	for _, v := range panes {
		sp := v.(*Obj)
		where := pyStr(sp.Get("cwd"))
		if Truthy(sp.Get("branch")) {
			where = fmt.Sprintf("%s [%s]", where, pyStr(sp.Get("branch")))
		}
		attached := ""
		if Truthy(sp.Get("attached")) {
			attached = "attached "
		}
		if Truthy(sp.Get("activity")) {
			attached += ago(sp.Get("activity")) + " ago"
		}
		parts := []string{}
		for _, x := range []string{strings.ToUpper(peerRole(sp.Str("role"))), sp.Str("kind"), attached} {
			if pyStrip(x) != "" {
				parts = append(parts, x)
			}
		}
		notes := []string{}
		for _, l := range sp.List("links") {
			lo := l.(*Obj)
			notes = append(notes, fmt.Sprintf("→ %s: %s", pyStr(lo.Get("host")), pyStr(lo.Get("cmd"))))
		}
		for _, s := range sp.List("sent") {
			so := s.(*Obj)
			notes = append(notes, fmt.Sprintf("→ messaged %s/%s %s ago", p.selfLabel(), pyStr(so.Get("to")), ago(so.Get("ts"))))
		}
		p.printRow(width, host+"/"+pyStr(sp.Get("address")), pyStr(sp.Get("agent")), pyStr(sp.Get("state")), where,
			sp.Str("title"), strings.Join(parts, ", "), notes, "")
	}
	for _, v := range snap.List("relayed") {
		rel := v.(*Obj)
		if Equal(rel.Get("from_host"), p.selfLabel()) {
			fmt.Fprintf(p.out, "  relay %s → %s/%s: %s %s ago\n", p.qualify(pyStr(rel.Get("from"))), host, pyStr(rel.Get("to")),
				pyStr(rel.Get("status")), ago(rel.Get("ts")))
		}
	}
}

// takeInt pops `flag N` like the reference's int(args[i + 1]).
func (p *P) takeInt(args *[]string, flag string, def int) int {
	for i, a := range *args {
		if a == flag {
			if i+1 >= len(*args) {
				p.die("list index out of range")
			}
			n, err := strconv.Atoi(strings.TrimSpace((*args)[i+1]))
			if err != nil {
				p.die(fmt.Sprintf("invalid literal for int() with base 10: %s", pyRepr((*args)[i+1])))
			}
			*args = append(append([]string{}, (*args)[:i]...), (*args)[i+2:]...)
			return n
		}
	}
	return def
}

func (p *P) cmdRead(args []string) {
	n := p.takeInt(&args, "-n", 60)
	if len(args) == 0 {
		p.die("read TARGET [-n LINES]")
	}
	target := p.runRemoteOrExit(args[0], []string{"read", args[0], "-n", strconv.Itoa(n)}, "")
	pane := p.resolve(target, p.panes(false))
	if pane.Managed != nil {
		fmt.Fprintln(p.out, strings.Join(lastN(p.managedScreen(pane), n), "\n"))
		return
	}
	if strings.HasPrefix(pane.ID, "pid:") {
		path := p.transcript(pane)
		if path == "" {
			p.die(fmt.Sprintf("%s is a headless %s run with no transcript peers can find; its prompt: %s", target, pane.Agent, pane.Title))
		}
		fmt.Fprintln(p.out, strings.Join(p.renderTranscript(path, n), "\n"))
		return
	}
	if pane.Dispatch != "" && which("hev") {
		// A dispatch run's pane holds only the supervisor; its worker writes to the run log.
		r := p.run([]string{"hev", "dispatch", "logs", pane.Dispatch}, nil)
		if r.code == 0 {
			if lines := renderEvents(splitlines(r.stdout), n); len(lines) > 0 {
				fmt.Fprintln(p.out, strings.Join(lines, "\n"))
				return
			}
		}
	}
	lines := splitlines(p.tmux("capture-pane", "-p", "-J", "-S", fmt.Sprintf("-%d", n+200), "-t", pane.ID))
	for len(lines) > 0 && pyStrip(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	fmt.Fprintln(p.out, strings.Join(lastN(lines, n), "\n"))
}

func (p *P) cmdRole(args []string) {
	usage := "role [SESSION] [foh|boh|offline]"
	if len(args) > 2 {
		p.die(usage)
	}
	rows := p.panes(true)
	role := ""
	targetArgs := args
	if len(args) > 0 && contains(peerRoles, args[len(args)-1]) {
		role = args[len(args)-1]
		targetArgs = args[:len(args)-1]
	}
	if len(targetArgs) > 1 {
		p.die(usage)
	}
	var pane *Row
	if len(targetArgs) == 1 {
		if h, _ := splitHost(targetArgs[0]); h != "" {
			p.die("role changes are local; run peers role on the owning host")
		}
		pane = p.resolve(targetArgs[0], rows)
	} else {
		pane = p.me(rows)
	}
	if pane == nil {
		p.die(noSender)
	}
	if role != "" {
		if pane.Dispatch != "" || pane.Headless {
			p.die("managed/headless work uses its owner lifecycle; roles here are for interactive tmux")
		}
		p.tmux("set-option", "-t", "="+pane.Session+":", "@peers-role", role)
	}
	shown := role
	if shown == "" {
		shown = peerRole(pane.Role)
	}
	fmt.Fprintf(p.out, "%s: %s\n", pane.Session, shown)
}

func (p *P) cmdManaged(args []string) {
	if (len(args) != 4 && len(args) != 5) || args[0] != "register" || args[2] != "--root" || (len(args) == 5 && args[4] != "--replace") {
		p.die("managed register NAME --root ABS [--replace]")
	}
	name, root := args[1], args[3]
	if err := p.managedRegister(name, root, contains(args, "--replace")); err != nil {
		p.die(err.Error())
	}
}

func (p *P) managedRegister(name, root string, replace bool) error {
	path, err := p.managedPath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	unlock, err := lockPath(filepath.Join(filepath.Dir(path), ".register.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	if !filepath.IsAbs(root) || realpath(root) != root {
		return refuse("managed root must be an absolute resolved directory")
	}
	state, err := readObj(filepath.Join(root, "state.json"))
	if err != nil {
		return err
	}
	if !state.Has("pid") {
		return fmt.Errorf("'pid'")
	}
	pid, ok := intOf(state.Get("pid"))
	if !ok {
		return refuse("state PID is not the live runner for this root")
	}
	birth, command := p.started(pid), p.ps().cmd[pid]
	words, ok := shlexSplit(command)
	if !ok {
		return fmt.Errorf("No closing quotation")
	}
	if birth == 0 || len(words) < 3 || words[len(words)-2] != filepath.Join(root, "runner.py") || words[len(words)-1] != root {
		return refuse("state PID is not the live runner for this root")
	}
	if Equal(state.Get("phase"), "stopped") {
		return refuse("managed runner is stopped")
	}
	// The account owner is the operator boundary, as with tmux registration.
	if !ownedByMe(root) {
		return refuse("managed root belongs to another account")
	}
	for _, r := range p.panes(true) {
		if r.Session == name && r.Managed == nil {
			return refuse("name already belongs to an interactive or native worker")
		}
	}
	for _, other := range p.managedRows() {
		b := other.Managed
		if b.Str("name") != name && Equal(b.Get("pid"), pid) && Equal(b.Get("birth"), birth) {
			return refuse("runner lifetime already registered as %s", b.Str("name"))
		}
	}
	old := p.managedRecord(name)
	if old != nil && old.Str("root") == root && Equal(old.Get("pid"), pid) && Equal(old.Get("birth"), birth) {
		shown := old.Copy()
		shown.Del("checkpoint")
		fmt.Fprintln(p.out, Dumps(shown))
		return nil
	}
	_, statErr := os.Stat(path)
	exists := statErr == nil
	if exists && !replace {
		return refuse("retained registration exists; inspect it before explicit --replace")
	}
	if old != nil {
		return refuse("previous managed runner is still live; cannot replace it")
	}
	rec := NewObj("version", 1, "name", name, "root", root, "pid", pid, "birth", birth, "command", command,
		"uid", os.Getuid(), "incarnation", uuidHex(), "registered_at", now())
	if exists {
		prior, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		v, err := Loads(prior)
		if err != nil {
			return err
		}
		pv, ok := v.(*Obj)
		if !ok {
			return fmt.Errorf("prior registration is not an object")
		}
		inboxAppend(path+".history.jsonl", pv)
	}
	if err := writeJSON(path, rec); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	fmt.Fprintln(p.out, Dumps(rec))
	return nil
}
