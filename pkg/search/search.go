// Package search is kit's query, for any Layer namespace: the request that
// `hev query` sends, with nothing about traces in it. A caller brings its
// own namespace, its own attributes and its own filters; this builds the
// body and reads the answer.
//
// Retrieval runs entirely in the store. Each phrasing becomes an ANN leg that
// the store embeds and a BM25 leg over the same column, and the store fuses
// every leg by reciprocal rank before anything comes back. Nothing is
// embedded, fused or reranked by the caller, so two tools that search two
// namespaces this way rank the same way.
package search

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// MaxPhrasings is how many phrasings ride in one request: two legs each,
// inside turbopuffer's 16-subquery multi-query limit.
const MaxPhrasings = 8

// Query is one question to one namespace.
type Query struct {
	// Phrasings are several ways of asking the same thing. Mix a
	// natural-language phrasing with the literal tokens you expect: the
	// dense legs match meaning, the BM25 legs match exact words.
	Phrasings []string
	// TopK is each leg's budget. The fused set can hold up to one TopK per
	// leg, so a caller that wants N results trims to N.
	TopK int
	// Filter is turbopuffer's filter form and scopes every leg.
	Filter any
	// Attrs are the attributes each row comes back with.
	Attrs []string
	// Column is the embedded, full-text-indexed column. Default "text".
	Column string
}

func (q Query) normalized() (Query, error) {
	var ps []string
	for _, p := range q.Phrasings {
		if p = strings.TrimSpace(p); p != "" {
			ps = append(ps, p)
		}
	}
	if len(ps) == 0 || len(ps) > MaxPhrasings {
		return q, fmt.Errorf("1–%d phrasings per query, got %d", MaxPhrasings, len(ps))
	}
	q.Phrasings = ps
	if q.TopK <= 0 {
		q.TopK = 10
	}
	if q.Column == "" {
		q.Column = "text"
	}
	return q, nil
}

// Body is the native multi-query: an ANN and a BM25 leg per phrasing, fused
// by RRF on the server. POST it to /v2/namespaces/{ns}/query and read the
// answer with Rows.
func (q Query) Body() (map[string]any, error) {
	q, err := q.normalized()
	if err != nil {
		return nil, err
	}
	leg := func(rankBy any) map[string]any {
		l := map[string]any{"rank_by": rankBy, "top_k": q.TopK, "include_attributes": q.Attrs}
		if q.Filter != nil {
			l["filters"] = q.Filter
		}
		return l
	}
	var legs []map[string]any
	for _, p := range q.Phrasings {
		legs = append(legs,
			leg([]any{q.Column, "ANN", []any{"Embed", p}}),
			leg([]any{q.Column, "BM25", p}))
	}
	return map[string]any{"queries": legs, "rerank_by": []any{"RRF"}}, nil
}

// HybridTextBody is one HybridText expression, for a gateway that issues and
// fuses the legs itself. It carries one phrasing; opts are the gateway's
// HybridText options, nil for none.
func (q Query) HybridTextBody(opts map[string]any) (map[string]any, error) {
	q, err := q.normalized()
	if err != nil {
		return nil, err
	}
	if len(q.Phrasings) > 1 {
		return nil, fmt.Errorf("several phrasings need a store that fuses multi-query legs; HybridText carries one")
	}
	rank := []any{q.Column, "HybridText", q.Phrasings[0]}
	if opts != nil {
		rank = append(rank, opts)
	}
	body := map[string]any{"rank_by": rank, "top_k": q.TopK, "include_attributes": q.Attrs}
	if q.Filter != nil {
		body["filters"] = q.Filter
	}
	return body, nil
}

// Rows reads a query answer into rows of T, from either envelope: the fused
// multi-query set (`results[0].rows`) or a single query's `rows`.
func Rows[T any](raw []byte) ([]T, error) {
	var out struct {
		Rows    []T `json:"rows"`
		Results []struct {
			Rows []T `json:"rows"`
		} `json:"results"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("layer query: %s", out.Error)
	}
	if out.Rows != nil {
		return out.Rows, nil
	}
	if len(out.Results) == 0 {
		return nil, nil
	}
	return out.Results[0].Rows, nil
}

// And composes optional predicates; nils drop out, and one clause is itself.
func And(filters ...any) any {
	clauses := []any{}
	for _, f := range filters {
		if f != nil {
			clauses = append(clauses, f)
		}
	}
	switch len(clauses) {
	case 0:
		return nil
	case 1:
		return clauses[0]
	default:
		return []any{"And", clauses}
	}
}

// ParseSince reads a lookback: Go durations ("36h") plus days ("7d") and
// weeks ("2w").
func ParseSince(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	for suffix, unit := range map[string]time.Duration{"d": 24 * time.Hour, "w": 7 * 24 * time.Hour} {
		if v, ok := strings.CutSuffix(s, suffix); ok {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return 0, fmt.Errorf("invalid duration %q", s)
			}
			return time.Duration(n) * unit, nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return d, nil
}

// Since is a lower bound on a timestamp attribute stored as text in layout
// (RFC 3339 for trace chunks). Timestamps that sort as text compare as time.
func Since(attr, since, layout string, now time.Time) (any, error) {
	d, err := ParseSince(since)
	if err != nil {
		return nil, err
	}
	return []any{attr, "Gte", now.Add(-d).UTC().Format(layout)}, nil
}
