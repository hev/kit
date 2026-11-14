// Package kitapi is a typed client for the kit API: the HTTP API a signed-in
// machine talks to with its kit key (hk_…), and nothing else. It covers
// capture (batched session, block and chunk rows), trace reads and search,
// the board, and one-time dashboard links.
//
// Every call carries Authorization: Bearer <key>. 429 and 5xx answers and
// transport errors are retried with backoff (429 honours Retry-After); other
// answers return an *Error at once.
package kitapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/pkg/archive"
	"github.com/hev/kit/pkg/board"
)

// Batch limits; Post* split larger inputs.
const (
	MaxBatchRows  = archive.MaxBatchRows
	MaxBatchBytes = archive.MaxBatchBytes
)

// ErrUnauthorized is a 401: the key was revoked or never valid.
var ErrUnauthorized = errors.New("kit API rejected the kit key (401): run `hev login` again")

// Error is a non-2xx answer the client did not retry (or stopped retrying).
type Error struct {
	Status     int
	Message    string
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("kit API: HTTP %d", e.Status)
	}
	return fmt.Sprintf("kit API: HTTP %d: %s", e.Status, e.Message)
}

// Retryable reports whether err is worth trying again later: a transport
// error, a 429 or a 5xx. A 4xx such as 400 or 413 is not.
func Retryable(err error) bool {
	if err == nil || errors.Is(err, ErrUnauthorized) || errors.Is(err, context.Canceled) {
		return false
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Status == http.StatusTooManyRequests || e.Status >= 500
	}
	return true
}

// Client talks to one kit API with one kit key. The zero value is not
// usable; use New.
type Client struct {
	API  string
	key  string
	HTTP *http.Client
	// Retries is how many times a retryable failure is retried (default 4).
	Retries int
	// Backoff is the wait before retry n (1-based) when the server gave no
	// Retry-After. Default: 250ms doubling, with jitter, capped at 10s.
	Backoff func(n int) time.Duration
	// MaxWait caps any single wait, including Retry-After (default 30s).
	MaxWait time.Duration
}

// New is a client for api (the [kit] api URL) with key.
func New(api, key string) *Client {
	return &Client{API: strings.TrimRight(api, "/"), key: key, HTTP: &http.Client{Timeout: 60 * time.Second}, Retries: 4, MaxWait: 30 * time.Second}
}

func defaultBackoff(n int) time.Duration {
	d := 250 * time.Millisecond << min(n-1, 6)
	if d > 10*time.Second {
		d = 10 * time.Second
	}
	return d/2 + time.Duration(rand.Int64N(int64(d/2)+1))
}

// Do sends one request and decodes a JSON answer into out (when non-nil).
// path is relative to the API root and must start with "/". body, when
// non-nil, is sent as JSON.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	raw, err := c.DoRaw(ctx, method, path, query, payload)
	if err != nil || out == nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("kit API %s %s: decode: %w", method, path, err)
	}
	return nil
}

// DoRaw is Do with the body already encoded and the answer returned as is.
func (c *Client) DoRaw(ctx context.Context, method, path string, query url.Values, payload []byte) ([]byte, error) {
	if c.API == "" || c.key == "" {
		return nil, errors.New("kit API: not signed in (no api or key)")
	}
	u := c.API + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	retries := c.Retries
	backoff := c.Backoff
	if backoff == nil {
		backoff = defaultBackoff
	}
	maxWait := c.MaxWait
	if maxWait <= 0 {
		maxWait = 30 * time.Second
	}
	for attempt := 0; ; attempt++ {
		raw, err := c.once(ctx, method, u, payload)
		if err == nil || attempt >= retries || !Retryable(err) {
			return raw, err
		}
		wait := backoff(attempt + 1)
		var e *Error
		if errors.As(err, &e) && e.RetryAfter > 0 {
			wait = e.RetryAfter
		}
		if wait > maxWait {
			wait = maxWait
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
	}
}

func (c *Client) once(ctx context.Context, method, u string, payload []byte) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		e := &Error{Status: resp.StatusCode, Message: errorMessage(raw)}
		if s := resp.Header.Get("Retry-After"); s != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n >= 0 {
				e.RetryAfter = time.Duration(n) * time.Second
			} else if t, err := http.ParseTime(s); err == nil {
				e.RetryAfter = time.Until(t)
			}
		}
		return nil, e
	}
	return raw, nil
}

func errorMessage(raw []byte) string {
	var v struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &v) == nil && v.Error != "" {
		return v.Error
	}
	s := strings.TrimSpace(string(raw))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// Whoami is what /v1/whoami says about the key.
type Whoami struct {
	KeyID    string `json:"key_id"`
	Hostname string `json:"hostname"`
	Team     struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	} `json:"team"`
	Author struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"author"`
}

func (c *Client) Whoami(ctx context.Context) (Whoami, error) {
	var w Whoami
	err := c.Do(ctx, "GET", "/v1/whoami", nil, nil, &w)
	return w, err
}

// Capture.

type batch[T any] struct {
	Rows []T `json:"rows"`
}

type accepted struct {
	Accepted int `json:"accepted"`
}

// PostSessions sends session rows, split into contract-sized batches, and
// returns how many the server accepted.
func (c *Client) PostSessions(ctx context.Context, rows []archive.SessionRow) (int, error) {
	return postRows(ctx, c, "/v1/traces/sessions", rows)
}

// PostBlocks sends block rows.
func (c *Client) PostBlocks(ctx context.Context, rows []archive.BlockRow) (int, error) {
	return postRows(ctx, c, "/v1/traces/blocks", rows)
}

// PostChunks sends chunk rows.
func (c *Client) PostChunks(ctx context.Context, rows []archive.Row) (int, error) {
	return postRows(ctx, c, "/v1/traces/chunks", rows)
}

// Batches splits rows into bodies of at most MaxBatchRows rows and
// MaxBatchBytes bytes each, as {"rows":[...]}. A single row larger than
// MaxBatchBytes is an error: the server would answer 413 however it is sent.
func Batches[T any](rows []T) ([][]byte, error) {
	var out [][]byte
	var cur []json.RawMessage
	size := len(`{"rows":[]}`)
	flush := func() error {
		if len(cur) == 0 {
			return nil
		}
		b, err := json.Marshal(batch[json.RawMessage]{Rows: cur})
		if err != nil {
			return err
		}
		out = append(out, b)
		cur, size = nil, len(`{"rows":[]}`)
		return nil
	}
	for _, r := range rows {
		b, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		if len(b)+len(`{"rows":[]}`) > MaxBatchBytes {
			return nil, &Error{Status: http.StatusRequestEntityTooLarge, Message: fmt.Sprintf("one row is %d bytes, over the %d byte batch limit", len(b), MaxBatchBytes)}
		}
		if len(cur) == MaxBatchRows || size+len(b)+1 > MaxBatchBytes {
			if err := flush(); err != nil {
				return nil, err
			}
		}
		cur = append(cur, b)
		size += len(b) + 1
	}
	return out, flush()
}

func postRows[T any](ctx context.Context, c *Client, path string, rows []T) (int, error) {
	bodies, err := Batches(rows)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, b := range bodies {
		got, err := c.PostBatch(ctx, path, b)
		n += got
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// PostBatch sends one already-encoded batch body to a capture path.
func (c *Client) PostBatch(ctx context.Context, path string, body []byte) (int, error) {
	raw, err := c.DoRaw(ctx, "POST", path, nil, body)
	if err != nil {
		return 0, err
	}
	var a accepted
	if err := json.Unmarshal(raw, &a); err != nil {
		return 0, fmt.Errorf("kit API POST %s: decode: %w", path, err)
	}
	return a.Accepted, nil
}

// DeleteSession removes one of the caller's own sessions.
func (c *Client) DeleteSession(ctx context.Context, id string) error {
	return c.Do(ctx, "DELETE", "/v1/traces/sessions/"+url.PathEscape(id), nil, nil, nil)
}

// Trace reads. Answers have the dashboard's /api shapes; they are returned
// raw so a local dashboard can pass them through untouched, and decoded
// where a CLI needs them.

// Sessions is GET /v1/traces/sessions with the dashboard's filters.
func (c *Client) Sessions(ctx context.Context, q url.Values) (json.RawMessage, error) {
	return c.DoRaw(ctx, "GET", "/v1/traces/sessions", q, nil)
}

// Session is one session with its blocks.
func (c *Client) Session(ctx context.Context, id string, q url.Values) (json.RawMessage, error) {
	return c.DoRaw(ctx, "GET", "/v1/traces/sessions/"+url.PathEscape(id), q, nil)
}

// Search modes.
const (
	ModePhrase = "phrase"
	ModeHybrid = "hybrid"
)

// SearchResult is the part of /api/search's answer a CLI prints.
type SearchResult struct {
	Q        string      `json:"q"`
	Mode     string      `json:"mode"`
	Top      int         `json:"top"`
	Sessions []SearchHit `json:"sessions"`
}

// SearchHit is one session that matched, with its best passage.
type SearchHit struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Project  string  `json:"project"`
	Harness  string  `json:"harness"`
	Host     string  `json:"host"`
	Branch   string  `json:"branch"`
	Snippet  string  `json:"snippet"`
	TurnUUID string  `json:"turn_uuid"`
	Role     string  `json:"role"`
	HitTS    string  `json:"hit_ts"`
	Score    float64 `json:"score"`
	Source   string  `json:"source"`
}

// SearchRaw is GET /v1/traces/search; q holds q, mode and any filters.
func (c *Client) SearchRaw(ctx context.Context, q url.Values) (json.RawMessage, error) {
	return c.DoRaw(ctx, "GET", "/v1/traces/search", q, nil)
}

// Search runs one query in mode (ModePhrase or ModeHybrid).
func (c *Client) Search(ctx context.Context, query, mode string, filters url.Values) (SearchResult, error) {
	q := url.Values{}
	for k, v := range filters {
		q[k] = v
	}
	q.Set("q", query)
	if mode != "" {
		q.Set("mode", mode)
	}
	var out SearchResult
	err := c.Do(ctx, "GET", "/v1/traces/search", q, nil, &out)
	return out, err
}

// Stats is GET /v1/traces/stats.
func (c *Client) Stats(ctx context.Context, q url.Values) (json.RawMessage, error) {
	return c.DoRaw(ctx, "GET", "/v1/traces/stats", q, nil)
}

// Values is GET /v1/traces/values (facet=project|model|tool|skill|harness|host).
func (c *Client) Values(ctx context.Context, q url.Values) (json.RawMessage, error) {
	return c.DoRaw(ctx, "GET", "/v1/traces/values", q, nil)
}

// Board.

// ThreadIn opens a thread. The server sets the poster from the key; Poster
// fields here are hints it may ignore.
type ThreadIn struct {
	board.Poster
	Board string   `json:"board"`
	Title string   `json:"title"`
	Tags  []string `json:"tags,omitempty"`
	Body  string   `json:"body"`
}

// PostIn replies to a thread.
type PostIn struct {
	board.Poster
	Body string `json:"body"`
}

// ThreadView is a thread with its posts.
type ThreadView struct {
	Thread board.Thread `json:"thread"`
	Posts  []board.Post `json:"posts"`
}

func (c *Client) Threads(ctx context.Context, q url.Values) ([]board.Thread, error) {
	var out []board.Thread
	err := c.Do(ctx, "GET", "/v1/board/threads", q, nil, &out)
	return out, err
}

func (c *Client) CreateThread(ctx context.Context, in ThreadIn) (*board.Thread, error) {
	var out board.Thread
	if err := c.Do(ctx, "POST", "/v1/board/threads", nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Thread(ctx context.Context, id int64) (ThreadView, error) {
	var out ThreadView
	err := c.Do(ctx, "GET", fmt.Sprintf("/v1/board/threads/%d", id), nil, nil, &out)
	return out, err
}

func (c *Client) Reply(ctx context.Context, id int64, in PostIn) (*board.Post, error) {
	var out board.Post
	if err := c.Do(ctx, "POST", fmt.Sprintf("/v1/board/threads/%d/posts", id), nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Useful marks a thread useful and returns its count.
func (c *Client) Useful(ctx context.Context, id int64) (int, error) {
	var out struct {
		Useful int `json:"useful"`
	}
	err := c.Do(ctx, "POST", fmt.Sprintf("/v1/board/threads/%d/useful", id), nil, map[string]any{}, &out)
	return out.Useful, err
}

func (c *Client) Pin(ctx context.Context, id int64, on bool) error {
	return c.Do(ctx, "POST", fmt.Sprintf("/v1/board/threads/%d/pin", id), nil, map[string]bool{"on": on}, nil)
}

func (c *Client) Lock(ctx context.Context, id int64, on bool) error {
	return c.Do(ctx, "POST", fmt.Sprintf("/v1/board/threads/%d/lock", id), nil, map[string]bool{"on": on}, nil)
}

// BoardSearch is GET /v1/board/search (q, also, since, top, board, tag, author, role).
func (c *Client) BoardSearch(ctx context.Context, q url.Values) ([]board.Hit, error) {
	var out []board.Hit
	err := c.Do(ctx, "GET", "/v1/board/search", q, nil, &out)
	return out, err
}

// OpenSession asks for a one-time link that signs a browser in to the
// hosted dashboard (POST /v1/session).
func (c *Client) OpenSession(ctx context.Context) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	if err := c.Do(ctx, "POST", "/v1/session", nil, map[string]any{}, &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", errors.New("kit API: /v1/session returned no url")
	}
	return out.URL, nil
}
