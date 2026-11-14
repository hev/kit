package kitapitest

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/pkg/kitapi"
)

type peersState struct {
	now      func() time.Time
	presence map[string]map[string]kitapi.PeerMachine
	messages map[string]map[string]kitapi.PeerMessage
}

// SetPeersClock sets the fake presence clock. Safe during requests.
func (s *Server) SetPeersClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers.now = now
}

var fullUUID = regexp.MustCompile(`^([0-9a-fA-F]{32}|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)
var address = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.:%-]+$`)

func decodePeer(w http.ResponseWriter, r *http.Request, v any) bool {
	b, e := io.ReadAll(io.LimitReader(r.Body, kitapi.PeersMaxBytes+1))
	if len(b) > kitapi.PeersMaxBytes {
		writeJSON(w, 413, map[string]string{"error": "peers body too large"})
		return false
	}
	if e != nil || json.Unmarshal(b, v) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return false
	}
	return true
}
func (s *Server) initPeers(mux *http.ServeMux) {
	s.peers = &peersState{now: time.Now, presence: map[string]map[string]kitapi.PeerMachine{}, messages: map[string]map[string]kitapi.PeerMessage{}}
	mux.HandleFunc("POST /v1/peers/presence", func(w http.ResponseWriter, r *http.Request) {
		var in kitapi.PresenceIn
		if !decodePeer(w, r, &in) {
			return
		}
		id := identity(r)
		if in.Host != id.Host {
			writeJSON(w, 403, map[string]string{"error": "host does not belong to key"})
			return
		}
		if in.Host == "" || in.ObservedAt.IsZero() {
			writeJSON(w, 400, map[string]string{"error": "host and observed_at required"})
			return
		}
		if len(in.Sessions) > kitapi.PeersMaxSessions {
			writeJSON(w, 413, map[string]string{"error": "too many sessions"})
			return
		}
		for _, v := range in.Sessions {
			if strings.ContainsAny(v.Project, "/\\") {
				writeJSON(w, 400, map[string]string{"error": "project must be a base name"})
				return
			}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		p := s.peers
		if p.presence[id.TeamID] == nil {
			p.presence[id.TeamID] = map[string]kitapi.PeerMachine{}
		}
		old, ok := p.presence[id.TeamID][in.Host]
		if !ok || in.ObservedAt.After(old.ObservedAt) {
			old = kitapi.PeerMachine{PresenceIn: in, AuthorID: id.AuthorID, ExpiresAt: p.now().Add(kitapi.PeersTTLSeconds * time.Second)}
			p.presence[id.TeamID][in.Host] = old
		}
		writeJSON(w, 200, kitapi.PresenceAck{TTLSeconds: kitapi.PeersTTLSeconds, ExpiresAt: old.ExpiresAt})
	})
	mux.HandleFunc("GET /v1/peers/presence", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := kitapi.PresencePage{TTLSeconds: kitapi.PeersTTLSeconds, Machines: []kitapi.PeerMachine{}}
		for _, v := range s.peers.presence[identity(r).TeamID] {
			if v.ExpiresAt.After(s.peers.now()) {
				out.Machines = append(out.Machines, v)
			}
		}
		sort.Slice(out.Machines, func(i, j int) bool { return out.Machines[i].Host < out.Machines[j].Host })
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /v1/peers/messages", func(w http.ResponseWriter, r *http.Request) {
		var in kitapi.PeerMessageIn
		if !decodePeer(w, r, &in) {
			return
		}
		id := identity(r)
		if !fullUUID.MatchString(in.ID) || !address.MatchString(in.Sender) || len(in.Recipients) == 0 || in.Time.IsZero() || in.Body == "" || (in.Kind != "direct" && in.Kind != "chat") || (in.Kind == "chat" && in.ChatID == "") || (in.Kind == "direct" && (in.ChatID != "" || in.Title != "")) {
			writeJSON(w, 400, map[string]string{"error": "invalid message"})
			return
		}
		if !strings.HasPrefix(in.Sender, id.Host+"/") {
			writeJSON(w, 403, map[string]string{"error": "sender host does not belong to key"})
			return
		}
		for _, a := range in.Recipients {
			if !address.MatchString(a) {
				writeJSON(w, 400, map[string]string{"error": "invalid recipient"})
				return
			}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		p := s.peers
		if old, ok := p.messages[id.TeamID][in.ID]; ok {
			if !reflect.DeepEqual(old.PeerMessageIn, in) {
				writeJSON(w, 409, map[string]string{"error": "message id conflict"})
				return
			}
			writeJSON(w, 200, old)
			return
		}
		if s.readOnly {
			writeJSON(w, 402, map[string]string{"error": "billing_read_only"})
			return
		}
		if p.messages[id.TeamID] == nil {
			p.messages[id.TeamID] = map[string]kitapi.PeerMessage{}
		}
		out := kitapi.PeerMessage{PeerMessageIn: in, AuthorID: id.AuthorID, StoredBytes: kitapi.PeerStoredBytes(in)}
		p.messages[id.TeamID][in.ID] = out
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("GET /v1/peers/messages", s.listPeersMessages)
}

type peerCursor struct {
	Team, Machine, Session, Chat string
	Time                         time.Time
	ID                           string
}

func (s *Server) listPeersMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 50
	if q.Has("limit") {
		n, e := strconv.Atoi(q.Get("limit"))
		if e != nil || n < 1 || n > 200 {
			writeJSON(w, 400, map[string]string{"error": "invalid limit"})
			return
		}
		limit = n
	}
	c := peerCursor{Team: identity(r).TeamID, Machine: q.Get("machine"), Session: q.Get("session"), Chat: q.Get("chat")}
	if raw := q.Get("cursor"); raw != "" {
		b, e := base64.RawURLEncoding.DecodeString(raw)
		var got peerCursor
		if e != nil || json.Unmarshal(b, &got) != nil || got.Team != c.Team || got.Machine != c.Machine || got.Session != c.Session || got.Chat != c.Chat || got.Time.IsZero() || got.ID == "" {
			writeJSON(w, 400, map[string]string{"error": "invalid cursor"})
			return
		}
		c = got
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := []kitapi.PeerMessage{}
	for _, v := range s.peers.messages[c.Team] {
		if !c.Time.IsZero() && (v.Time.After(c.Time) || (v.Time.Equal(c.Time) && v.ID >= c.ID)) {
			continue
		}
		if c.Chat != "" && v.ChatID != c.Chat {
			continue
		}
		machine, session := c.Machine == "", c.Session == ""
		for _, a := range append([]string{v.Sender}, v.Recipients...) {
			h, _, _ := strings.Cut(a, "/")
			machine = machine || h == c.Machine
			session = session || a == c.Session
		}
		if machine && session {
			rows = append(rows, v)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Time.Equal(rows[j].Time) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].Time.After(rows[j].Time)
	})
	out := kitapi.PeerMessagesPage{Messages: rows}
	if len(rows) > limit {
		out.Messages = rows[:limit]
		last := out.Messages[limit-1]
		c.Time, c.ID = last.Time, last.ID
		b, _ := json.Marshal(c)
		out.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	}
	writeJSON(w, 200, out)
}
