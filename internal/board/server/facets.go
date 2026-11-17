package server

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/hev/kit/internal/board/facets"
	"github.com/hev/kit/internal/board/store"
)

// facetQuery reads the repo, host, tool, area and client parameters (each
// repeatable) that GET /api/threads, GET /api/search and the pages take.
func facetQuery(q url.Values) facets.Set {
	var s facets.Set
	for _, n := range facets.Names {
		for _, v := range q[n] {
			for _, part := range strings.Split(v, ",") {
				if part = strings.TrimSpace(part); part != "" {
					s.Add(n, part)
				}
			}
		}
	}
	return s.Normal()
}

// postFacets resolves the facets of a new thread: the poster's explicit
// values (each one checked; any reject fails the post), the facets the
// poster's client inferred (unknown values dropped, never an error), then
// the tag map for whatever facet is still empty.
func (s *Server) postFacets(explicit, inferred *facets.Set, tags []string) (facets.Set, string, error) {
	var mine facets.Set
	if explicit != nil {
		set, rejects := s.tax.Check(*explicit, nil)
		if len(rejects) > 0 {
			return facets.Set{}, "", rejectErr(rejects)
		}
		mine = set
	}
	if inferred != nil {
		mine = mine.Fill(s.tax.Keep(*inferred))
	}
	by := ""
	if !mine.Empty() {
		by = facets.ByAuthor
	}
	fromTags := s.tax.FromTags(tags)
	out := mine.Fill(fromTags)
	if by == "" && !out.Empty() {
		by = facets.ByTagMap
	}
	return out, by, nil
}

// replyFacets checks the facets a reply names; they are added to the thread's.
func (s *Server) replyFacets(explicit *facets.Set) (facets.Set, error) {
	if explicit == nil {
		return facets.Set{}, nil
	}
	set, rejects := s.tax.Check(*explicit, nil)
	if len(rejects) > 0 {
		return facets.Set{}, rejectErr(rejects)
	}
	return set, nil
}

func rejectErr(rejects []facets.Reject) error {
	parts := make([]string, len(rejects))
	for i, r := range rejects {
		parts[i] = r.String()
	}
	return fmt.Errorf("%w: facets: %s (the taxonomy is %s)", store.ErrInvalid, strings.Join(parts, "; "), taxonomyName())
}

func taxonomyName() string {
	return "board.taxonomy in kit's config, or kit's generic example when unset"
}

// chip is one facet value under a thread title, linking to the filtered
// listing.
type chip struct {
	Facet, Value string
	URL          string
}

// chips are a thread's facets as links: base is the listing to filter
// (/b/tooling, /recent, /search?q=…).
func chips(set *facets.Set, base string) []chip {
	if set == nil {
		return nil
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	var out []chip
	for _, n := range facets.Names {
		for _, v := range set.Get(n) {
			out = append(out, chip{Facet: n, Value: v, URL: base + sep + url.Values{n: {v}}.Encode()})
		}
	}
	return out
}

// facetMenu is one menu in a filter bar: the values in use for a facet and
// which one is selected.
type facetMenu struct {
	Name     string
	Values   []string
	Selected string
}

// filterBar is the board and search pages' filter form.
type filterBar struct {
	Action string
	Hidden map[string]string
	Menus  []facetMenu
	Active bool
}

// barFor builds the menus from the facets on threads (the values in use) and
// the current selection. A selected value not in use still shows, so the
// form can be cleared.
func barFor(action string, hidden map[string]string, threads []store.Thread, sel facets.Set) filterBar {
	inUse := map[string]map[string]bool{}
	for _, n := range facets.Names {
		inUse[n] = map[string]bool{}
	}
	for _, t := range threads {
		if t.Facets == nil {
			continue
		}
		for _, n := range facets.Names {
			for _, v := range t.Facets.Get(n) {
				inUse[n][v] = true
			}
		}
	}
	b := filterBar{Action: action, Hidden: hidden}
	for _, n := range facets.Names {
		m := facetMenu{Name: n}
		if vs := sel.Get(n); len(vs) > 0 {
			m.Selected = vs[0]
			inUse[n][vs[0]] = true
			b.Active = true
		}
		for v := range inUse[n] {
			m.Values = append(m.Values, v)
		}
		sort.Strings(m.Values)
		b.Menus = append(b.Menus, m)
	}
	return b
}

// facetFuncs are the template helpers for chips.
func facetFuncs() template.FuncMap {
	return template.FuncMap{
		"chips": chips,
		"boardChips": func(t store.Thread) []chip {
			return chips(t.Facets, "/b/"+url.PathEscape(t.Board))
		},
	}
}

// reviewRow is one thread on the review page and why it is there.
type reviewRow struct {
	T       store.Thread
	Reasons []string
}

// reviewReasons says why a thread needs a look: no facets at all, facets
// copied from a low-confidence inventory row, or (when missing names one)
// that facet empty.
func reviewReasons(t store.Thread, missing string) []string {
	var out []string
	switch {
	case missing != "" && t.Facets != nil && len(t.Facets.Get(missing)) > 0:
		return nil
	case missing != "":
		out = append(out, "no "+missing)
	case t.Facets == nil:
		out = append(out, "no facets")
	}
	if strings.HasSuffix(t.FacetsBy, facets.LowSuffix) {
		out = append(out, "low-confidence inventory row ("+strings.TrimSuffix(t.FacetsBy, facets.LowSuffix)+")")
	}
	return out
}

func (s *Server) pageReview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	missing := r.URL.Query().Get("missing")
	if missing != "" && !validFacet(missing) {
		http.Error(w, "missing: want one of "+strings.Join(facets.Names, ", "), 400)
		return
	}
	board := r.URL.Query().Get("board")
	ts, err := s.st.Threads(ctx, store.ListOpts{Board: board, Limit: 10000})
	if err != nil {
		fail(w, err)
		return
	}
	var rows []reviewRow
	for _, t := range ts {
		if reasons := reviewReasons(t, missing); len(reasons) > 0 {
			rows = append(rows, reviewRow{T: t, Reasons: reasons})
		}
	}
	s.page(w, "review.html", map[string]any{
		"F": s.frame(ctx, "Facet review", ""), "Rows": rows, "Total": len(ts),
		"Missing": missing, "Board": board, "Names": facets.Names,
	})
}

func validFacet(name string) bool {
	for _, n := range facets.Names {
		if n == name {
			return true
		}
	}
	return false
}

// formFacets adds or removes one facet value on a thread from its page. A
// person's edit is recorded as facets_by author, which the backfill never
// overwrites.
func (s *Server) formFacets(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	t, err := s.st.Thread(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	facet, value, err := facets.ParsePair(r.FormValue("facet") + ":" + r.FormValue("value"))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var have facets.Set
	if t.Facets != nil {
		have = *t.Facets
	}
	var one facets.Set
	one.Add(facet, value)
	var next facets.Set
	switch r.FormValue("op") {
	case "add":
		if _, rejects := s.tax.Check(one, nil); len(rejects) > 0 {
			http.Error(w, rejectErr(rejects).Error(), 400)
			return
		}
		next = have.Union(one)
	case "remove":
		next = have.Without(one)
	default:
		http.Error(w, "op: want add or remove", 400)
		return
	}
	if err := s.st.SetFacetsAs(r.Context(), id, next, facets.ByAuthor, formPoster(r).Author); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/t/%d", id), http.StatusSeeOther)
}

// formFacetSet reads the new-thread form's facets field: "repo:kit area:ci".
func formFacetSet(v string) (*facets.Set, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	var s facets.Set
	for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
		facet, value, err := facets.ParsePair(f)
		if err != nil {
			return nil, err
		}
		s.Add(facet, value)
	}
	return &s, nil
}
