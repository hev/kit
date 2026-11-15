// Package slackfake is an in-process stand-in for the three Slack Web API
// methods kit calls. It exists for tests only: kit's tests never reach Slack.
// Behaviour follows the documentation pages cited beside each rule.
package slackfake

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
)

// Message is one accepted chat.postMessage.
type Message struct {
	Channel  string
	TS       string
	ThreadTS string
	Text     string
}

// Request is one recorded call, with the raw body and Authorization header,
// so tests can prove what did and did not leave the process.
type Request struct {
	Method string
	Auth   string
	Body   string
}

// Channel is a conversation the fake knows.
type Channel struct {
	Member   bool
	Archived bool
}

// Server is a fake Slack Web API.
type Server struct {
	*httptest.Server
	mu        sync.Mutex
	tokens    map[string]bool // token -> revoked
	channels  map[string]*Channel
	readScope bool
	team      string
	next      int64
	messages  []Message
	requests  []Request
	// Failure injection, consumed in order by chat.postMessage.
	rateLimit   []int    // Retry-After seconds per 429
	failCodes   []string // ok:false error codes
	serverError int
	threadsOff  bool
}

// New starts a fake with one valid token and the given channels, where the
// app is a member of each. Close it with t.Cleanup(s.Close).
func New(token string, channels ...string) *Server {
	s := &Server{tokens: map[string]bool{token: false}, channels: map[string]*Channel{}, readScope: true, team: "Example", next: 1700000000000100}
	for _, c := range channels {
		s.channels[c] = &Channel{Member: true}
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// URL is the base URL a notify.Slack client should use.
func (s *Server) BaseURL() string { return s.Server.URL + "/api/" }

func (s *Server) AddToken(token string) { s.mu.Lock(); s.tokens[token] = false; s.mu.Unlock() }
func (s *Server) Revoke(token string)   { s.mu.Lock(); s.tokens[token] = true; s.mu.Unlock() }
func (s *Server) SetReadScope(ok bool)  { s.mu.Lock(); s.readScope = ok; s.mu.Unlock() }
func (s *Server) RefuseThreads(on bool) { s.mu.Lock(); s.threadsOff = on; s.mu.Unlock() }
func (s *Server) ServerErrors(n int)    { s.mu.Lock(); s.serverError = n; s.mu.Unlock() }
func (s *Server) RateLimit(secs ...int) {
	s.mu.Lock()
	s.rateLimit = append(s.rateLimit, secs...)
	s.mu.Unlock()
}
func (s *Server) FailWith(codes ...string) {
	s.mu.Lock()
	s.failCodes = append(s.failCodes, codes...)
	s.mu.Unlock()
}

// SetChannel adds or changes a channel.
func (s *Server) SetChannel(id string, c Channel) {
	s.mu.Lock()
	s.channels[id] = &c
	s.mu.Unlock()
}

// RemoveChannel makes a channel unknown, as when it is deleted.
func (s *Server) RemoveChannel(id string) { s.mu.Lock(); delete(s.channels, id); s.mu.Unlock() }

// Messages returns accepted posts in order.
func (s *Server) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message(nil), s.messages...)
}

// Requests returns every call in order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

func reply(w http.ResponseWriter, v map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code string) { reply(w, map[string]any{"ok": false, "error": code}) }

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	method := strings.TrimPrefix(r.URL.Path, "/api/")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, Request{Method: method, Auth: r.Header.Get("Authorization"), Body: string(body)})
	if r.Method != http.MethodPost {
		fail(w, "invalid_post_type")
		return
	}
	// Bot tokens are sent as "Authorization: Bearer xoxb-..."
	// (https://docs.slack.dev/authentication/tokens).
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		fail(w, "not_authed")
		return
	}
	revoked, known := s.tokens[token]
	if !known {
		fail(w, "invalid_auth")
		return
	}
	if revoked {
		fail(w, "token_revoked")
		return
	}
	switch method {
	case "auth.test":
		// auth.test needs no scope and names the workspace
		// (https://docs.slack.dev/reference/methods/auth.test).
		reply(w, map[string]any{"ok": true, "team": s.team, "team_id": "T00000000", "user": "kit", "bot_id": "B00000000"})
	case "conversations.info":
		// Requires channels:read/groups:read; otherwise missing_scope
		// (https://docs.slack.dev/reference/methods/conversations.info).
		form, _ := url.ParseQuery(string(body))
		if !s.readScope {
			fail(w, "missing_scope")
			return
		}
		c, ok := s.channels[form.Get("channel")]
		if !ok {
			fail(w, "channel_not_found")
			return
		}
		reply(w, map[string]any{"ok": true, "channel": map[string]any{"id": form.Get("channel"), "is_member": c.Member, "is_archived": c.Archived}})
	case "chat.postMessage":
		s.post(w, body)
	default:
		fail(w, "unknown_method")
	}
}

func (s *Server) post(w http.ResponseWriter, body []byte) {
	if len(s.rateLimit) > 0 {
		// HTTP 429 with Retry-After seconds
		// (https://docs.slack.dev/apis/web-api/rate-limits).
		w.Header().Set("Retry-After", fmt.Sprint(s.rateLimit[0]))
		s.rateLimit = s.rateLimit[1:]
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	if s.serverError > 0 {
		s.serverError--
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if len(s.failCodes) > 0 {
		code := s.failCodes[0]
		s.failCodes = s.failCodes[1:]
		fail(w, code)
		return
	}
	var in struct {
		Channel  string `json:"channel"`
		Text     string `json:"text"`
		ThreadTS string `json:"thread_ts"`
	}
	if json.Unmarshal(body, &in) != nil {
		fail(w, "invalid_json")
		return
	}
	// Error codes from https://docs.slack.dev/reference/methods/chat.postMessage.
	c, ok := s.channels[in.Channel]
	switch {
	case !ok:
		fail(w, "channel_not_found")
		return
	case c.Archived:
		fail(w, "is_archived")
		return
	case !c.Member:
		// Without chat:write.public an app must be in the channel.
		fail(w, "not_in_channel")
		return
	case strings.TrimSpace(in.Text) == "":
		fail(w, "no_text")
		return
	}
	if in.ThreadTS != "" {
		root := false
		for _, m := range s.messages {
			if m.Channel == in.Channel && m.TS == in.ThreadTS && m.ThreadTS == "" {
				root = true
			}
		}
		if !root || s.threadsOff {
			fail(w, "cannot_reply_to_message")
			return
		}
	}
	ts := fmt.Sprintf("%d.%06d", s.next/1000000, s.next%1000000)
	s.next += 100
	s.messages = append(s.messages, Message{Channel: in.Channel, TS: ts, ThreadTS: in.ThreadTS, Text: in.Text})
	reply(w, map[string]any{"ok": true, "channel": in.Channel, "ts": ts})
}
