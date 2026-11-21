package peers

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/hev/kit/internal/dispatch"
)

type apiRequest struct {
	Action     string          `json:"action"`
	ID         string          `json:"id"`
	CallerHost string          `json:"caller_host"`
	Body       json.RawMessage `json:"body"`
}

var apiPeerID = regexp.MustCompile(`^[A-Za-z0-9._-]+:[A-Za-z0-9._%:-]+$`)
var apiSender = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._:-]+$`)

// HandleAPIRPC is for a trusted execution account. It exposes inbox-only
// delivery and never calls the interactive terminal paste path.
func HandleAPIRPC(ctx context.Context, raw string) dispatch.Response {
	var request apiRequest
	if len(raw) > 8<<20 || json.Unmarshal([]byte(raw), &request) != nil {
		return dispatch.Response{Error: "invalid peers request"}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	status, body := New().apiExecute(ctx, request)
	data, err := json.Marshal(map[string]any{"status": status, "body": body})
	if err != nil {
		return dispatch.Response{Error: "peers response unavailable"}
	}
	return dispatch.Response{Log: string(data)}
}

func (e *Engine) apiExecute(ctx context.Context, q apiRequest) (int, any) {
	if q.Action != "list" && !apiPeerID.MatchString(q.ID) {
		return 400, nil
	}
	host, name, _ := strings.Cut(q.ID, ":")
	if q.Action != "list" && host != e.Host {
		return 404, nil
	}
	peers, err := e.list(ctx, false)
	if err != nil {
		return 503, nil
	}
	if q.Action == "list" {
		result := []Message{}
		seen := map[string]bool{}
		for _, peer := range peers {
			id := strings.Replace(peer.Address, "/", ":", 1)
			if apiPeerID.MatchString(id) && !seen[id] {
				result = append(result, e.apiPresence(ctx, peer, id))
				seen[id] = true
			}
		}
		return 200, result
	}
	var peer Peer
	for _, candidate := range peers {
		if candidate.Address == host+"/"+name {
			peer = candidate
			break
		}
	}
	if peer.Address == "" {
		return 404, nil
	}
	switch q.Action {
	case "get":
		return 200, e.apiPresence(ctx, peer, q.ID)
	case "put":
		var body Message
		if json.Unmarshal(q.Body, &body) != nil || body == nil {
			return 400, nil
		}
		if q.CallerHost != host || body["recipient_incarnation"] != peer.Identity {
			return 403, nil
		}
		return 200, e.apiPresence(ctx, peer, q.ID)
	case "messages":
		ms, err := e.view(peer.Session, peer.Identity)
		if err != nil {
			return 503, nil
		}
		if ms == nil {
			ms = []Message{}
		}
		status := apiInboxStatus(ms, peer.Identity)
		for _, m := range ms {
			formatAPIMessage(m, peer)
		}
		return 200, Message{"data": ms, "inbox_status": status}
	case "send", "deliver":
		return e.apiSend(ctx, q, peer)
	case "receipts":
		return e.apiReceipts(ctx, q, peer)
	default:
		return 400, nil
	}
}

func (e *Engine) apiPresence(ctx context.Context, p Peer, id string) Message {
	host, name, _ := strings.Cut(id, ":")
	var agent any
	if p.Agent != "" {
		agent = p.Agent
	}
	state := p.State
	if state == "run" {
		state = "working"
	}
	m := Message{"id": id, "host": host, "name": name, "address": p.Address, "agent": agent, "state": state,
		"agent_pid": p.PID, "recipient_incarnation": p.Identity, "headless": p.Headless,
		"inbox": p.Inbox, "last_seen": time.Now().UTC(), "stale": false}
	if p.Run {
		m["dispatch"] = p.Session
	} else if role, err := e.tmux(ctx, "show-options", "-qv", "-t", "="+p.Session, "@peers-role"); err == nil {
		switch strings.TrimSpace(role) {
		case "foh", "boh", "offline":
			m["role"] = strings.TrimSpace(role)
		}
	}
	if ms, err := e.view(p.Session, p.Identity); err == nil {
		m["inbox_status"] = apiInboxStatus(ms, p.Identity)
	}
	return m
}

func (e *Engine) apiSend(ctx context.Context, q apiRequest, p Peer) (int, any) {
	var body struct {
		ID          string `json:"id"`
		From        string `json:"from"`
		Text        string `json:"message"`
		Incarnation string `json:"recipient_incarnation"`
		Order       string `json:"order"`
		Run         string `json:"run"`
		Thread      string `json:"thread"`
		Title       string `json:"title"`
		Agent       string `json:"from_agent"`
	}
	if json.Unmarshal(q.Body, &body) != nil || !apiSender.MatchString(body.From) || strings.TrimSpace(body.Text) == "" || len(body.Text) > 4_000_000 || len(body.ID) > 640 {
		return 400, nil
	}
	if presence := e.apiPresence(ctx, p, q.ID); presence["role"] == "offline" {
		return 403, nil
	}
	if !strings.HasPrefix(body.From, q.CallerHost+"/") || !p.Inbox {
		return 403, nil
	}
	if body.Incarnation != "" && body.Incarnation != p.Identity {
		return 409, nil
	}
	m := Message{"id": body.ID, "from": body.From, "to": p.Address, "header": messageHeader(body.From), "message": body.Text}
	for key, value := range map[string]string{"order": body.Order, "run": body.Run, "thread": body.Thread, "title": body.Title, "from_agent": body.Agent} {
		if value != "" {
			m[key] = value
		}
	}
	held, err := e.hold(p.Session, p.Identity, m, func() error {
		if p.Run {
			return nil
		}
		id, err := e.identity(ctx, p.Session)
		if err != nil || id != p.Identity {
			return errPeerLifetime
		}
		return nil
	})
	if err != nil {
		return 409, nil
	}
	status := 200
	if held["ts"] == m["ts"] && m["ts"] != nil {
		status = 201
		if p.Run {
			_, err := dispatch.Handle(dispatch.Request{APILocalOnly: true, Action: "peer-send", Name: p.Session, Note: messageHeader(body.From) + "\n" + body.Text})
			if err != nil {
				return 503, nil
			}
		}
	}
	formatAPIMessage(held, p)
	return status, held
}

var errPeerLifetime = &peerLifetimeError{}

type peerLifetimeError struct{}

func (*peerLifetimeError) Error() string { return "peer lifetime changed" }

func (e *Engine) apiReceipts(ctx context.Context, q apiRequest, p Peer) (int, any) {
	host, _, _ := strings.Cut(q.ID, ":")
	if q.CallerHost != host {
		return 403, nil
	}
	var body struct {
		Kind        string   `json:"kind"`
		IDs         []string `json:"ids"`
		Incarnation string   `json:"recipient_incarnation"`
	}
	if json.Unmarshal(q.Body, &body) != nil || (body.Kind != "consumed" && body.Kind != "acknowledged") || len(body.IDs) == 0 || len(body.IDs) > 64 {
		return 400, nil
	}
	unlock, err := e.lock(p.Session)
	if err != nil {
		return 503, nil
	}
	defer unlock()
	current := p.Identity
	if !p.Run {
		current, _ = e.identity(ctx, p.Session)
	}
	recorded := []string{}
	rejected := []Message{}
	for _, id := range body.IDs {
		if current == "" || body.Incarnation != current {
			rejected = append(rejected, Message{"id": id, "error": "stale incarnation"})
		} else if e.receipt(p.Session, current, body.Kind, []string{id}) != nil {
			rejected = append(rejected, Message{"id": id, "error": "invalid receipt"})
		} else {
			recorded = append(recorded, id)
		}
	}
	return 200, Message{"kind": body.Kind, "recorded": recorded, "rejected": rejected}
}

func apiInboxStatus(ms []Message, identity string) Message {
	unread, unacked, historical := 0, 0, 0
	for _, m := range ms {
		if m["current"] != true {
			historical++
		} else {
			if m["consumed_at"] == nil {
				unread++
			}
			if m["acknowledged_at"] == nil {
				unacked++
			}
		}
	}
	return Message{"recipient_incarnation": identity, "unread": unread, "unacknowledged": unacked, "historical": historical}
}

func formatAPIMessage(m Message, p Peer) {
	m["current"] = m["recipient_incarnation"] == p.Identity
	if m["to"] == nil {
		m["to"] = p.Address
	}
	if m["consumed_at"] != nil && m["transport_state"] == "queued" {
		m["transport_state"] = "consumed"
	}
	for _, field := range []string{"ts", "origin_ts", "consumed_at", "acknowledged_at"} {
		if stamp, ok := m[field].(float64); ok {
			sec := int64(stamp)
			m[field] = time.Unix(sec, int64((stamp-float64(sec))*1e9)).UTC()
		} else if m[field] == nil {
			delete(m, field)
		}
	}
	delete(m, "recipient_session")
}
