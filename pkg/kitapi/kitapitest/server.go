// Package kitapitest is an in-process fake of the kit API for tests: every
// route of the contract, backed by kit's own file archive and file board, so
// a client test exercises the same row shapes and answers a hosted server
// built on pkg/archive, pkg/dashboard and pkg/board gives.
//
// It is a test double, not a server to deploy: keys are a static map, there
// is one team, and nothing is metered.
package kitapitest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/pkg/archive"
	"github.com/hev/kit/pkg/board"
	"github.com/hev/kit/pkg/dashboard"
	"github.com/hev/kit/pkg/kitapi"
)

// Identity is what a key stands for. The server stamps these on every row
// and post, whatever the body says.
type Identity struct {
	KeyID      string
	AuthorID   string
	AuthorName string
	Host       string
	TeamID     string
	TeamSlug   string
}

// Server is the fake. Its fields are safe to read after the test's requests
// have finished; use the methods while requests may be in flight.
type Server struct {
	*httptest.Server
	Traces *archive.FileStore
	Board  *board.Store

	mu       sync.Mutex
	keys     map[string]Identity
	down     bool
	limited  int
	requests map[string]int
	sessions map[string]string // session id -> author id
}

// New starts a fake kit API with one key. Close it with t.Cleanup (done
// for you).
func New(t testing.TB, key string, id Identity) *Server {
	t.Helper()
	dir := t.TempDir()
	traces, err := archive.OpenFiles(filepath.Join(dir, "traces"), "team")
	if err != nil {
		t.Fatal(err)
	}
	st, err := board.OpenFiles(filepath.Join(dir, "board"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Traces: traces, Board: st, keys: map[string]Identity{key: id}, requests: map[string]int{}, sessions: map[string]string{}}
	boardAPI, err := board.APIHandler(st, board.HandlerOptions{Author: func(r *http.Request) (board.Poster, error) {
		id, _ := r.Context().Value(identityKey{}).(Identity)
		return board.Poster{Author: id.AuthorName, Role: "human", Host: id.Host}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	reads := dashboard.NewTracesHandler(traces)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/whoami", func(w http.ResponseWriter, r *http.Request) {
		id := identity(r)
		writeJSON(w, 200, map[string]any{"key_id": id.KeyID, "hostname": id.Host,
			"team": map[string]string{"id": id.TeamID, "slug": id.TeamSlug}, "author": map[string]string{"id": id.AuthorID, "name": id.AuthorName}})
	})
	mux.HandleFunc("POST /v1/traces/sessions", func(w http.ResponseWriter, r *http.Request) {
		rows, ok := decodeBatch[archive.SessionRow](w, r)
		if !ok {
			return
		}
		id := identity(r)
		s.mu.Lock()
		for i := range rows {
			if owner, seen := s.sessions[rows[i].SessionID]; seen && owner != id.AuthorID {
				s.mu.Unlock()
				writeJSON(w, 403, map[string]string{"error": "session belongs to another author"})
				return
			}
			s.sessions[rows[i].SessionID] = id.AuthorID
			rows[i].AuthorID, rows[i].AuthorName, rows[i].Host = id.AuthorID, id.AuthorName, id.Host
		}
		s.mu.Unlock()
		if _, err := traces.WriteSessions(rows); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]int{"accepted": len(rows)})
	})
	mux.HandleFunc("POST /v1/traces/blocks", func(w http.ResponseWriter, r *http.Request) {
		rows, ok := decodeBatch[archive.BlockRow](w, r)
		if !ok {
			return
		}
		if _, err := traces.WriteBlocks(rows); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]int{"accepted": len(rows)})
	})
	mux.HandleFunc("POST /v1/traces/chunks", func(w http.ResponseWriter, r *http.Request) {
		rows, ok := decodeBatch[archive.Row](w, r)
		if !ok {
			return
		}
		host := identity(r).Host
		for i := range rows {
			rows[i].Host = host
		}
		if _, err := traces.Write(rows); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]int{"accepted": len(rows)})
	})
	mux.HandleFunc("DELETE /v1/traces/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("id")
		s.mu.Lock()
		owner, seen := s.sessions[sid]
		s.mu.Unlock()
		if !seen {
			writeJSON(w, 404, map[string]string{"error": "no such session"})
			return
		}
		if owner != identity(r).AuthorID {
			writeJSON(w, 403, map[string]string{"error": "not your session"})
			return
		}
		if err := traces.DeleteSessionArchive(sid); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		s.mu.Lock()
		delete(s.sessions, sid)
		s.mu.Unlock()
		writeJSON(w, 200, map[string]bool{"deleted": true})
	})
	mux.Handle("GET /v1/traces/", http.StripPrefix("/v1/traces", reads))
	mux.Handle("/v1/board/", http.StripPrefix("/v1/board", boardAPI))
	mux.HandleFunc("POST /v1/session", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"url": s.URL + "/s/one-time-" + identity(r).KeyID})
	})

	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests[r.Method+" "+r.URL.Path]++
		down, limited := s.down, s.limited
		if limited > 0 {
			s.limited--
		}
		s.mu.Unlock()
		if down {
			writeJSON(w, 503, map[string]string{"error": "down for the test"})
			return
		}
		if limited > 0 {
			w.Header().Set("Retry-After", "0")
			writeJSON(w, 429, map[string]string{"error": "slow down"})
			return
		}
		auth, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		s.mu.Lock()
		id, known := s.keys[auth]
		s.mu.Unlock()
		if !ok || !known {
			writeJSON(w, 401, map[string]string{"error": "unknown kit key"})
			return
		}
		mux.ServeHTTP(w, r.WithContext(withIdentity(r.Context(), id)))
	}))
	t.Cleanup(s.Close)
	return s
}

// API is the fake's base URL, the [kit] api value.
func (s *Server) API() string { return s.URL }

// Client is a kitapi client for key with fast retries.
func (s *Server) Client(key string) *kitapi.Client {
	c := kitapi.New(s.URL, key)
	c.Backoff = func(int) time.Duration { return 0 }
	return c
}

// SetDown makes every request answer 503 until called with false.
func (s *Server) SetDown(down bool) {
	s.mu.Lock()
	s.down = down
	s.mu.Unlock()
}

// RateLimit answers the next n requests with 429 and Retry-After: 0.
func (s *Server) RateLimit(n int) {
	s.mu.Lock()
	s.limited = n
	s.mu.Unlock()
}

// Revoke forgets a key; later requests with it answer 401.
func (s *Server) Revoke(key string) {
	s.mu.Lock()
	delete(s.keys, key)
	s.mu.Unlock()
}

// AddKey adds another key, for tests with two authors.
func (s *Server) AddKey(key string, id Identity) {
	s.mu.Lock()
	s.keys[key] = id
	s.mu.Unlock()
}

// Requests is how many requests reached method+" "+path (whatever the answer).
func (s *Server) Requests(methodPath string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[methodPath]
}

func decodeBatch[T any](w http.ResponseWriter, r *http.Request) ([]T, bool) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, kitapi.MaxBatchBytes+1))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return nil, false
	}
	if len(raw) > kitapi.MaxBatchBytes {
		writeJSON(w, 413, map[string]string{"error": fmt.Sprintf("batch over %d bytes", kitapi.MaxBatchBytes)})
		return nil, false
	}
	var rows []T
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '[' {
		err = json.Unmarshal(raw, &rows)
	} else {
		var body struct {
			Rows []T `json:"rows"`
		}
		err = json.Unmarshal(raw, &body)
		rows = body.Rows
	}
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad batch: " + err.Error()})
		return nil, false
	}
	if len(rows) > kitapi.MaxBatchRows {
		writeJSON(w, 413, map[string]string{"error": "batch over " + strconv.Itoa(kitapi.MaxBatchRows) + " rows"})
		return nil, false
	}
	return rows, true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
