package dashboard

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Principal must be established by the authenticated wrapper, never by request
// parameters. Reviewer is a stable verified human identity, not a display name.
type Principal struct {
	Tenant   string `json:"tenant"`
	Reviewer string `json:"reviewer"`
}
type ReviewIdentity func(*http.Request, Credentials) (Principal, error)

// QueueRequest is bounded to 100 items. Cursor is opaque and must be bound to
// tenant and filters by the provider. Status is unreviewed or reviewed; empty
// means all. Uncertainty is uncertain or certain; empty means all.
type QueueRequest struct {
	Kind, Status, Uncertainty, Cursor string
	Limit                             int
}
type Label struct {
	EvalID      string     `json:"eval_id"`
	ContextRef  string     `json:"context_ref"`
	SampleClass string     `json:"sample_class"` // genuine, controlled_probe, or synthetic; never pool precision denominators
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Session     string     `json:"session"`
	Turn        string     `json:"turn"`
	Source      string     `json:"source"`
	Policy      string     `json:"policy"`
	Prediction  string     `json:"prediction"`
	Score       *float64   `json:"score,omitempty"`
	Uncertain   bool       `json:"uncertain"`
	Synthetic   bool       `json:"synthetic"`
	Judgments   []Judgment `json:"judgments,omitempty"`
}

// Judgment distinguishes prior agent evidence from verified human review.
type Judgment struct {
	ActorType string `json:"actor_type"`
	Actor     string `json:"actor"`
	Verdict   string `json:"verdict"`
	Note      string `json:"note,omitempty"`
}
type KindTotals struct {
	SampleClass string `json:"sample_class"`
	State       string `json:"state"` // complete, lower_bound, or unavailable
	Kind        string `json:"kind"`
	Reviewed    int    `json:"reviewed"`
	Remaining   int    `json:"remaining"`
	Uncertain   int    `json:"uncertain"`
	Required    int    `json:"required"`
	Shortage    int    `json:"shortage"`
}
type QueuePage struct {
	Items     []Label      `json:"items"`
	Next      string       `json:"next"`
	Totals    []KindTotals `json:"totals"`
	Truncated bool         `json:"truncated"`
	// Coverage explains the scope of totals, including unavailable sources.
	Coverage string `json:"coverage"`
}
type ContextBlock struct {
	Role   string `json:"role"`
	Text   string `json:"text"`
	Source string `json:"source"`
}
type LabelContext struct {
	State       string         `json:"state"` // complete, missing, ambiguous, or truncated
	Explanation string         `json:"explanation"`
	Blocks      []ContextBlock `json:"blocks"` // chronological, including preceding actions/errors
}

// LabelSource must authorize every item lookup against tenant. Queue retains
// individual eval/source/policy identities, including multiple labels/session.
// Build an index once or use bounded storage queries; Context must not rescan
// the archive per card. Totals cover configured kinds, independent of paging.
type LabelSource interface {
	Queue(context.Context, string, QueueRequest) (QueuePage, error)
	Context(context.Context, string, string) (LabelContext, error)
}
type Review struct {
	Revision  int       `json:"revision"`
	Tenant    string    `json:"tenant"`
	Item      string    `json:"item"`
	Reviewer  string    `json:"reviewer"`
	ActorType string    `json:"actor_type"`
	Verdict   string    `json:"verdict"`
	Note      string    `json:"note,omitempty"`
	At        time.Time `json:"at"`
}
type ReviewChange struct {
	Verdict          string `json:"verdict"`
	Note             string `json:"note"`
	ExpectedRevision int    `json:"expected_revision"`
}

var ErrReviewConflict = errors.New("review revision conflict")
var ErrReviewNotFound = errors.New("review item not found")

// ReviewStore stores independent, append-only revisions without changing evals.
// Save must atomically compare ExpectedRevision to this tenant/item's latest
// revision. History is oldest first. Providers must not trust browser identity.
type ReviewStore interface {
	History(context.Context, string, string) ([]Review, error)
	Save(context.Context, Principal, string, ReviewChange) (Review, error)
}
type ReviewConfig struct {
	Source   LabelSource
	Store    ReviewStore
	Identity ReviewIdentity
	Kinds    []string
}

//go:embed review.html
var reviewHTML string

func (c *ReviewConfig) validate() error {
	if c.Source == nil || c.Store == nil || c.Identity == nil || len(c.Kinds) == 0 {
		return errors.New("review requires source, store, verified identity and kinds")
	}
	seen := map[string]bool{}
	for _, k := range c.Kinds {
		if strings.TrimSpace(k) == "" || seen[k] {
			return errors.New("invalid review kinds")
		}
		seen[k] = true
	}
	return nil
}
func reviewJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func reviewError(w http.ResponseWriter, err error) {
	code := http.StatusBadGateway
	if errors.Is(err, ErrReviewConflict) {
		code = http.StatusConflict
	}
	if errors.Is(err, ErrReviewNotFound) {
		code = http.StatusNotFound
	}
	http.Error(w, http.StatusText(code), code) // never reflect private storage errors
}
func (c *ReviewConfig) handle(w http.ResponseWriter, r *http.Request, credentials Credentials) {
	p, err := c.Identity(r, credentials)
	if err != nil || p.Tenant == "" || p.Reviewer == "" || p.Tenant != credentials.Namespace {
		http.Error(w, "review authentication required", 401)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path == "/review" && r.Method == "GET" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Write([]byte(reviewHTML))
		return
	}
	if r.URL.Path == "/ui/review.css" && r.Method == "GET" {
		w.Header().Set("Content-Type", "text/css")
		w.Write([]byte(reviewCSS))
		return
	}
	if r.URL.Path == "/ui/review.js" && r.Method == "GET" {
		w.Header().Set("Content-Type", "text/javascript")
		w.Write([]byte(reviewJS))
		return
	}
	if r.URL.Path == "/api/review/queue" && r.Method == "GET" {
		q := r.URL.Query()
		limit := 25
		if q.Has("limit") {
			limit, err = strconv.Atoi(q.Get("limit"))
			if err != nil || limit < 1 || limit > 100 {
				http.Error(w, "limit must be 1..100", 400)
				return
			}
		}
		req := QueueRequest{q.Get("kind"), q.Get("status"), q.Get("uncertainty"), q.Get("cursor"), limit}
		validKind := req.Kind == ""
		for _, k := range c.Kinds {
			validKind = validKind || k == req.Kind
		}
		if !validKind || (req.Status != "" && req.Status != "reviewed" && req.Status != "unreviewed") || (req.Uncertainty != "" && req.Uncertainty != "uncertain" && req.Uncertainty != "certain") || len(req.Cursor) > 4096 {
			http.Error(w, "invalid queue filter", 400)
			return
		}
		page, err := c.Source.Queue(r.Context(), p.Tenant, req)
		if err != nil {
			reviewError(w, err)
			return
		}
		if len(page.Items) > limit {
			http.Error(w, "queue exceeded page bound", 502)
			return
		}
		reviewJSON(w, map[string]any{"page": page, "kinds": c.Kinds})
		return
	}
	prefix := "/api/review/items/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	tail := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(tail, "/")
	if len(parts) != 2 || parts[0] == "" || len(parts[0]) > 512 {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	// Context lookup authorizes the item before all history and write operations.
	ctx, err := c.Source.Context(r.Context(), p.Tenant, id)
	if err != nil {
		reviewError(w, err)
		return
	}
	if parts[1] == "context" && r.Method == "GET" {
		reviewJSON(w, ctx)
		return
	}
	if parts[1] != "reviews" {
		http.NotFound(w, r)
		return
	}
	if r.Method == "GET" {
		history, err := c.Store.History(r.Context(), p.Tenant, id)
		if err != nil {
			reviewError(w, err)
			return
		}
		reviewJSON(w, history)
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	// Require same-origin JSON with a custom header. Missing Origin fails closed;
	// wrappers behind proxies must preserve the browser-facing Host and TLS.
	origin, err := url.Parse(r.Header.Get("Origin"))
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if err != nil || origin.User != nil || origin.Scheme != scheme || origin.Host != r.Host || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || r.Header.Get("X-Kit-Review") != "1" || strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		http.Error(w, "same-origin review required", 403)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var change ReviewChange
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&change) != nil || change.ExpectedRevision < 0 || len(change.Note) > 8000 || (change.Verdict != "correct" && change.Verdict != "incorrect" && change.Verdict != "uncertain") {
		http.Error(w, "invalid review", 400)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		http.Error(w, "invalid review", 400)
		return
	}
	review, err := c.Store.Save(r.Context(), p, id, change)
	if err != nil {
		reviewError(w, err)
		return
	}
	reviewJSON(w, review)
}

//go:embed review.js
var reviewJS string

//go:embed review.css
var reviewCSS string
