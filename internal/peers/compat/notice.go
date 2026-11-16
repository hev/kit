package compat

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// RelayNotice uses the same incarnation-bound outbox and beacon receipts as
// interactive peers sends, with an explicit code-authored sender. It never
// falls through to SSH when a cached recipient host exists.
func RelayNotice(state, host, target, from, id, message string) (handled bool, err error) {
	p := newP(strings.NewReader(""), io.Discard, io.Discard)
	p.stateDir = state
	p.snapshots = filepath.Join(state, "hosts")
	p.outbox = filepath.Join(state, "outbox")
	p.queueLog = filepath.Join(state, "queued.jsonl")
	senderHost, sender, validSender := strings.Cut(from, "/")
	if !validSender {
		return true, fmt.Errorf("invalid notice sender")
	}
	id = strings.TrimPrefix(id, senderHost+":")
	paths := glob(filepath.Join(p.snapshots, "*.json"))
	for _, path := range paths {
		snap, e := readObj(path)
		if e != nil {
			continue
		}
		sh := snap.Str("host")
		// Historical dispatch reply addresses can contain the machine's long name.
		alias := strings.EqualFold(sh, host) || sh == "macbook" && strings.Contains(strings.ToLower(host), "macbook")
		if !alias {
			continue
		}
		for _, v := range snap.List("relayed") {
			rec, ok := v.(*Obj)
			if !ok {
				continue
			}
			if rec.Str("id") == id && rec.Str("from_host") == senderHost && rec.Str("to") == target && (strings.HasPrefix(rec.Str("status"), "held unread in inbox") || strings.HasPrefix(rec.Str("status"), "held in inbox")) {
				return true, nil
			}
		}
		for _, v := range snap.List("panes") {
			sp, ok := v.(*Obj)
			if !ok || sp.Str("address") != target {
				continue
			}
			at, _ := snap.Num("at")
			if now()-at > stale || !Truthy(sp.Get("inbox")) || !Truthy(sp.Get("recipient_incarnation")) || sp.Str("role") == "offline" {
				return true, fmt.Errorf("relay pending: recipient snapshot stale or has no active inbox")
			}

			cwd, _ := os.Getwd()
			record := NewObj("id", id, "ts", now(), "to_host", sh, "to", target, "recipient_incarnation", sp.Get("recipient_incarnation"), "inbox_only", true, "from_host", senderHost, "from", sender, "from_agent", "hev", "from_cwd", cwd, "replyable", true, "message", message, "force", false)
			// Preserve the first enqueue's content and binding across retries.
			for _, old := range inboxRows(p.queueLog) {
				if sameKey(old, record, "from_host", "id") {
					if old.Str("to") != target || old.Str("message") != message || old.Str("from") != sender {
						return true, fmt.Errorf("relay message ID reused with different content or recipient")
					}
					record = old
					break
				}
			}
			if _, e := p.queueRelay(record); e != nil {
				return true, e
			}
			return true, fmt.Errorf("relay pending: queued for recipient beacon; notices pull can also deliver")
		}
		return true, fmt.Errorf("relay pending: recipient absent from cached host")
	}
	return false, nil
}
