package peers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func (e *Engine) inbox(ctx context.Context, ps []Peer, args []string, session, sender, id string, asJSON, all, peek, pending, count, currentAll bool, limit int, in io.Reader, out io.Writer) error {
	op := ""
	if len(args) > 0 {
		op = args[0]
	}
	if op == "on" || op == "off" {
		if len(args) > 2 {
			return fmt.Errorf("inbox on|off [SESSION]")
		}
		if len(args) == 2 {
			session = args[1]
		}
		if session == "" {
			p, err := e.recipientOwner(ctx, ps)
			if err != nil {
				return err
			}
			session = p.Session
		}
		p, err := findInbox(ps, session)
		if err != nil {
			return err
		}
		if p.Run {
			return fmt.Errorf("dispatch runs always use their run inbox")
		}
		_, err = e.tmux(ctx, "set-option", "-t", "="+session+":", "@peers-inbox", op)
		if err == nil {
			fmt.Fprintf(out, "%s: inbox %s\n", session, op)
		}
		return err
	}
	var owner Peer
	mutating := op == "hold" || op == "consume" || op == "ack" || op == "owner" || !(peek || count || all || currentAll)
	if mutating || session == "" {
		p, err := e.recipientOwner(ctx, ps)
		if err != nil {
			return err
		}
		owner = p
		if session == "" {
			session = p.Session
		}
		if mutating && owner.Session != session {
			return fmt.Errorf("only the current recipient may consume or acknowledge")
		}
	}
	p, err := findInbox(ps, session)
	if err != nil && !all {
		return err
	}
	identity := p.Identity
	if op == "hold" {
		b, err := io.ReadAll(in)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(b)) == "" {
			return fmt.Errorf("empty message")
		}
		m, err := e.hold(session, identity, Message{"id": id, "from": sender, "header": "[held message from " + sender + "]", "message": strings.TrimRight(string(b), "\n")}, func() error {
			if p.Run {
				return nil
			}
			actual, err := e.identity(ctx, session)
			if err != nil || actual != identity {
				return fmt.Errorf("recipient lifetime changed before enqueue")
			}
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s: queued in inbox [id %s]; not acknowledged\n", session, m["id"])
		return nil
	}
	unlock, err := e.lock(session)
	if err != nil {
		return err
	}
	defer unlock()
	if mutating && !p.Run {
		actual, err := e.identity(ctx, session)
		if err != nil || actual != identity {
			return fmt.Errorf("recipient incarnation changed")
		}
	}
	emit := func(v any) error { return json.NewEncoder(out).Encode(v) }
	if op == "owner" {
		return emit(map[string]any{"session": session, "recipient_incarnation": identity, "on": p.Inbox})
	}
	if op == "consume" || op == "ack" {
		kind := "consumed"
		if op == "ack" {
			kind = "acknowledged"
		}
		if err := e.receipt(session, identity, kind, args[1:]); err != nil {
			return err
		}
		ids, seen := []string{}, map[string]bool{}
		for _, id := range args[1:] {
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		return emit(map[string]any{"session": session, kind: ids})
	}
	if op != "" {
		return fmt.Errorf("unknown inbox operation %q", op)
	}
	ms, err := e.view(session, identity)
	if err != nil {
		return err
	}
	unread, unacked, historical := 0, 0, 0
	shown := []Message{}
	for _, m := range ms {
		current := m["current"] == true
		if current {
			if m["consumed_at"] == nil {
				unread++
			}
			if m["acknowledged_at"] == nil {
				unacked++
			}
		} else {
			historical++
		}
		if all || (current && (currentAll || (pending && m["acknowledged_at"] == nil) || (!pending && m["consumed_at"] == nil))) {
			shown = append(shown, m)
		}
	}
	if count {
		fmt.Fprintln(out, unread)
		return nil
	}
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
	}
	if mutating {
		ids := []string{}
		for _, m := range shown {
			if m["consumed_at"] == nil {
				if id, ok := m["id"].(string); ok {
					ids = append(ids, id)
				}
			}
		}
		if len(ids) > 0 {
			if err := e.receipt(session, identity, "consumed", ids); err != nil {
				return err
			}
		}
	}
	if asJSON {
		return emit(map[string]any{"session": session, "recipient_incarnation": identity, "on": p.Inbox, "unread": unread, "unacknowledged": unacked, "historical": historical, "messages": shown})
	}
	for _, m := range shown {
		fmt.Fprintf(out, "--- %s [%s]\n%s\n%s\n", m["id"], m["transport_state"], m["header"], m["message"])
	}
	if len(shown) == 0 {
		fmt.Fprintf(out, "%s: inbox empty\n", session)
	}
	return nil
}

// Inbox identity belongs to the session even when several panes share it.
func findInbox(ps []Peer, session string) (Peer, error) {
	for _, p := range ps {
		if p.Session == session {
			return p, nil
		}
	}
	return Peer{}, fmt.Errorf("inbox session %q not found", session)
}

func (e *Engine) recipientOwner(ctx context.Context, ps []Peer) (Peer, error) {
	if _, present := os.LookupEnv("PEERS_INBOX_TOKEN"); present {
		return Peer{}, fmt.Errorf("runtime capability bindings are not supported by this native consumer")
	}
	return e.owner(ctx, ps)
}
