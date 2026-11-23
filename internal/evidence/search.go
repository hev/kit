package evidence

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/hev/kit/internal/grep"
)

// MaxAlso is how many extra phrasings a search takes.
const MaxAlso = 7

// SearchQuery is a lexical search over manifests and text items.
type SearchQuery struct {
	Q      string
	Also   []string
	Filter Filter
	// Limit caps the hits (default 50, at most 200).
	Limit int
}

// Match is EvidenceMatch: which part of a package matched.
type Match struct {
	Target   string    `json:"target"`
	ItemID   string    `json:"item_id,omitempty"`
	Seq      int       `json:"seq,omitempty"`
	Modality string    `json:"modality,omitempty"`
	Name     string    `json:"name,omitempty"`
	Caption  string    `json:"caption,omitempty"`
	Snippet  string    `json:"snippet,omitempty"`
	Segment  *struct{} `json:"segment"`
	Score    float64   `json:"score"`
}

// Hit is EvidenceHit: one package and its best matches.
type Hit struct {
	Package Summary `json:"package"`
	Matches []Match `json:"matches"`
	Score   float64 `json:"score"`
}

// SearchResponse is EvidenceSearchResponse as an OSS file store answers it.
type SearchResponse struct {
	NextCursor *string  `json:"next_cursor"`
	Q          string   `json:"q"`
	Also       []string `json:"also,omitempty"`
	Route      string   `json:"route"`
	Engine     string   `json:"engine"`
	Semantic   bool     `json:"semantic"`
	Data       []Hit    `json:"data"`
}

// maxMatches is how many matches a hit carries.
const maxMatches = 5

// Search finds packages holding every word of a phrasing (OR across Also)
// in their manifest fields or text items, grouped one hit per package and
// ranked by distinct words matched, then newest. ripgrep, when on PATH,
// narrows which packages to read; the Go fallback reads them all. The
// decision is made here in Go on the stored text either way.
func (s *Store) Search(ctx context.Context, sq SearchQuery) (*SearchResponse, error) {
	if len(sq.Also) > MaxAlso {
		return nil, invalid("%d --also phrasings, over the %d limit", len(sq.Also), MaxAlso)
	}
	if err := sq.Filter.Validate(); err != nil {
		return nil, err
	}
	q, err := grep.Compile(append([]string{sq.Q}, sq.Also...))
	if err != nil {
		return nil, invalid("%v", err)
	}
	limit := sq.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	all, err := s.all()
	if err != nil {
		return nil, err
	}
	var pkgs []*Package
	files := map[string]string{} // searched file -> package id
	var paths []string
	for _, p := range all {
		if !sq.Filter.Match(p) {
			continue
		}
		pkgs = append(pkgs, p)
		mp := filepath.Join(s.dir(p.ID), ManifestName)
		files[mp] = p.ID
		paths = append(paths, mp)
		for _, it := range p.Items {
			if it.Modality == "text" {
				tp := filepath.Join(s.dir(p.ID), it.Name)
				files[tp] = p.ID
				paths = append(paths, tp)
			}
		}
	}
	candidates, err := s.candidates(ctx, q, paths, files)
	if err != nil {
		return nil, err
	}
	resp := &SearchResponse{Q: sq.Q, Also: sq.Also, Route: "phrase.scan", Engine: grep.Engine(q), Data: []Hit{}}
	for _, p := range pkgs {
		if candidates != nil && !candidates[p.ID] {
			continue
		}
		if h, ok := s.score(q, p); ok {
			resp.Data = append(resp.Data, h)
		}
	}
	created := map[string]string{}
	for _, p := range pkgs {
		created[p.ID] = p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	grep.Sort(resp.Data, func(h Hit) grep.Ranked {
		return grep.Ranked{Score: int(h.Score), TS: created[h.Package.ID], ID: h.Package.ID}
	})
	if len(resp.Data) > limit {
		resp.Data = resp.Data[:limit]
	}
	return resp, nil
}

// candidates is the set of packages with a searched file holding one of the
// query's needles, or nil when the query has no needles (every package is a
// candidate). With rg this reads only matching lines; the Go walk reads
// every line of the searched files.
func (s *Store) candidates(ctx context.Context, q *grep.Query, paths []string, files map[string]string) (map[string]bool, error) {
	needles := q.Needles()
	if len(needles) == 0 {
		return nil, nil
	}
	nq, err := grep.Compile(needles)
	if err != nil {
		return nil, nil
	}
	out := map[string]bool{}
	var mu sync.Mutex
	const batch = 256
	for i := 0; i < len(paths); i += batch {
		chunk := paths[i:min(i+batch, len(paths))]
		err := grep.Lines(ctx, nq, chunk, "*", func(path string, _ []byte) bool {
			id, ok := files[path]
			if !ok {
				id, ok = files[filepath.Clean(path)]
			}
			if ok {
				mu.Lock()
				out[id] = true
				mu.Unlock()
			}
			return true
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

type doc struct {
	m    Match
	text string
}

func (s *Store) score(q *grep.Query, p *Package) (Hit, bool) {
	pkgText := strings.Join(append([]string{p.Title, p.Summary, p.Source, p.Order, p.Session, p.Gate, p.Repo, p.Branch, p.Author}, p.Tags...), "\n")
	docs := []doc{{m: Match{Target: "package"}, text: pkgText}}
	whole := []string{pkgText}
	for _, c := range p.Claims {
		t := strings.TrimSpace(c.Criterion + "\n" + c.Text)
		docs = append(docs, doc{m: Match{Target: "claim"}, text: t})
		whole = append(whole, t)
	}
	for _, it := range p.Items {
		parts := []string{it.Name, it.Caption, it.URL, it.App, it.Step}
		keys := make([]string, 0, len(it.Metadata))
		for k := range it.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, it.Metadata[k])
		}
		if it.Modality == "text" {
			if it.Text != nil {
				parts = append(parts, *it.Text)
			} else if b, err := readText(filepath.Join(s.dir(p.ID), it.Name), s.Limits.TextBytes); err == nil {
				parts = append(parts, string(b))
			}
		}
		t := strings.Join(parts, "\n")
		docs = append(docs, doc{m: Match{Target: "item", ItemID: it.ID, Seq: it.Seq, Modality: it.Modality, Name: it.Name, Caption: it.Caption}, text: t})
		whole = append(whole, t)
	}
	var matches []Match
	best := 0
	for _, d := range docs {
		n, ok := q.Score(d.text)
		if !ok {
			continue
		}
		d.m.Score = float64(n)
		d.m.Snippet = q.Snippet(d.text, 160)
		matches = append(matches, d.m)
		best = max(best, n)
	}
	if len(matches) == 0 {
		// The words may be spread over the package's parts.
		all := strings.Join(whole, "\n")
		n, ok := q.Score(all)
		if !ok {
			return Hit{}, false
		}
		matches = []Match{{Target: "package", Snippet: q.Snippet(all, 160), Score: float64(n)}}
		best = n
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	if len(matches) > maxMatches {
		matches = matches[:maxMatches]
	}
	return Hit{Package: p.Summarize(), Matches: matches, Score: float64(best)}, true
}

func readText(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadCapped(f, max)
}
