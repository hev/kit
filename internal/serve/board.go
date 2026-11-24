package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hev/kit/pkg/kitapi"
)

// The Board tab reads the team board through the board's own JSON API
// (internal/board/server), so hev serve and hev board serve cannot disagree
// about what a thread is. It is read-only: only the routes below are mounted,
// only GET reaches them, and reading a thread never counts as a view.
//
// Board errors never reach the browser as they are: a store error can carry
// a file path or an endpoint, so the page gets a fixed message per status.

// maxBoardBody caps one board response (a thread, a list, an image) held in
// memory on its way to the browser.
const maxBoardBody = 32 << 20

// DefaultBoardTimeout bounds one call to a remote board server.
const DefaultBoardTimeout = 20 * time.Second

// Board is where the Board tab reads from. Build one with LocalBoard,
// RemoteBoard or UnavailableBoard and hand it to Server.WithBoard.
type Board struct {
	mode string // filesystem, layer, remote or unavailable
	get  func(ctx context.Context, path string, q url.Values) (*boardReply, error)
}

type boardReply struct {
	status int
	ctype  string
	body   []byte
}

var errBoardUnavailable = errors.New("board unavailable")

// LocalBoard reads through the board's HTTP handler in this process: this
// host's files, or the Layer namespace when a key is set. mode is the
// store's mode ("filesystem" or "layer") and is shown on the page.
func LocalBoard(api http.Handler, mode string) *Board {
	return &Board{mode: mode, get: func(ctx context.Context, path string, q url.Values) (*boardReply, error) {
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, (&url.URL{Path: path, RawQuery: q.Encode()}).RequestURI(), nil)
		req.RemoteAddr = "127.0.0.1:0"
		rec := &cappedRecorder{header: http.Header{}}
		api.ServeHTTP(rec, req)
		if rec.over {
			return nil, errBoardUnavailable
		}
		if rec.code == 0 {
			rec.code = http.StatusOK
		}
		return &boardReply{status: rec.code, ctype: rec.header.Get("Content-Type"), body: rec.body.Bytes()}, nil
	}}
}

// RemoteBoard reads from a board server over HTTP (board.url): the shared
// board a team runs. Every call is bounded by timeout.
func RemoteBoard(base string, timeout time.Duration) (*Board, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("board server URL must be an http(s) URL")
	}
	if timeout <= 0 {
		timeout = DefaultBoardTimeout
	}
	client := &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Board{mode: "remote", get: func(ctx context.Context, path string, q url.Values) (*boardReply, error) {
		target := *u
		target.Path = u.Path + path
		target.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return nil, errBoardUnavailable
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, errBoardUnavailable
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBoardBody+1))
		if err != nil || len(body) > maxBoardBody {
			return nil, errBoardUnavailable
		}
		return &boardReply{status: resp.StatusCode, ctype: resp.Header.Get("Content-Type"), body: body}, nil
	}}, nil
}

// HostedBoard reads the signed-in team’s board through the Club Kit API.
func HostedBoard(client *kitapi.Client) *Board {
	return &Board{mode: "remote", get: func(ctx context.Context, path string, q url.Values) (*boardReply, error) {
		if !strings.HasPrefix(path, "/api/") {
			return &boardReply{status: http.StatusNotFound}, nil
		}
		raw, err := client.DoRaw(ctx, http.MethodGet, "/v1/board"+strings.TrimPrefix(path, "/api"), q, nil)
		var remote *kitapi.Error
		if errors.As(err, &remote) {
			return &boardReply{status: remote.Status}, nil
		}
		if err != nil || len(raw) > maxBoardBody {
			return nil, errBoardUnavailable
		}
		return &boardReply{status: http.StatusOK, ctype: "application/json", body: raw}, nil
	}}
}

// UnavailableBoard keeps the Board tab and says the board could not be
// opened; the reason belongs in hev serve's own startup output, not the page.
func UnavailableBoard() *Board {
	return &Board{mode: "unavailable", get: func(context.Context, string, url.Values) (*boardReply, error) {
		return nil, errBoardUnavailable
	}}
}

// WithBoard shows the Board tab and serves its read-only API under
// /api/board/. Without it the tab is hidden and those routes are 404.
func (s *Server) WithBoard(b *Board) *Server { s.board = b; return s }

// Tab is a navigation entry a wrapper adds beside Traces and Board, for a
// page the wrapper serves itself (for example a Pro view mounted in front of
// this handler).
type Tab struct {
	Label string
	Href  string
}

// WithTabs adds wrapper tabs after Traces and Board. Only relative paths and
// http(s) URLs are kept.
func (s *Server) WithTabs(tabs ...Tab) *Server {
	for _, t := range tabs {
		if strings.TrimSpace(t.Label) == "" || !safeHref(t.Href) {
			continue
		}
		s.tabs = append(s.tabs, t)
	}
	return s
}

func safeHref(h string) bool {
	if strings.HasPrefix(h, "/") && !strings.HasPrefix(h, "//") {
		return true
	}
	u, err := url.Parse(h)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

func (s *Server) tabsHTML() string {
	var b strings.Builder
	for _, t := range s.tabs {
		fmt.Fprintf(&b, `<a class="tab" href="%s">%s</a>`, html.EscapeString(t.Href), html.EscapeString(t.Label))
	}
	return b.String()
}

var (
	boardThreadPath = regexp.MustCompile(`^threads/([0-9]{1,18})$`)
	boardBlobPath   = regexp.MustCompile(`^blob/([0-9a-f]{64})$`)
	// The query parameters each route forwards; anything else is dropped.
	boardParams = map[string][]string{
		"boards":  nil,
		"storage": nil,
		"threads": {"board", "tag", "limit", "offset"},
		"search":  {"q", "board", "tag", "top", "since"},
	}
)

// boardAPI answers GET /api/board/{path...} from the configured board.
func (s *Server) boardAPI(w http.ResponseWriter, r *http.Request) {
	if s.board == nil {
		http.NotFound(w, r)
		return
	}
	rest := r.PathValue("path")
	q := url.Values{}
	upstream := ""
	blob := false
	if m := boardThreadPath.FindStringSubmatch(rest); m != nil {
		upstream = "/api/threads/" + m[1]
		q.Set("peek", "1") // reading in hev serve is not a view
	} else if m := boardBlobPath.FindStringSubmatch(rest); m != nil {
		upstream, blob = "/blob/"+m[1], true
	} else if keep, ok := boardParams[rest]; ok {
		upstream = "/api/" + rest
		for _, k := range keep {
			for _, v := range r.URL.Query()[k] {
				q.Add(k, v)
			}
		}
	} else {
		writeError(w, errors.New("not found"), http.StatusNotFound)
		return
	}
	if rest == "search" && strings.TrimSpace(q.Get("q")) == "" {
		writeError(w, errors.New("search needs words to look for"), http.StatusBadRequest)
		return
	}
	reply, err := s.board.get(r.Context(), upstream, q)
	if err != nil {
		writeError(w, errBoardUnavailable, http.StatusBadGateway)
		return
	}
	switch {
	case reply.status == http.StatusNotFound:
		writeError(w, errors.New("not found"), http.StatusNotFound)
		return
	case reply.status == http.StatusBadRequest:
		writeError(w, errors.New("the board refused this request"), http.StatusBadRequest)
		return
	case reply.status < 200 || reply.status > 299:
		writeError(w, errBoardUnavailable, http.StatusBadGateway)
		return
	}
	if blob {
		ctype := reply.ctype
		if !strings.HasPrefix(ctype, "image/") || strings.ContainsAny(ctype, ";,") {
			writeError(w, errBoardUnavailable, http.StatusBadGateway)
			return
		}
		h := w.Header()
		h.Set("Content-Type", ctype)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "private, max-age=31536000, immutable")
		h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		_, _ = w.Write(reply.body)
		return
	}
	if !json.Valid(reply.body) {
		writeError(w, errBoardUnavailable, http.StatusBadGateway)
		return
	}
	if rest == "storage" {
		// The board's own storage answer is {mode}; say who serves it too.
		writeJSON(w, map[string]any{"mode": s.board.mode, "store": modeOf(reply.body)})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	addTiming(w, nil)
	_, _ = w.Write(reply.body)
}

func modeOf(body []byte) string {
	var v struct {
		Mode string `json:"mode"`
	}
	_ = json.Unmarshal(body, &v)
	if v.Mode == "filesystem" || v.Mode == "layer" {
		return v.Mode
	}
	return ""
}

func (s *Server) boardPage() map[string]any {
	if s.board == nil {
		return map[string]any{"enabled": false}
	}
	return map[string]any{"enabled": true, "mode": s.board.mode}
}

// cappedRecorder is an in-process response that stops holding bytes past
// maxBoardBody.
type cappedRecorder struct {
	header http.Header
	code   int
	body   bytes.Buffer
	over   bool
}

func (c *cappedRecorder) Header() http.Header { return c.header }
func (c *cappedRecorder) WriteHeader(code int) {
	if c.code == 0 {
		c.code = code
	}
}
func (c *cappedRecorder) Write(p []byte) (int, error) {
	if c.code == 0 {
		c.code = http.StatusOK
	}
	if c.body.Len()+len(p) > maxBoardBody {
		c.over = true
		return 0, errors.New("board response too large")
	}
	return c.body.Write(p)
}
