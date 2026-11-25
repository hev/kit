package compat

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func anyIn(args []string, flags ...string) bool {
	for _, f := range flags {
		if contains(args, f) {
			return true
		}
	}
	return false
}

func dedupe(ids []string) []string {
	out, seen := []string{}, map[string]bool{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (p *P) tokenRevoked(rows []*Row) bool {
	_, ok := os.LookupEnv("PEERS_INBOX_TOKEN")
	return ok && p.runtimeRecipient(rows) == nil
}

// cmdInbox reads, counts or toggles an inbox session's held messages. Inbox
// setup may precede participation; it never changes the session role.
func (p *P) cmdInbox(args []string) {
	session := ""
	for i, a := range args {
		if a == "--session" {
			if i+1 >= len(args) {
				p.die("--session needs a name")
			}
			session = args[i+1]
			args = append(append([]string{}, args[:i]...), args[i+2:]...)
			break
		}
	}
	inspect := []string{"--peek", "--count", "--all", "--current-all", "--wait"}
	if session != "" && strings.Contains(session, "/") {
		host, target := splitHost(session)
		if host != "" {
			if !anyIn(args, "--peek", "--all", "--count", "--current-all") ||
				(len(args) > 0 && contains([]string{"ack", "consume", "hold", "on", "off", "owner", "bind-runtime"}, args[0])) {
				p.die("remote inbox access is inspection-only; use --peek or --all")
			}
			if !contains(p.peerHosts(), host) {
				p.die("recipient ACK status unavailable on this route; queueing is not acknowledgment")
			}
			r := p.remote(host, append([]string{"inbox", "--session", target}, args...), "", empty())
			fmt.Fprint(p.out, r.stdout)
			if r.code != 0 {
				msg := pyStrip(r.stderr)
				if msg == "" {
					msg = "remote inbox inspection failed; acknowledgment unknown"
				}
				p.die(msg)
			}
			return
		}
		session = target
	}
	rows := p.panes(true)
	if len(args) > 0 && (args[0] == "on" || args[0] == "off") {
		target := session
		if len(args) > 1 {
			target = args[1]
		}
		if target == "" {
			mine := p.me(rows)
			if mine == nil {
				p.die(noSender)
			}
			target = mine.Session
		}
		if p.managedRecord(target) != nil {
			p.die("managed inbox registration follows its runner lifetime; no tmux toggle")
		}
		p.tmux("set-option", "-t", "="+target+":", "@peers-inbox", args[0])
		suffix := ""
		if args[0] == "on" {
			suffix = " (peer messages are held, not typed in)"
		}
		fmt.Fprintf(p.out, "%s: inbox %s%s\n", target, args[0], suffix)
		return
	}
	if len(args) > 0 && args[0] == "bind-runtime" {
		p.bindRuntime(rows, args[1:])
		return
	}
	if session == "" {
		_, token := os.LookupEnv("PEERS_INBOX_TOKEN")
		var mine *Row
		if anyIn(args, inspect...) && !token {
			mine = p.me(rows)
		} else {
			mine = p.recipientMe(rows)
		}
		if mine == nil {
			p.die(noSender)
		}
		session = mine.Session
		if contains(args, "--wait") {
			p.inboxWait(session, p.paneIncarnation(mine), args)
			return
		}
	}
	if contains(args, "--wait") {
		p.inboxWait(session, p.incarnation(session, "", false), args)
		return
	}
	if len(args) > 0 && args[0] == "hold" { // a message that reached the session some other way
		sender := "unknown"
		if v, ok := argAfter(args, "--sender"); ok {
			if !hasAfter(args, "--sender") {
				p.die("list index out of range")
			}
			sender = v
		}
		message := pyStrip(p.readStdinArg("-"))
		if message == "" {
			p.die("empty message")
		}
		held, err := p.hold(session, "[held message from "+sender+"]", message, sender, nil)
		if err != nil {
			p.die(err.Error())
		}
		fmt.Fprintf(p.out, "%s: queued in inbox [id %s]; not acknowledged\n", session, pyStr(held.Get("id")))
		return
	}
	// History is safe to inspect even after the session has disappeared.
	history := contains(args, "--all")
	unlock := p.inboxLock(session)
	defer unlock()
	st := p.inboxView(session, "")
	identity := st.identity
	op := ""
	if len(args) > 0 {
		op = args[0]
	}
	ack, consume, owner := op == "ack", op == "consume", op == "owner"
	mutating := owner || ack || consume || !anyIn(args, inspect...)
	if mutating {
		mine := p.recipientMe(rows)
		if mine == nil || mine.Session != session || !participates(mine.Role) {
			p.die("only the intended current recipient may consume or acknowledge; use --peek to inspect")
		}
		if identity == "" || p.paneIncarnation(mine) != identity {
			p.die("cannot establish current recipient incarnation")
		}
	}
	receipts := p.inboxPath(session) + ".receipts.jsonl"
	if owner {
		fmt.Fprintln(p.out, Dumps(NewObj("session", session, "recipient_incarnation", nullable(identity), "on", p.inboxOn(session))))
		return
	}
	byID := map[string]*Obj{}
	for _, m := range st.current {
		if id, ok := m.Get("id").(string); ok {
			byID[id] = m
		}
	}
	if consume {
		ids := dedupe(args[1:])
		known := len(ids) > 0
		for _, id := range ids {
			if byID[id] == nil {
				known = false
			}
		}
		if !known {
			p.die("consume requires known message IDs for this recipient incarnation")
		}
		if p.incarnation(session, "", false) != identity || p.tokenRevoked(rows) {
			p.die("recipient incarnation or runtime changed before consumption")
		}
		for _, id := range ids {
			if byID[id].Get("consumed_at") == nil {
				inboxAppend(receipts, NewObj("id", id, "recipient_incarnation", identity, "kind", "consumed", "ts", now()))
			}
		}
		fmt.Fprintln(p.out, Dumps(NewObj("session", session, "consumed", ids)))
		return
	}
	if ack {
		ids := args[1:]
		if len(ids) == 0 {
			p.die("inbox ack MESSAGE_ID... (after consuming the messages)")
		}
		for _, id := range ids {
			if byID[id] == nil || byID[id].Get("consumed_at") == nil {
				p.die("ACK requires an already consumed message for this recipient incarnation")
			}
		}
		if p.incarnation(session, "", false) != identity || p.tokenRevoked(rows) {
			p.die("recipient incarnation or runtime changed before acknowledgment")
		}
		for _, id := range dedupe(ids) {
			if byID[id].Get("acknowledged_at") == nil {
				inboxAppend(receipts, NewObj("id", id, "recipient_incarnation", identity, "kind", "acknowledged", "ts", now()))
			}
		}
		fmt.Fprintln(p.out, Dumps(NewObj("session", session, "acknowledged", dedupe(ids))))
		return
	}
	if contains(args, "--count") {
		fmt.Fprintln(p.out, len(st.unread))
		return
	}
	pending := []*Obj{}
	unacked := 0
	for _, m := range st.current {
		if m.Get("acknowledged_at") == nil {
			pending = append(pending, m)
			unacked++
		}
	}
	var shown []*Obj
	switch {
	case history:
		shown = st.messages
	case contains(args, "--current-all"):
		shown = st.current
	case contains(args, "--pending"):
		shown = pending
	default:
		shown = st.unread
	}
	if !history { // {(incarnation, id): m}: the last copy, in the first copy's place
		index := map[string]int{}
		kept := []*Obj{}
		for _, m := range shown {
			k := Dumps([]any{m.Get("recipient_incarnation"), m.Get("id")})
			if i, ok := index[k]; ok {
				kept[i] = m
				continue
			}
			index[k] = len(kept)
			kept = append(kept, m)
		}
		shown = kept
	}
	if contains(args, "--limit") {
		v, _ := argAfter(args, "--limit")
		limit, err := strconv.Atoi(strings.TrimSpace(v))
		if !hasAfter(args, "--limit") || err != nil || limit < 1 || limit > 64 {
			p.die("--limit must be 1..64")
		}
		if len(shown) > limit {
			shown = shown[:limit]
		}
	}
	historical := len(st.messages) - len(st.current)
	switch {
	case contains(args, "--json"):
		fmt.Fprintln(p.out, Dumps(NewObj("session", session, "recipient_incarnation", nullable(identity), "on", p.inboxOn(session),
			"unread", len(st.unread), "unacknowledged", unacked, "historical", historical, "messages", shown)))
	case len(shown) == 0:
		fmt.Fprintf(p.out, "%s: inbox empty (historical: %d)\n", session, historical)
	default:
		for _, m := range shown {
			id := "legacy"
			if m.Has("id") {
				id = pyStr(m.Get("id"))
			}
			where := "historical"
			if m.Get("current") == true {
				where = "current"
			}
			fmt.Fprintf(p.out, "--- %s [%s; %s] %s ago\n%s\n%s\n\n", id, pyStr(m.Get("transport_state")), where, ago(m.Get("ts")),
				pyStr(m.Get("header")), pyStr(m.Get("message")))
		}
	}
	if mutating {
		if p.incarnation(session, "", false) != identity || p.tokenRevoked(rows) {
			p.die("recipient incarnation or runtime changed before consumption")
		}
		for _, m := range shown {
			if m.Get("consumed_at") != nil {
				continue
			}
			inboxAppend(receipts, NewObj("id", m.Get("id"), "recipient_incarnation", identity, "kind", "consumed", "ts", now()))
		}
	}
}

func hasAfter(args []string, flag string) bool {
	for i, a := range args {
		if a == flag {
			return i+1 < len(args)
		}
	}
	return false
}
