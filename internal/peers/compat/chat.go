package compat

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A chat is a thread id and its members, as host-qualified addresses. `say`
// fans a message out to every other member: local panes directly, hosts we can
// ssh to through _chat, and a host that can't be reached through the relay
// outbox. Every copy carries the thread record.

const chatUsage = `chat new MEMBER... [--title T] [-m MESSAGE|-]   start a chat with these peers
chat say ID MESSAGE|-                           message everyone in it
chat add ID MEMBER... [-m MESSAGE|-]            bring more peers in
chat leave ID|--all [--reason TEXT]             stop getting its messages (--all: every chat
                                                you're in; the /clear hooks run it with --hook)
chat ls                                         chats this machine knows
chat show ID [-n N]                             the thread so far`

func (p *P) qualify(addr string) string {
	if strings.Contains(addr, "/") {
		return addr
	}
	return p.selfLabel() + "/" + addr
}

func (p *P) relative(addr string) string {
	host := p.selfLabel()
	if strings.HasPrefix(addr, host+"/") {
		return addr[len(host)+1:]
	}
	return addr
}

func (p *P) loadThread(tid string) *Obj {
	t, err := readObj(filepath.Join(p.threads, strings.TrimLeft(tid, "#")+".json"))
	if err != nil {
		return nil
	}
	return t
}

func maxNum(a, b any) any {
	fa, _ := num(a)
	fb, _ := num(b)
	if fb > fa {
		return b
	}
	return a
}

func orZero(v any) any {
	if !Truthy(v) {
		return 0
	}
	return v
}

// saveThread merges t into what this machine knows: per member, the latest join and leave win.
func (p *P) saveThread(t *Obj) *Obj {
	old := p.loadThread(t.Str("id"))
	if old == nil {
		old = NewObj()
	}
	members := NewObj()
	for _, src := range []*Obj{old, t} {
		ms := src.Obj("members")
		for _, m := range ms.Keys() {
			v := ms.Obj(m)
			if !members.Has(m) {
				members.Set(m, NewObj("joined", 0, "left", 0))
			}
			cur := members.Obj(m)
			jv, _ := num(orZero(v.Get("joined")))
			jc, _ := num(cur.Get("joined"))
			if jv > jc || !Truthy(cur.Get("recipient_incarnation")) {
				cur.Set("recipient_incarnation", v.Get("recipient_incarnation"))
			}
			cur.Set("joined", maxNum(cur.Get("joined"), orZero(v.Get("joined"))))
			cur.Set("left", maxNum(cur.Get("left"), orZero(v.Get("left"))))
		}
	}
	title := t.Get("title")
	if !Truthy(title) {
		title = old.Get("title")
		if !old.Has("title") {
			title = ""
		}
	}
	merged := old.Merge(t, NewObj("members", members, "title", title))
	if err := writeJSON(filepath.Join(p.threads, t.Str("id")+".json"), merged); err != nil {
		panic(err)
	}
	return merged
}

func active(t *Obj) []string {
	out := []string{}
	ms := t.Obj("members")
	for _, m := range ms.Keys() {
		v := ms.Obj(m)
		j, _ := num(v.Get("joined"))
		l, _ := num(v.Get("left"))
		if j > l {
			out = append(out, m)
		}
	}
	return out
}

func (p *P) relList(addrs []string) string {
	out := make([]string, len(addrs))
	for i, a := range addrs {
		out[i] = p.relative(a)
	}
	return strings.Join(out, ", ")
}

// chatHeader is the header on every chat message.
func (p *P) chatHeader(t *Obj, origin, desc, note string) string {
	title := ""
	if Truthy(t.Get("title")) {
		title = " “" + pyStr(t.Get("title")) + "”"
	}
	if note != "" {
		note = ", " + note
	}
	id := pyStr(t.Get("id"))
	return fmt.Sprintf("[peer chat #%s%s: from %s (%s)%s. In this chat: %s. Reply to all of them with: peers chat say %s \"...\" · whole thread: peers chat show %s]",
		id, title, p.relative(origin), desc, note, p.relList(active(t)), id, id)
}

func memberOf(t *Obj, addr string) *Obj {
	ms := t.Obj("members")
	if !ms.Has(addr) {
		ms.Set(addr, NewObj("joined", 0, "left", 0))
	}
	return ms.Obj(addr)
}

// deliverChat delivers to members on this machine; returns [address, outcome] pairs.
func (p *P) deliverChat(t *Obj, origin, desc, message string, names []string, rows []*Row) [][]string {
	t = p.saveThread(t)
	header := p.chatHeader(t, origin, desc, "")
	title := t.Get("title")
	if !t.Has("title") {
		title = ""
	}
	extra := NewObj("thread", t.Get("id"), "title", title, "from_desc", desc)
	out := [][]string{}
	for _, n := range names {
		var expected any
		if m := t.Obj("members").Obj(p.qualify(n)); m != nil {
			expected = m.Get("recipient_incarnation")
		}
		var err error
		var delivered *Row
		var st string
		if !Truthy(expected) {
			err = refuse("legacy chat membership is unbound; explicitly add the intended current recipient")
		} else {
			delivered, st, err = p.sendTo(n, rows, header, message, origin, false, extra.Merge(NewObj("recipient_incarnation", expected)))
		}
		if err == nil {
			outcome := "delivered"
			if st == "inbox" {
				outcome = heldStatus(delivered)
			} else if st == "working" {
				outcome += " (mid-turn; it reads it at its next tool boundary)"
			}
			out = append(out, []string{p.qualify(n), outcome})
			continue
		}
		if _, ok := err.(*Refused); !ok {
			panic(err)
		}
		if strings.HasPrefix(err.Error(), "no pane") { // its session was killed: it's out of the chat
			memberOf(t, p.qualify(n)).Set("left", now())
			t = p.saveThread(t)
			out = append(out, []string{p.qualify(n), "gone (no such session any more); removed from the chat"})
		} else {
			out = append(out, []string{p.qualify(n), "not delivered: " + err.Error()})
		}
	}
	return out
}

func (p *P) fanOut(t *Obj, origin, desc, message string, only []string) [][]string {
	rows := p.panes(false)
	var hosts []string
	byHost := map[string][]string{}
	for _, m := range active(t) {
		if m != origin && (only == nil || contains(only, m)) {
			host, name, _ := strings.Cut(m, "/")
			if _, ok := byHost[host]; !ok {
				hosts = append(hosts, host)
			}
			byHost[host] = append(byHost[host], name)
		}
	}
	results := [][]string{}
	for _, host := range hosts {
		names := byHost[host]
		switch {
		case host == p.selfLabel():
			results = append(results, p.deliverChat(t, origin, desc, message, names, rows)...)
		case contains(p.peerHosts(), host):
			payload := NewObj("thread", t, "from", origin, "desc", desc, "message", message, "to", names)
			r := p.remote(host, []string{"_chat"}, Dumps(payload), empty())
			if r.code == 0 {
				got := [][]string{}
				body := r.stdout
				if body == "" {
					body = "[]"
				}
				if v, err := Loads([]byte(body)); err == nil {
					list, _ := v.([]any)
					for _, pair := range list {
						pl, _ := pair.([]any)
						if len(pl) == 2 {
							got = append(got, []string{pyStr(pl[0]), pyStr(pl[1])})
						}
					}
				} else {
					p.die(err.Error())
				}
				for _, g := range got {
					if strings.HasPrefix(g[1], "gone") {
						memberOf(t, g[0]).Set("left", now())
						p.saveThread(t)
					}
				}
				results = append(results, got...)
			} else {
				why := lastLine(r.stderr, "ssh failed")
				for _, n := range names {
					results = append(results, []string{host + "/" + n, fmt.Sprintf("not delivered: %s unreachable (%s)", host, why)})
				}
			}
		case p.snapshot(host) != nil:
			for _, n := range names {
				var inc any
				if m := t.Obj("members").Obj(host + "/" + n); m != nil {
					inc = m.Get("recipient_incarnation")
				}
				_, sp := p.snapshotPane(n, host)
				record := NewObj("id", uuidHex()[:12], "ts", now(), "to_host", host, "to", n, "recipient_incarnation", inc,
					"inbox_only", Truthy(sp.Get("inbox")), "from_host", p.selfLabel(), "from", origin, "from_desc", desc,
					"from_agent", "", "from_cwd", "", "message", message, "thread", t)
				if _, err := p.queueRelay(record); err != nil {
					results = append(results, []string{host + "/" + n, "not queued: " + err.Error()})
					continue
				}
				results = append(results, []string{host + "/" + n, "queued; relayed on its next check-in, within a minute while it's awake"})
			}
		default:
			for _, n := range names {
				results = append(results, []string{host + "/" + n, "not delivered: no route to " + host})
			}
		}
	}
	return results
}

// chatRecords is this machine's log lines for a thread, and the logs other
// machines' beacons shipped here.
func (p *P) chatRecords(tid string) []*Obj {
	out := []*Obj{}
	for _, path := range append([]string{p.sentLog}, sortedGlob(filepath.Join(p.snapshots, "*.sent.jsonl"))...) {
		for _, v := range jsonLines(path) {
			if rec, ok := v.(*Obj); ok && Equal(rec.Get("thread"), tid) {
				out = append(out, rec)
			}
		}
	}
	return out
}

type chatMessage struct {
	ts      float64
	from    string
	to      []string
	message string
}

func (p *P) threadMessages(tid string, everywhere bool) []*chatMessage {
	recs := p.chatRecords(tid)
	if everywhere && p.hostLabel == "" {
		for _, host := range p.peerHosts() {
			r := p.remote(host, []string{"_chatlog", tid}, "", empty())
			if r.code == 0 {
				body := r.stdout
				if body == "" {
					body = "[]"
				}
				if v, err := Loads([]byte(body)); err == nil {
					list, _ := v.([]any)
					for _, x := range list {
						if o, ok := x.(*Obj); ok {
							recs = append(recs, o)
						}
					}
				}
			}
		}
	}
	// One copy is logged per recipient; fold them back into one message.
	sort.SliceStable(recs, func(i, j int) bool {
		a, _ := recs[i].Num("ts")
		b, _ := recs[j].Num("ts")
		return a < b
	})
	msgs := []*chatMessage{}
	for _, rec := range recs {
		from := pyStr(rec.Get("from"))
		frm := from
		if !strings.Contains(from, "/") {
			frm = p.qualify(from)
		}
		ts, _ := rec.Num("ts")
		message := pyStr(rec.Get("message"))
		var same *chatMessage
		for i := len(msgs) - 1; i >= 0; i-- {
			m := msgs[i]
			if m.from == frm && m.message == message && ts-m.ts < 900 {
				same = m
				break
			}
		}
		rto := pyStr(rec.Get("to"))
		to := rto
		if !strings.HasPrefix(rto, "#") && !strings.Contains(rto, "/") {
			host := pyStr(rec.Get("host"))
			if !Truthy(rec.Get("host")) {
				host = p.selfLabel()
			}
			to = host + "/" + rto
		}
		if same != nil {
			if !strings.HasPrefix(to, "#") && !contains(same.to, to) {
				same.to = append(same.to, to)
			}
			continue
		}
		m := &chatMessage{ts: ts, from: frm, to: []string{}, message: message}
		if !strings.HasPrefix(to, "#") {
			m.to = []string{to}
		}
		msgs = append(msgs, m)
	}
	return msgs
}

// take pops `flag VALUE` out of args.
func (p *P) take(args *[]string, flag string) (string, bool) {
	for i, a := range *args {
		if a == flag {
			if i+1 >= len(*args) {
				p.die(flag + " needs a value")
			}
			v := (*args)[i+1]
			*args = append(append([]string{}, (*args)[:i]...), (*args)[i+2:]...)
			return v, true
		}
	}
	return "", false
}

func (p *P) meInChat() (string, string, []*Row) {
	rows := p.panes(false)
	w, err := p.whoami(rows)
	if err != nil {
		p.die(err.Error())
	}
	return p.qualify(w.Address), w.Agent + ", " + w.Cwd, rows
}

func (p *P) printResults(results [][]string) {
	for _, r := range results {
		fmt.Fprintf(p.out, "  %s: %s\n", p.relative(r[0]), r[1])
	}
}

func (p *P) memberBinding(addr string, rows []*Row) any {
	host, target := splitHost(addr)
	if host == "" || host == p.selfLabel() {
		row, err := find(target, rows)
		if err != nil {
			p.die(err.Error())
		}
		return nullable(p.paneIncarnation(row))
	}
	if !contains(p.peerHosts(), host) {
		_, sp := p.snapshotPane(target, host)
		return sp.Get("recipient_incarnation")
	}
	r := p.remote(host, []string{"ls", "--json"}, "", empty())
	packet, err := LoadObj([]byte(r.stdout))
	if err != nil || !packet.Has("panes") {
		return nil
	}
	match := []*Obj{}
	for _, v := range packet.List("panes") {
		o, ok := v.(*Obj)
		if ok && (Equal(o.Get("address"), addr) || Equal(o.Get("pane"), target) || Equal(o.Get("session"), target)) {
			match = append(match, o)
		}
	}
	if r.code == 0 && len(match) == 1 {
		return match[0].Get("recipient_incarnation")
	}
	return nil
}

func (p *P) joinedMember(addr string, rows []*Row, at float64) *Obj {
	binding := p.memberBinding(addr, rows)
	if !Truthy(binding) {
		p.die(fmt.Sprintf("cannot bind chat membership for %s; inspect current recipient before retrying", addr))
	}
	return NewObj("joined", at, "left", 0, "recipient_incarnation", binding)
}

func (p *P) cmdChat(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(p.out, chatUsage)
		return
	}
	sub, args := args[0], args[1:]
	if sub == "ls" {
		p.chatLs()
		return
	}
	if sub == "leave" && contains(args, "--all") {
		reason, ok := p.take(&args, "--reason")
		if !ok || reason == "" {
			reason = "left the chat"
		}
		quiet := contains(args, "-q")
		if harness, ok := p.take(&args, "--hook"); ok && harness != "" && !p.hookIdentity(harness) {
			return
		}
		origin, desc, rows := p.meInChat()
		for _, path := range glob(filepath.Join(p.threads, "*.json")) {
			t := p.loadThread(strings.TrimSuffix(filepath.Base(path), ".json"))
			if t != nil && contains(active(t), origin) && Equal(t.Obj("members").Obj(origin).Get("recipient_incarnation"), p.memberBinding(origin, rows)) {
				p.leave(t, origin, desc, reason, quiet)
			}
		}
		return
	}
	if sub == "new" {
		title, _ := p.take(&args, "--title")
		message, hasMessage := p.take(&args, "-m")
		if len(args) == 0 {
			p.die("chat new MEMBER... [--title T] [-m MESSAGE|-]")
		}
		origin, desc, rows := p.meInChat()
		members := []string{}
		for _, a := range args {
			host, name := splitHost(a)
			if host == "" || host == p.selfLabel() {
				row, err := find(name, rows)
				if err != nil {
					p.die(err.Error())
				}
				if row.Agent == "" {
					p.die(a + " is a plain shell, not an agent")
				}
				a = p.address(row, rows)
			}
			a = p.qualify(a)
			if a == origin {
				p.die(a + " is you")
			}
			members = append(members, a)
		}
		at := now()
		ms := NewObj()
		for _, m := range append([]string{origin}, members...) {
			ms.Set(m, p.joinedMember(m, rows, at))
		}
		t := NewObj("id", uuidHex()[:6], "title", title, "created", at, "created_by", origin, "members", ms)
		t = p.saveThread(t)
		fmt.Fprintf(p.out, "chat #%s with %s\n", t.Str("id"), p.relList(members))
		if hasMessage && message != "" {
			// The reference passes -m - through literally; read stdin as its usage documents.
			if message == "-" {
				message = pyStrip(p.readStdinArg(message))
			}
			p.printResults(p.fanOut(t, origin, desc, p.sayText(message, t, origin), nil))
		} else {
			fmt.Fprintf(p.out, "nothing sent yet: peers chat say %s \"...\"\n", t.Str("id"))
		}
		return
	}
	if len(args) == 0 {
		p.die(chatUsage)
	}
	tid := strings.TrimLeft(args[0], "#")
	args = args[1:]
	t := p.loadThread(tid)
	if t == nil {
		p.die(fmt.Sprintf("no chat #%s on %s (peers chat ls); a machine learns a chat when one of its messages lands here", tid, p.selfLabel()))
	}
	if sub == "show" {
		n := 40
		if v, ok := p.take(&args, "-n"); ok && v != "" {
			var err error
			if n, err = strconv.Atoi(strings.TrimSpace(v)); err != nil {
				p.die(fmt.Sprintf("invalid literal for int() with base 10: %s", pyRepr(v)))
			}
		}
		title := ""
		if Truthy(t.Get("title")) {
			title = " “" + pyStr(t.Get("title")) + "”"
		}
		fmt.Fprintf(p.out, "#%s%s: %s\n", pyStr(t.Get("id")), title, p.relList(active(t)))
		for _, m := range lastN(p.threadMessages(tid, true), n) {
			stamp := time.Unix(int64(m.ts), 0).Local().Format("Jan 02 15:04")
			to := p.relList(m.to)
			if to == "" {
				to = "the chat"
			}
			fmt.Fprintf(p.out, "\n%s  %s → %s\n", stamp, p.relative(m.from), to)
			lines := []string{}
			for _, l := range splitlines(m.message) {
				lines = append(lines, "  "+l)
			}
			fmt.Fprintln(p.out, strings.Join(lines, "\n"))
		}
		return
	}
	origin, desc, rows := p.meInChat()
	at := now()
	switch sub {
	case "say":
		if len(args) == 0 {
			p.die("chat say ID MESSAGE|-")
		}
		message := pyStrip(p.readStdinArg(args[0]))
		if message == "" {
			p.die("empty message")
		}
		if !contains(active(t), origin) || !Equal(t.Obj("members").Obj(origin).Get("recipient_incarnation"), p.memberBinding(origin, rows)) {
			t.Obj("members").Set(origin, p.joinedMember(origin, rows, at))
			t = p.saveThread(t)
		}
		fmt.Fprintf(p.out, "#%s:\n", t.Str("id"))
		p.printResults(p.fanOut(t, origin, desc, p.sayText(message, t, origin), nil))
	case "add":
		message, hasMessage := p.take(&args, "-m")
		if len(args) == 0 {
			p.die("chat add ID MEMBER... [-m MESSAGE|-]")
		}
		added := []string{}
		for _, a := range args {
			added = append(added, p.qualify(a))
		}
		bind := append([]string{}, added...)
		if !contains(active(t), origin) {
			bind = append(bind, origin)
		}
		for _, m := range bind {
			t.Obj("members").Set(m, p.joinedMember(m, rows, at))
		}
		t = p.saveThread(t)
		backlog := lastN(p.threadMessages(tid, true), 5)
		explicit := hasMessage && message != ""
		text := p.relative(origin) + " added you to this chat."
		if explicit {
			text = pyStrip(p.readStdinArg(message))
		}
		if len(backlog) > 0 && !explicit {
			lines := []string{}
			for _, m := range backlog {
				lines = append(lines, fmt.Sprintf("- %s: %s", p.relative(m.from), truncRunes(m.message, 300)))
			}
			text += " The last messages:\n" + strings.Join(lines, "\n")
		}
		fmt.Fprintf(p.out, "#%s: added %s\n", t.Str("id"), p.relList(added))
		p.printResults(p.fanOut(t, origin, desc, p.sayText(text, t, origin), added))
	case "leave":
		reason, ok := p.take(&args, "--reason")
		if !ok || reason == "" {
			reason = "left the chat"
		}
		p.leave(t, origin, desc, reason, false)
	default:
		p.die(fmt.Sprintf("unknown chat command %s\n%s", pyRepr(sub), chatUsage))
	}
}

// leave tells the others, then drops out. The note carries the thread record
// with this member already gone, so every machine that gets it stops sending here.
func (p *P) leave(t *Obj, origin, desc, reason string, quiet bool) {
	others := []string{}
	for _, m := range active(t) {
		if m != origin {
			others = append(others, m)
		}
	}
	memberOf(t, origin).Set("left", now())
	t = p.saveThread(t)
	results := [][]string{}
	if len(others) > 0 {
		results = p.fanOut(t, origin, desc, p.sayText(fmt.Sprintf("%s %s.", p.relative(origin), reason), t, origin), others)
	}
	if !quiet {
		p.printResults(results)
		fmt.Fprintf(p.out, "left #%s\n", t.Str("id"))
	}
}

// hookIdentity is called from a harness's SessionStart hook (on /clear), with
// the hook's JSON on stdin. Claude runs hooks under its own process, so me()
// finds the pane. Codex runs them in its app-server daemon, outside the pane,
// so pick the one pane of that harness in the hook's cwd; with two or more,
// leave nothing rather than guess.
func (p *P) hookIdentity(harness string) bool {
	hook := NewObj()
	if !p.stdinIsTTY() {
		raw, _ := readAll(p.in)
		if raw == "" {
			raw = "{}"
		}
		if o, err := LoadObj([]byte(raw)); err == nil {
			hook = o
		}
	}
	rows := p.panes(false)
	if p.me(rows) != nil {
		return true
	}
	cwd := realpath(hook.Str("cwd"))
	found := []*Row{}
	for _, r := range rows {
		if r.Agent == harness && !r.Headless && realpath(r.Cwd) == cwd {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		return false
	}
	os.Setenv("PEERS_FROM", found[0].ID)
	return true
}

func (p *P) stdinIsTTY() bool {
	f, ok := p.in.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// sayText logs the message once as said by its sender, so the thread shows it even if no copy lands.
func (p *P) sayText(message string, t *Obj, origin string) string {
	title := t.Get("title")
	if !t.Has("title") {
		title = ""
	}
	appendLine(p.sentLog, NewObj("ts", now(), "host", p.selfLabel(), "from", origin, "to", "#"+t.Str("id"),
		"message", message, "thread", t.Get("id"), "title", title))
	return message
}

func (p *P) chatLs() {
	type entry struct {
		last float64
		t    *Obj
		recs []*Obj
	}
	threads := []entry{}
	for _, path := range glob(filepath.Join(p.threads, "*.json")) {
		t := p.loadThread(strings.TrimSuffix(filepath.Base(path), ".json"))
		if t == nil {
			continue
		}
		recs := p.chatRecords(t.Str("id"))
		last, _ := t.Num("created")
		for _, r := range recs {
			if ts, _ := r.Num("ts"); ts > last {
				last = ts
			}
		}
		threads = append(threads, entry{last, t, recs})
	}
	if len(threads) == 0 {
		fmt.Fprintf(p.out, "no chats on %s. peers chat new MEMBER... to start one\n", p.selfLabel())
		return
	}
	sort.SliceStable(threads, func(i, j int) bool { return threads[i].last > threads[j].last })
	for _, e := range threads {
		title := ""
		if Truthy(e.t.Get("title")) {
			title = " “" + pyStr(e.t.Get("title")) + "”"
		}
		distinct := map[string]bool{}
		for _, r := range e.recs {
			distinct[Dumps([]any{r.Get("from"), r.Get("message")})] = true
		}
		fmt.Fprintf(p.out, "#%s%s  %s  (%d messages, last %s ago)\n", pyStr(e.t.Get("id")), title, p.relList(active(e.t)), len(distinct), ago(e.last))
		if len(e.recs) > 0 {
			r := e.recs[len(e.recs)-1]
			preview := truncRunes(spaceRE.ReplaceAllString(pyStr(r.Get("message")), " "), 100)
			fmt.Fprintf(p.out, "    %s: %s\n", p.relative(p.qualify(pyStr(r.Get("from")))), preview)
		}
	}
}

// cmdChatRecv runs over ssh from another machine's `chat say`: deliver to members here.
func (p *P) cmdChatRecv([]string) {
	payload, err := LoadObj([]byte(p.readStdinArg("-")))
	if err != nil {
		p.die(err.Error())
	}
	names := []string{}
	for _, n := range payload.List("to") {
		names = append(names, pyStr(n))
	}
	out := p.deliverChat(payload.Obj("thread"), pyStr(payload.Get("from")), pyStr(payload.Get("desc")), pyStr(payload.Get("message")), names, p.panes(false))
	fmt.Fprintln(p.out, Dumps(out))
}

func (p *P) cmdChatlog(args []string) {
	if len(args) == 0 {
		p.die("list index out of range")
	}
	fmt.Fprintln(p.out, Dumps(p.chatRecords(args[0])))
}
