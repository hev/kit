package collect

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/pkg/search"
)

// MaxLimit bounds limit on both search routes.
const MaxLimit = 20

const (
	maxTraceBody = 32 << 20
	maxAPIBody   = 1 << 20
	snippetRunes = 280
)

// Config is everything a collector serves with.
type Config struct {
	// Key is HEV_COLLECT_SERVICE_KEY. Every route but /healthz requires it.
	Key    string
	Index  TurnIndex
	Boards Boards
	Sink   EnvelopeSink
	// Scrubber is kit's redaction; nil means capture.redact is off.
	Scrubber *redact.Scrubber
	Host     string
	Logger   *slog.Logger
}

// Server is `hev collect`'s HTTP surface.
type Server struct {
	cfg     Config
	keySum  [32]byte
	agg     *Aggregator
	handler http.Handler
}

// ErrNoKey is returned when no service key is configured.
var ErrNoKey = errors.New("HEV_COLLECT_SERVICE_KEY is not set: hev collect refuses to serve without a service key")

func New(cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.Key) == "" {
		return nil, ErrNoKey
	}
	if cfg.Index == nil || cfg.Boards == nil || cfg.Sink == nil {
		return nil, errors.New("collect: index, boards and envelope sink are required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	s := &Server{cfg: cfg, keySum: sha256.Sum256([]byte(cfg.Key)), agg: NewAggregator()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.Handle("POST /v1/traces", s.auth(http.HandlerFunc(s.traces)))
	mux.Handle("POST /v1/app/traces/search", s.auth(http.HandlerFunc(s.searchTraces)))
	mux.Handle("GET /v1/app/traces/conversations/{chatId}", s.auth(http.HandlerFunc(s.conversation)))
	mux.Handle("POST /v1/app/boards/{board}/posts", s.auth(http.HandlerFunc(s.boardPost)))
	mux.Handle("POST /v1/app/boards/{board}/search", s.auth(http.HandlerFunc(s.boardSearch)))
	s.handler = s.logRequests(mux)
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.handler }

// Run expires idle turns and writes the envelopes of spans that never found
// their turn, until ctx ends; then it writes whatever is still waiting.
func (s *Server) Run(ctx context.Context) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			s.writeOrphans(s.agg.Drain())
			return
		case <-tick.C:
			s.writeOrphans(s.agg.Expire())
		}
	}
}

func (s *Server) writeOrphans(spans []Span) {
	if len(spans) == 0 {
		return
	}
	envs := make([]*daemon.TraceEnvelope, 0, len(spans))
	for _, sp := range spans {
		envs = append(envs, Envelope(Attributed{Span: sp}, s.cfg.Host, s.policy()))
	}
	if err := s.cfg.Sink.Write(envs); err != nil {
		s.cfg.Logger.Error("write envelopes", "err", err)
	}
}

func (s *Server) policy() string {
	if s.cfg.Scrubber == nil {
		return ""
	}
	return RedactionPolicy
}

// auth compares the bearer token in constant time: both sides are hashed
// first, so neither the comparison nor its length leaks the key.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
		if !ok || subtle.ConstantTimeCompare(sum[:], s.keySum[:]) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// logRequests logs method, route path, status and duration. It never logs
// headers, query strings or bodies: the key rides in a header, and owners and
// prompts are the app's users' data.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.cfg.Logger.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func badRequest(w http.ResponseWriter, format string, args ...any) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf(format, args...)})
}

// serverError answers 500 with a generic message and logs the cause, which
// can carry a store's error text.
func (s *Server) serverError(w http.ResponseWriter, what string, err error) {
	s.cfg.Logger.Error(what, "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": what + " failed"})
}

// ---- capture -----------------------------------------------------------------

func (s *Server) traces(w http.ResponseWriter, r *http.Request) {
	var body io.Reader = http.MaxBytesReader(w, r.Body, maxTraceBody)
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(body)
		if err != nil {
			badRequest(w, "gzip: %v", err)
			return
		}
		defer gz.Close()
		body = io.LimitReader(gz, maxTraceBody)
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		badRequest(w, "read body: %v", err)
		return
	}
	proto := strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-protobuf")
	var spans []Span
	if proto {
		spans, err = DecodeProto(raw)
	} else {
		spans, err = DecodeJSON(raw)
	}
	if err != nil {
		badRequest(w, "%v", err)
		return
	}
	if err := s.Ingest(r.Context(), spans); err != nil {
		// 503 is retryable for an OTLP exporter; the batch merges again
		// idempotently into the same turn rows.
		s.cfg.Logger.Error("ingest", "err", err, "spans", len(spans))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ingest failed; retry"})
		return
	}
	if proto {
		// An empty ExportTraceServiceResponse is zero bytes.
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

// Ingest scrubs each span, writes the envelopes of every span that now knows
// its turn, and upserts the turns those spans changed. Redaction happens
// first, so nothing unscrubbed reaches an envelope, a turn, or the index.
func (s *Server) Ingest(ctx context.Context, spans []Span) error {
	for i := range spans {
		Redact(s.cfg.Scrubber, &spans[i])
	}
	attributed, turns := s.agg.Add(spans)
	envs := make([]*daemon.TraceEnvelope, 0, len(attributed))
	for _, a := range attributed {
		envs = append(envs, Envelope(a, s.cfg.Host, s.policy()))
	}
	if err := s.cfg.Sink.Write(envs); err != nil {
		return fmt.Errorf("write envelopes: %w", err)
	}
	for i := range turns {
		// Prompt and answer were scrubbed with their spans; scrub the
		// extracted text again so a secret split across JSON escaping in a
		// recorded message cannot reach the index.
		turns[i].Prompt = s.scrub(turns[i].Prompt)
		turns[i].Answer = s.scrub(turns[i].Answer)
	}
	if err := s.cfg.Index.Upsert(ctx, turns); err != nil {
		return fmt.Errorf("index turns: %w", err)
	}
	return nil
}

func (s *Server) scrub(text string) string {
	out, _ := s.cfg.Scrubber.Text(text)
	return out
}

// ---- traces API ----------------------------------------------------------------

type traceSearchRequest struct {
	Owner  string `json:"owner"`
	Tenant string `json:"tenant"`
	Query  string `json:"query"`
	Since  string `json:"since"`
	Limit  int    `json:"limit"`
}

type traceHit struct {
	ChatID        string  `json:"chatId"`
	TurnID        string  `json:"turnId"`
	TS            string  `json:"ts"`
	Prompt        string  `json:"prompt"`
	AnswerSnippet string  `json:"answerSnippet"`
	Score         float64 `json:"score"`
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAPIBody))
	if err := dec.Decode(v); err != nil {
		badRequest(w, "body must be a JSON object: %v", err)
		return false
	}
	return true
}

func clampLimit(n int) int {
	if n <= 0 {
		return 10
	}
	return min(n, MaxLimit)
}

// parseSince takes an RFC 3339 time or a lookback (36h, 7d, 2w).
func parseSince(s string, now time.Time) (time.Time, error) {
	if s = strings.TrimSpace(s); s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	d, err := search.ParseSince(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("since must be an RFC 3339 time or a lookback like 36h, 7d, 2w")
	}
	return now.Add(-d), nil
}

func (s *Server) searchTraces(w http.ResponseWriter, r *http.Request) {
	var req traceSearchRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Owner == "" || req.Tenant == "" {
		badRequest(w, "owner and tenant are required")
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		badRequest(w, "query is required")
		return
	}
	since, err := parseSince(req.Since, time.Now())
	if err != nil {
		badRequest(w, "%v", err)
		return
	}
	limit := clampLimit(req.Limit)
	hits, err := s.cfg.Index.Search(r.Context(), req.Owner, req.Tenant, req.Query, since, limit)
	if err != nil {
		s.serverError(w, "search", err)
		return
	}
	out := []traceHit{}
	for _, h := range hits {
		// The index filters by owner and tenant; this is the second lock.
		if h.Turn.Owner != req.Owner || h.Turn.Tenant != req.Tenant {
			continue
		}
		if !since.IsZero() && h.Turn.TS.Before(since) {
			continue
		}
		out = append(out, traceHit{ChatID: h.Turn.ChatID, TurnID: h.Turn.TurnID, TS: h.Turn.TS.UTC().Format(time.RFC3339Nano), Prompt: h.Turn.Prompt, AnswerSnippet: snip(h.Turn.Answer, snippetRunes), Score: h.Score})
		if len(out) == limit {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": out})
}

func snip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

type convTurn struct {
	TurnID string `json:"turnId"`
	TS     string `json:"ts"`
	Prompt string `json:"prompt"`
	Answer string `json:"answer"`
	Tools  []Tool `json:"tools"`
}

func (s *Server) conversation(w http.ResponseWriter, r *http.Request) {
	chatID := r.PathValue("chatId")
	owner, tenant := r.URL.Query().Get("owner"), r.URL.Query().Get("tenant")
	if owner == "" || tenant == "" {
		badRequest(w, "owner and tenant are required")
		return
	}
	turns, err := s.cfg.Index.Conversation(r.Context(), owner, tenant, chatID)
	if err != nil {
		s.serverError(w, "conversation", err)
		return
	}
	sort.SliceStable(turns, func(i, j int) bool { return turns[i].TS.Before(turns[j].TS) })
	out := []convTurn{}
	for _, t := range turns {
		if t.Owner != owner || t.Tenant != tenant || t.ChatID != chatID {
			continue
		}
		tools := t.Tools
		if tools == nil {
			tools = []Tool{}
		}
		out = append(out, convTurn{TurnID: t.TurnID, TS: t.TS.UTC().Format(time.RFC3339Nano), Prompt: t.Prompt, Answer: t.Answer, Tools: tools})
	}
	if len(out) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such conversation"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chatId": chatID, "turns": out})
}

// ---- boards API ----------------------------------------------------------------

func (s *Server) boardPost(w http.ResponseWriter, r *http.Request) {
	var req BoardPost
	if !decodeBody(w, r, &req) {
		return
	}
	req.Author, req.Title, req.Body = strings.TrimSpace(req.Author), strings.TrimSpace(req.Title), strings.TrimSpace(req.Body)
	if req.Author == "" || req.Title == "" || req.Body == "" {
		badRequest(w, "author, title and body are required")
		return
	}
	// Memories are written by an agent from its user's conversation; they
	// pass the same redaction as traces before they reach the store.
	req.Title, req.Body = s.scrub(req.Title), s.scrub(req.Body)
	res, err := s.cfg.Boards.Post(r.Context(), r.PathValue("board"), req)
	if errors.Is(err, ErrBadBoardRequest) {
		badRequest(w, "%v", err)
		return
	}
	if err != nil {
		s.serverError(w, "board post", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) boardSearch(w http.ResponseWriter, r *http.Request) {
	var req BoardQuery
	if !decodeBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Query) == "" && len(req.Tags) == 0 {
		badRequest(w, "query or tags are required")
		return
	}
	req.Limit = clampLimit(req.Limit)
	hits, err := s.cfg.Boards.Search(r.Context(), r.PathValue("board"), req)
	if errors.Is(err, ErrBadBoardRequest) {
		badRequest(w, "%v", err)
		return
	}
	if err != nil {
		s.serverError(w, "board search", err)
		return
	}
	if hits == nil {
		hits = []BoardHit{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
}
