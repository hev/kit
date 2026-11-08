package compat

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (p *P) deliver(paneID, text string) {
	buf := fmt.Sprintf("peers-%d", os.Getpid())
	p.tmuxIn(&text, true, "load-buffer", "-b", buf, "-")
	p.tmux("paste-buffer", "-p", "-d", "-b", buf, "-t", paneID)
	p.sleep(600 * time.Millisecond) // let the harness finish ingesting the paste before Enter
	p.tmux("send-keys", "-t", paneID, "Enter")
}

// sendTo delivers to a pane on this machine, or returns Refused.
func (p *P) sendTo(target string, rows []*Row, header, message, sender string, force bool, extra *Obj) (*Row, string, error) {
	if extra == nil {
		extra = NewObj()
	}
	pane, err := find(target, rows)
	if err != nil {
		return nil, "", err
	}
	if mine := p.me(rows); mine != nil && pane.ID == mine.ID {
		return nil, "", refuse("that's your own pane")
	}
	if pane.Agent == "" {
		return nil, "", refuse("%s is a plain shell, not an agent; use tmux send-keys if you really mean to type there", target)
	}
	if pane.Agent == "pi" && (pane.Dispatch != "" || pane.Headless || !p.inboxOn(pane.Session)) {
		return nil, "", refuse("%s runs PI; peers requires a registered inbox session and never types into PI (including --force)", target)
	}
	if !participates(pane.Role) {
		return nil, "", refuse("recipient is offline")
	}
	// Recheck current role: a beacon's inventory can predate an offline switch.
	role := "boh"
	if !pane.Headless {
		role = strings.TrimSpace(p.tmuxQ("show-options", "-t", "="+pane.Session+":", "-qv", "@peers-role"))
	}
	if role != "" && role != "foh" && role != "boh" {
		return nil, "", refuse("recipient is offline")
	}
	if Truthy(extra.Get("recipient_incarnation")) {
		actual := p.paneIncarnation(pane)
		if actual == "" {
			return nil, "", refuse("cannot establish recipient incarnation")
		}
		if !Equal(actual, extra.Get("recipient_incarnation")) {
			return nil, "", refuse("recipient incarnation changed")
		}
	}
	st := p.state(pane)
	text := header + "\n" + message
	registered := pane.Managed != nil || (pane.Dispatch == "" && !pane.Headless && p.inboxOn(pane.Session))
	key := extra.Get("id")
	previousHold := false
	if Truthy(key) {
		for _, m := range inboxRows(p.inboxPath(pane.Session)) {
			if Equal(m.Get("id"), key) {
				previousHold = true
				break
			}
		}
	}
	if Truthy(extra.Get("inbox_only")) && !registered && !previousHold {
		return nil, "", refuse("recipient inbox is no longer registered; queued invitation was not pasted")
	}
	switch {
	case registered || previousHold:
		binding := p.paneIncarnation(pane)
		if binding == "" {
			return nil, "", refuse("cannot establish recipient incarnation")
		}
		extra = NewObj("recipient_incarnation", binding).Merge(extra)
		held, err := p.hold(pane.Session, header, message, sender, extra)
		if err != nil {
			return nil, "", err
		}
		extra = extra.Merge(NewObj("id", held.Get("id"), "recipient_incarnation", held.Get("recipient_incarnation"), "transport_state", "queued"))
		var receipt *Obj
		for _, m := range p.inboxView(pane.Session, "").messages {
			if Equal(m.Get("id"), held.Get("id")) {
				receipt = m
				break
			}
		}
		if receipt == nil {
			panic("StopIteration")
		}
		cp := *pane
		cp.inboxReceipt = receipt
		pane = &cp
		st = "inbox"
	case pane.Dispatch != "":
		if !which("hev") {
			return nil, "", refuse("%s is hev dispatch run %s, but hev isn't on PATH here", target, pane.Dispatch)
		}
		r := p.run([]string{"hev", "dispatch", "say", "--", pane.Dispatch, text}, nil)
		if r.code != 0 {
			msg := r.stderr
			if msg == "" {
				msg = r.stdout
			}
			return nil, "", refuse("hev dispatch say %s failed: %s", pane.Dispatch, pyStrip(msg))
		}
	case pane.Headless:
		return nil, "", refuse("%s is a headless %s run (one prompt, no input box); it can't be steered while it runs. peers read %s to follow it",
			target, pane.Agent, target)
	default:
		if st == "prompt" {
			return nil, "", refuse("%s is showing a menu or permission dialog; typing would answer it. peers read %s and tell the user instead", target, target)
		}
		if st == "draft" && !force {
			return nil, "", refuse("%s has unsent text in its input box (a human is probably typing). Try again later; --force appends to their draft and submits it", target)
		}
		if st == "?" && !force {
			return nil, "", refuse("%s runs %s, whose screen peers can't read; --force to send anyway", target, pane.Agent)
		}
		p.deliver(pane.ID, text)
	}
	to := p.address(pane, rows)
	if i := strings.Index(to, "/"); i >= 0 {
		to = to[i+1:]
	}
	appendLine(p.sentLog, NewObj("ts", now(), "host", p.selfLabel(), "from", sender, "to", to, "message", message).Merge(extra))
	return pane, st, nil
}

func heldStatus(pane *Row) string {
	r := pane.inboxReceipt
	if r.Get("acknowledged_at") != nil {
		return "held in inbox; recipient acknowledgment recorded"
	}
	if r.Get("consumed_at") != nil {
		return "held in inbox; consumed, awaiting recipient acknowledgment"
	}
	return "held unread in inbox; recipient acknowledgment is not yet recorded"
}

func (p *P) cmdChefNotify([]string) {
	p.die("chef-notify retired with the factory CLI (EOL 2026-11-02). Workers report " +
		"through their hev dispatch receipt, or `peers send` to their FOH.")
}

// relayAlias: a bare session name with no pane here resolves to the one machine
// whose pushed snapshot has it. Ambiguous names must be qualified.
func (p *P) relayAlias(target string, rows []*Row) string {
	if h, _ := splitHost(target); h != "" || os.Getenv("PEERS_REMOTE_FROM") != "" || strings.Contains(target, ":") ||
		strings.HasPrefix(target, "%") || strings.HasPrefix(target, "pid:") || strings.HasPrefix(target, "managed:") {
		return target
	}
	if _, err := find(target, rows); err == nil {
		return target
	}
	hits := []string{}
	for _, snap := range p.snapshotList() {
		if !Truthy(snap.Get("host")) {
			continue
		}
		for _, r := range snap.List("panes") {
			if o, ok := r.(*Obj); ok && Equal(o.Get("address"), target) {
				hits = append(hits, snap.Str("host"))
				break
			}
		}
	}
	if len(hits) > 1 {
		names := []string{}
		for _, h := range hits {
			names = append(names, h+"/"+target)
		}
		p.die(fmt.Sprintf("%s is on several machines; pick one: %s", pyRepr(target), strings.Join(names, ", ")))
	}
	if len(hits) == 1 {
		return hits[0] + "/" + target
	}
	return target
}

var stableID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,160}$`)

func (p *P) cmdSend(args []string) {
	metadata := NewObj()
	forwarded := []string{}
	for _, fk := range [][2]string{{"--id", "id"}, {"--order", "order"}, {"--run", "run"}} {
		for i, a := range args {
			if a != fk[0] {
				continue
			}
			if i+1 >= len(args) || !stableID.MatchString(args[i+1]) {
				p.die(fk[0] + " needs a stable identifier (letters, digits, . _ : -)")
			}
			metadata.Set(fk[1], args[i+1])
			forwarded = append(forwarded, args[i:i+2]...)
			args = append(append([]string{}, args[:i]...), args[i+2:]...)
			break
		}
	}
	force := contains(args, "--force")
	kept := []string{}
	for _, a := range args {
		if a != "--force" {
			kept = append(kept, a)
		}
	}
	args = kept
	if len(args) < 2 {
		p.die("send TARGET MESSAGE   (or - to read the message from stdin)")
	}
	message := pyStrip(p.readStdinArg(args[1]))
	if message == "" {
		p.die("empty message")
	}
	remoteFrom := os.Getenv("PEERS_REMOTE_FROM")
	args[0] = p.relayAlias(args[0], p.panes(false))
	if h, _ := splitHost(args[0]); remoteFrom == "" && h != "" {
		if _, err := p.whoami(p.panes(false)); err != nil { // refuse here, before it leaves the machine
			p.die(err.Error())
		}
	}
	remoteArgs := []string{"send", args[0], "-"}
	if force {
		remoteArgs = append(remoteArgs, "--force")
	}
	target := p.runRemoteOrExit(args[0], append(remoteArgs, forwarded...), message)
	rows := p.panes(false)
	var w who
	note := ""
	if remoteFrom != "" {
		w = who{Address: remoteFrom, Agent: os.Getenv("PEERS_REMOTE_AGENT"), Cwd: os.Getenv("PEERS_REMOTE_CWD"),
			Replyable: os.Getenv("PEERS_REMOTE_REPLYABLE") != ""}
		if _, ok := os.LookupEnv("PEERS_REMOTE_AGENT"); !ok {
			w.Agent = "agent"
		}
		host, _, _ := strings.Cut(remoteFrom, "/")
		note = ", on another machine; this one can't ssh to " + host + ", so a reply is queued " +
			"and delivered when it next checks in, within a minute while it's awake"
	} else {
		var err error
		if w, err = p.whoami(rows); err != nil {
			p.die(err.Error())
		}
	}
	header := messageHeader(w.Address, w.Agent, w.Cwd, w.Replyable, note)
	extra := NewObj("from_agent", w.Agent, "from_cwd", w.Cwd, "id", uuidHex()).Merge(metadata)
	pane, st, err := p.sendTo(target, rows, header, message, w.Address, force, extra)
	if err != nil {
		p.die(err.Error())
	}
	how := ""
	switch {
	case st == "inbox":
		how = "; " + heldStatus(pane) + " (don't resend)"
	case pane.Dispatch != "":
		how = "; hev dispatch queued it in the run's inbox for its next check"
	case st == "working":
		how = "; it is mid-turn, so the message waits for its next turn or tool boundary"
	}
	verb, was := "sent to", ", was "+st
	if st == "inbox" {
		verb, was = "queued for", ""
	}
	fmt.Fprintf(p.out, "%s %s (%s%s) [id %s]%s\n", verb, p.address(pane, rows), pane.Agent, was, pyStr(extra.Get("id")), how)
}

func (p *P) cmdWait(args []string) {
	timeout := p.takeInt(&args, "--timeout", 600)
	if len(args) == 0 {
		p.die("wait TARGET [--timeout SECONDS]")
	}
	target := p.runRemoteOrExit(args[0], []string{"wait", args[0], "--timeout", strconv.Itoa(timeout)}, "")
	deadline := now() + float64(timeout)
	p.sleep(3 * time.Second) // a just-sent prompt takes a moment to show its spinner
	for {
		p.procs = nil
		rows := p.panes(false)
		pane, err := find(target, rows)
		if err != nil {
			if strings.HasPrefix(target, "pid:") {
				fmt.Fprintf(p.out, "%s: finished\n", target)
				return
			}
			p.die(err.Error())
		}
		st := p.state(pane)
		if st != "working" {
			fmt.Fprintf(p.out, "%s: %s\n", target, st)
			return
		}
		if now() > deadline {
			fmt.Fprintf(p.out, "%s: still working after %ds\n", target, timeout)
			p.exit(2)
		}
		p.sleep(3 * time.Second)
	}
}

// --- a machine that can't be reached: snapshots and relayed messages -----------

func (p *P) snapshotPane(target, host string) (*Obj, *Obj) {
	snap := p.snapshot(host)
	at, _ := snap.Num("at")
	if now()-at > stale {
		p.die(fmt.Sprintf("%s last checked in %s ago (asleep or offline)", host, ago(snap.Get("at"))))
	}
	name, _, _ := strings.Cut(target, ":")
	for _, v := range snap.List("panes") {
		sp := v.(*Obj)
		if Equal(sp.Get("address"), target) || Equal(sp.Get("address"), name) || Equal(sp.Get("id"), target) {
			return snap, sp
		}
	}
	p.die(fmt.Sprintf("no pane on %s matches %s as of %s ago (peers ls to see who's there)", host, pyRepr(target), ago(snap.Get("at"))))
	return nil, nil
}

func argAfter(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", true
		}
	}
	return "", false
}

func (p *P) snapshotRead(host string, args []string) {
	n := 60
	if v, ok := argAfter(args, "-n"); ok {
		n, _ = strconv.Atoi(v)
	}
	snap, sp := p.snapshotPane(args[0], host)
	lines := []string{}
	for _, l := range sp.List("screen") {
		lines = append(lines, pyStr(l))
	}
	fmt.Fprintln(p.out, strings.Join(lastN(lines, n), "\n"))
	fmt.Fprintf(p.out, "\n[%s/%s as of %s ago, %s; at most %d lines are pushed]\n", host, pyStr(sp.Get("address")), ago(snap.Get("at")),
		pyStr(sp.Get("state")), snapshotLines)
}

func relayFingerprint(msg *Obj) string {
	payload := map[string]any{}
	for _, k := range []string{"from_host", "from", "from_agent", "from_cwd", "to_host", "to", "recipient_incarnation", "message", "order", "run"} {
		payload[k] = msg.Get(k)
	}
	thread := msg.Get("thread")
	if t, ok := thread.(*Obj); ok {
		thread = t.Get("id")
	}
	payload["thread"] = thread
	return sha256Hex(Dumps(payload))
}

func sameKey(a, b *Obj, keys ...string) bool {
	for _, k := range keys {
		if !Equal(a.Get(k), b.Get(k)) {
			return false
		}
	}
	return true
}

// queueRelay: a retry key cannot be repurposed after the outbox copy was fetched.
func (p *P) queueRelay(record *Obj) (*Obj, error) {
	if !Truthy(record.Get("recipient_incarnation")) {
		return nil, refuse("recipient snapshot has no incarnation binding; refresh it before sending")
	}
	os.MkdirAll(p.stateDir, 0o755)
	unlock := lockFile(p.queueLog + ".lock")
	defer unlock()
	journal := inboxRows(p.queueLog)
	prior := append([]*Obj{}, journal...)
	for _, path := range glob(filepath.Join(p.outbox, "*.json")) {
		if o, err := readObj(path); err == nil {
			prior = append(prior, o)
		}
	}
	for _, old := range prior {
		if sameKey(old, record, "from_host", "id") {
			if relayFingerprint(old) != relayFingerprint(record) {
				return nil, refuse("relay message ID reused with different content or recipient")
			}
			record = old
			break
		}
	}
	logged := false
	for _, r := range journal {
		if sameKey(r, record, "from_host", "id") {
			logged = true
			break
		}
	}
	if !logged {
		inboxAppend(p.queueLog, record)
	}
	key := sha256Hex(pyStr(record.Get("from_host")) + ":" + pyStr(record.Get("id")))
	if err := writeJSON(filepath.Join(p.outbox, key+".json"), record); err != nil {
		panic(err)
	}
	return record, nil
}

func (p *P) snapshotSend(host string, args []string, message string) {
	force := contains(args, "--force")
	target := args[0]
	message = pyStrip(message)
	if message == "" {
		p.die("empty message")
	}
	snap, sp := p.snapshotPane(target, host)
	w, err := p.whoami(p.panes(false))
	if err != nil {
		p.die(err.Error())
	}
	id, ok := argAfter(args, "--id")
	if !ok {
		id = uuidHex()
	}
	record := NewObj("id", id)
	for _, fk := range [][2]string{{"--order", "order"}, {"--run", "run"}} {
		if v, ok := argAfter(args, fk[0]); ok {
			record.Set(fk[1], v)
		}
	}
	record.Set("ts", now()).Set("to_host", host).Set("to", sp.Get("address")).
		Set("recipient_incarnation", sp.Get("recipient_incarnation")).Set("inbox_only", Truthy(sp.Get("inbox"))).
		Set("from_host", p.selfLabel()).Set("from", w.Address).Set("from_agent", w.Agent).Set("from_cwd", w.Cwd).
		Set("replyable", w.Replyable).Set("message", message).Set("force", force)
	record, err = p.queueRelay(record)
	if err != nil {
		p.die(err.Error())
	}
	note := ""
	if st := sp.Get("state"); Equal(st, "draft") || Equal(st, "prompt") {
		note = fmt.Sprintf(" It is %s right now, so delivery may wait.", pyStr(st))
	}
	fmt.Fprintf(p.out, "queued for %s/%s (%s, %s as of %s ago) [inbox ID %s:%s]; recipient acknowledgment not yet received. "+
		"%s picks it up on its next check-in, within a minute while it's awake.%s peers ls shows the relay's outcome\n",
		host, pyStr(sp.Get("address")), pyStr(sp.Get("agent")), pyStr(sp.Get("state")), ago(snap.Get("at")),
		pyStr(record.Get("from_host")), pyStr(record.Get("id")), host, note)
}

func (p *P) snapshotWait(host string, args []string) {
	timeout := 600
	if v, ok := argAfter(args, "--timeout"); ok {
		timeout, _ = strconv.Atoi(v)
	}
	deadline := now() + float64(timeout)
	for {
		snap, sp := p.snapshotPane(args[0], host)
		if !Equal(sp.Get("state"), "working") {
			fmt.Fprintf(p.out, "%s/%s: %s (as of %s ago)\n", host, pyStr(sp.Get("address")), pyStr(sp.Get("state")), ago(snap.Get("at")))
			return
		}
		if now() > deadline {
			fmt.Fprintf(p.out, "%s/%s: still working after %ds\n", host, pyStr(sp.Get("address")), timeout)
			p.exit(2)
		}
		p.sleep(15 * time.Second)
	}
}

var sshHostname = regexp.MustCompile(`(?m)^hostname (\S+)`)
var linkExes = map[string]bool{"ssh": true, "mosh": true, "mosh-client": true, "et": true}

func (p *P) buildSnapshot(rows []*Row) *Obj {
	ps := p.ps()
	aliases := map[string]string{}
	for _, host := range p.peerHosts() {
		aliases[host] = host
		out := p.run([]string{"ssh", "-G", host}, nil).stdout
		if m := sshHostname.FindStringSubmatch(out); m != nil {
			aliases[m[1]] = host
			aliases[strings.Split(m[1], ".")[0]] = host
		}
	}
	sent := recent(p.sentLog, recentWindow)
	panesOut := []any{}
	for _, r := range rows {
		if !participates(r.Role) || r.Agent == "" {
			continue
		}
		addr := p.address(r, rows)
		links := []any{}
		for _, pid := range ps.subtree(r.PID) {
			words := strings.Fields(ps.cmd[pid])
			if len(words) == 0 || !linkExes[filepath.Base(words[0])] {
				continue
			}
			if _, mcp := ps.agents[ps.ppid[pid]]; mcp {
				continue // an MCP server the harness keeps open, not something steering
			}
			for _, w := range words[1:] {
				at := w[strings.LastIndex(w, "@")+1:]
				if hit, ok := aliases[at]; ok {
					links = append(links, NewObj("host", hit, "cmd", truncRunes(strings.Join(words, " "), 140)))
					break
				}
			}
		}
		b := p.branch(r.Cwd)
		screenLines := []string{}
		if r.Managed != nil {
			screenLines = p.managedScreen(r)
		} else if !strings.HasPrefix(r.ID, "pid:") {
			for _, l := range splitlines(p.tmuxQ("capture-pane", "-p", "-J", "-t", r.ID)) {
				screenLines = append(screenLines, strings.TrimRightFunc(l, isPySpace))
			}
			for len(screenLines) > 0 && pyStrip(screenLines[len(screenLines)-1]) == "" {
				screenLines = screenLines[:len(screenLines)-1]
			}
		}
		recentSent := []any{}
		for _, s := range sent {
			if Equal(s.Get("from"), addr) && strings.Contains(s.Str("to"), "/") {
				recentSent = append(recentSent, s)
			}
		}
		on, _ := p.inboxStatusFlag(r)
		screen := []any{}
		for _, l := range lastN(screenLines, snapshotLines) {
			screen = append(screen, l)
		}
		panesOut = append(panesOut, NewObj(
			"address", addr, "id", r.ID, "agent", nullable(r.Agent), "agent_pid", nullInt(r.AgentPID),
			"agent_started", p.agentStarted(r.AgentPID), "session_id", p.sessionID(r),
			"recipient_incarnation", nullable(p.paneIncarnation(r)), "inbox", on, "state", p.state(r),
			"cwd", p.tilde(r.Cwd), "branch", b, "title", p.cleanTitle(r), "kind", kind(r), "role", peerRole(r.Role),
			"dispatch", nullable(r.Dispatch), "headless", r.Headless, "attached", r.Attached, "activity", r.Activity,
			"links", links, "sent", lastN(recentSent, 3), "screen", screen))
	}
	relayed := []any{}
	for _, r := range lastN(recent(p.relayLog, recentWindow), 10) {
		relayed = append(relayed, r)
	}
	return NewObj("host", p.selfLabel(), "at", now(), "panes", panesOut, "relayed", relayed)
}

func isPySpace(r rune) bool { return pyStrip(string(r)) == "" }

func (p *P) inboxStatusFlag(r *Row) (bool, error) {
	return (r.Managed != nil || !r.Headless) && p.inboxOn(r.Session), nil
}

func fsyncPath(path string) {
	if f, err := os.Open(path); err == nil {
		f.Sync()
		f.Close()
	}
}

// storeTransfer copies a fetched relay before emitting its transport receipt on next beacon.
func (p *P) storeTransfer(msg *Obj) error {
	os.MkdirAll(p.stateDir, 0o755)
	unlock := p.relayLock()
	defer unlock()
	key := sha256Hex(pyStr(msg.Get("via")) + ":" + pyStr(msg.Get("from_host")) + ":" + pyStr(msg.Get("id")))
	path := filepath.Join(p.pending, key+".json")
	prior := inboxRows(p.transferLog)
	fingerprint := relayFingerprint(msg)
	var known *Obj
	for i := len(prior) - 1; i >= 0; i-- {
		if sameKey(prior[i], msg, "via", "from_host", "id") {
			known = prior[i]
			break
		}
	}
	if known != nil {
		kf := known.Get("fingerprint")
		if !known.Has("fingerprint") {
			kf = fingerprint
		}
		if !Equal(kf, fingerprint) {
			return refuse("fetched relay ID conflicts with its prior transfer")
		}
		if Truthy(known.Get("completed")) {
			return nil
		}
	}
	if err := writeJSON(path, msg); err != nil {
		return err
	}
	fsyncPath(path)
	fsyncPath(p.pending)
	if known == nil {
		inboxAppend(p.transferLog, NewObj("id", msg.Get("id"), "from_host", msg.Get("from_host"), "via", msg.Get("via"),
			"fingerprint", fingerprint, "ts", now()))
	}
	return nil
}

func (p *P) cmdBeacon([]string) {
	os.Setenv("PEERS_NO_SELF", "1") // the beacon speaks for the machine, not for a pane
	rows := p.panes(false)
	base := p.buildSnapshot(rows)
	for _, host := range p.peerHosts() {
		cursorPath := filepath.Join(p.stateDir, "sent-cursor-"+host)
		sent, cursor := p.unsent(cursorPath, 500)
		transfers := []any{}
		for _, rec := range inboxRows(p.transferLog) {
			if Equal(rec.Get("via"), host) {
				transfers = append(transfers, NewObj("id", rec.Get("id"), "from_host", rec.Get("from_host")))
			}
		}
		payload := base.Merge(NewObj("sent", sent, "transfer_protocol", 1, "transfers", transfers))
		r := p.remote(host, []string{"_recv"}, Dumps(payload), empty())
		if r.code != 0 {
			fmt.Fprintf(p.err, "%s: %s\n", host, lastLine(r.stderr, "ssh failed"))
			continue
		}
		if err := os.WriteFile(cursorPath, []byte(strconv.FormatInt(cursor, 10)), 0o644); err != nil {
			panic(err)
		}
		body := r.stdout
		if body == "" {
			body = "[]"
		}
		v, err := Loads([]byte(body))
		if err != nil {
			p.die(fmt.Sprintf("%s: invalid _recv reply: %v", host, err))
		}
		list, _ := v.([]any)
		for _, m := range list {
			msg := m.(*Obj)
			msg.Set("via", host)
			if err := p.storeTransfer(msg); err != nil {
				p.die(err.Error())
			}
		}
	}
	if err := p.deliverPending(rows); err != nil {
		p.die(err.Error())
	}
}

// unsent is the lines of the sent log past the byte offset in cursorPath.
func (p *P) unsent(cursorPath string, limit int) ([]any, int64) {
	start := int64(0)
	if b, err := os.ReadFile(cursorPath); err == nil {
		if n, err := strconv.ParseInt(pyStrip(string(b)), 10, 64); err == nil {
			start = n
		} else if pyStrip(string(b)) == "" {
			start = 0
		}
	}
	out := []any{}
	f, err := os.Open(p.sentLog)
	if err != nil {
		return out, start
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && start > st.Size() { // log was replaced
		start = 0
	}
	f.Seek(start, 0)
	r := bufio.NewReader(f)
	for len(out) < limit {
		line, err := r.ReadBytes('\n')
		if err != nil || len(line) == 0 || line[len(line)-1] != '\n' {
			break
		}
		start += int64(len(line))
		if v, err := Loads(line); err == nil {
			out = append(out, v)
		}
	}
	return out, start
}

func (p *P) relayLock() func() {
	os.MkdirAll(p.stateDir, 0o755)
	return lockFile(p.transferLog + ".lock")
}

func (p *P) deliverPending(rows []*Row) error {
	unlock := p.relayLock()
	defer unlock()
	return p.deliverPendingLocked(rows)
}

func (p *P) deliverPendingLocked(rows []*Row) error {
	paths := glob(filepath.Join(p.pending, "*.json"))
	sort.Strings(paths)
	for _, path := range paths {
		msg, err := readObj(path)
		if err != nil {
			return err
		}
		var completed *Obj
		transfers := inboxRows(p.transferLog)
		for i := len(transfers) - 1; i >= 0; i-- {
			if sameKey(transfers[i], msg, "via", "from_host", "id") {
				completed = transfers[i]
				break
			}
		}
		if completed != nil && Truthy(completed.Get("completed")) {
			if !Equal(completed.Get("fingerprint"), relayFingerprint(msg)) {
				return refuse("pending relay conflicts with completed transfer")
			}
			os.Remove(path)
			continue
		}
		var header, origin string
		var extra *Obj
		via := pyStr(msg.Get("via"))
		msgTS, _ := msg.Num("ts")
		if Truthy(msg.Get("thread")) {
			thread := p.saveThread(msg.Obj("thread"))
			origin = pyStr(msg.Get("from"))
			header = p.chatHeader(thread, origin, pyStr(msg.Get("from_desc")), fmt.Sprintf("relayed from %s, queued %s ago", via, ago(msg.Get("ts"))))
			title := thread.Get("title")
			if !thread.Has("title") {
				title = ""
			}
			extra = NewObj("thread", thread.Get("id"), "title", title)
		} else {
			origin = via
			if Truthy(msg.Get("from")) {
				origin = via + "/" + pyStr(msg.Get("from"))
			}
			replyable := true
			if msg.Has("replyable") {
				replyable = Truthy(msg.Get("replyable"))
			}
			header = messageHeader(origin, pyStr(msg.Get("from_agent")), pyStr(msg.Get("from_cwd")),
				Truthy(msg.Get("from")) && replyable, fmt.Sprintf(", relayed from %s (queued %s ago)", via, ago(msg.Get("ts"))))
			extra = NewObj("from_agent", msg.Get("from_agent"), "from_cwd", msg.Get("from_cwd"))
		}
		log := msg.Merge(NewObj("queued_ts", msg.Get("ts"), "ts", now()))
		err = func() error {
			to := pyStr(msg.Get("to"))
			pane, err := find(to, rows)
			if err != nil {
				return err
			}
			expected := msg.Get("recipient_incarnation")
			actual := p.paneIncarnation(pane)
			if Truthy(expected) && actual == "" {
				return refuse("cannot establish recipient incarnation")
			}
			if !Truthy(expected) || !Equal(actual, expected) {
				return refuse("recipient incarnation changed or legacy relay is unbound")
			}
			add := NewObj()
			for _, k := range []string{"order", "run"} {
				if msg.Has(k) {
					add.Set(k, msg.Get(k))
				}
			}
			extra = extra.Merge(add, NewObj("id", pyStr(msg.Get("from_host"))+":"+pyStr(msg.Get("id")), "recipient_incarnation", expected,
				"origin_ts", msg.Get("ts"), "inbox_only", Truthy(msg.Get("inbox_only"))))
			delivered, st, err := p.sendTo(to, rows, header, pyStr(msg.Get("message")), origin, Truthy(msg.Get("force")), extra)
			if err != nil {
				return err
			}
			status := "submitted; not acknowledged"
			if st == "inbox" {
				status = heldStatus(delivered)
			}
			appendLine(p.relayLog, log.Merge(NewObj("status", status)))
			// Completion is durable before removing pending; refetch must not recreate it.
			inboxAppend(p.transferLog, NewObj("id", msg.Get("id"), "from_host", msg.Get("from_host"), "via", msg.Get("via"),
				"fingerprint", relayFingerprint(msg), "completed", true))
			os.Remove(path)
			return nil
		}()
		if err == nil {
			continue
		}
		if _, ok := err.(*Refused); !ok {
			return err
		}
		reason := err.Error()
		gone := strings.Contains(reason, "incarnation changed") || strings.Contains(reason, "legacy relay is unbound") ||
			strings.HasPrefix(reason, "no pane") || strings.Contains(reason, "own pane") || strings.Contains(reason, "plain shell") ||
			strings.Contains(reason, "headless")
		if gone || now()-msgTS > relayTTL {
			appendLine(p.relayLog, log.Merge(NewObj("status", "dropped: "+reason)))
			os.Remove(path)
		} else {
			first, _, _ := strings.Cut(reason, ";")
			appendLine(p.relayLog, log.Merge(NewObj("status", "waiting: "+first)))
		}
	}
	return nil
}

// cmdRecv runs over ssh from a beacon: store its snapshot, hand back its queued messages.
func (p *P) cmdRecv([]string) {
	snap, err := LoadObj([]byte(p.readStdinArg("-")))
	if err != nil {
		p.die(err.Error())
	}
	protocol := snap.Pop("transfer_protocol")
	transfers := map[string]bool{}
	for _, v := range snap.List("transfers") {
		if rec, ok := v.(*Obj); ok {
			transfers[Dumps([]any{rec.Get("from_host"), rec.Get("id")})] = true
		}
	}
	snap.Del("transfers")
	host := pyStr(snap.Get("host"))
	for _, v := range snap.List("sent") { // the beacon host's message log
		if rec, ok := v.(*Obj); ok {
			appendLine(filepath.Join(p.snapshots, host+".sent.jsonl"), rec)
		}
	}
	snap.Del("sent")
	if err := writeJSON(filepath.Join(p.snapshots, host+".json"), snap); err != nil {
		panic(err)
	}
	msgs := []any{}
	for _, path := range glob(filepath.Join(p.outbox, "*.json")) {
		msg, err := readObj(path)
		if err != nil {
			continue
		}
		if !Equal(msg.Get("to_host"), snap.Get("host")) {
			continue
		}
		if Equal(protocol, 1) && transfers[Dumps([]any{msg.Get("from_host"), msg.Get("id")})] {
			os.Remove(path) // transport copied durably; this is NOT a recipient ACK
			continue
		}
		msgs = append(msgs, msg)
		if !Equal(protocol, 1) { // compatibility with the previous caller's wire shape
			os.Remove(path)
		}
	}
	fmt.Fprintln(p.out, Dumps(msgs))
}
