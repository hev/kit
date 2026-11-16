package store

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// Filesystem mode has no embeddings, so search is lexical: Okapi BM25 over
// each post, with a thread's title and tags weighted twice its body. Each
// phrasing ranks the posts on its own and the rankings are fused by
// reciprocal rank, the way extra phrasings widen a Layer search.

type lexField struct {
	text   string
	weight float64
}

type lexDoc struct {
	thread *fileThread
	post   Post
	keep   bool // passes the search filters
	fields []lexField

	tf  map[string]float64
	len float64
}

const (
	bm25K1 = 1.2
	bm25B  = 0.75
	rrfK   = 60
)

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields("a an and are as at be but by for from has have how i in is it its of on or that the this to was we were what when where which who why will with you") {
		stopwords[w] = true
	}
}

// terms lowercases, splits on anything that is not a letter or digit, drops
// stopwords, and folds simple plurals so "ports" finds "port".
func terms(text string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if stopwords[w] {
			continue
		}
		out = append(out, fold(w))
	}
	return out
}

func fold(w string) string {
	switch {
	case len(w) > 4 && strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us"):
		return w[:len(w)-1]
	}
	return w
}

// rankLexical returns the documents that pass the filters and match at least
// one phrasing, best first.
func rankLexical(corpus []lexDoc, phrasings []string) []*lexDoc {
	df := map[string]float64{}
	total := 0.0
	for i := range corpus {
		d := &corpus[i]
		d.tf = map[string]float64{}
		for _, f := range d.fields {
			for _, t := range terms(f.text) {
				d.tf[t] += f.weight
				d.len += f.weight
			}
		}
		for t := range d.tf {
			df[t]++
		}
		total += d.len
	}
	if len(corpus) == 0 {
		return nil
	}
	n := float64(len(corpus))
	avg := math.Max(total/n, 1)
	fused := map[*lexDoc]float64{}
	for _, phrasing := range phrasings {
		q := map[string]bool{}
		for _, t := range terms(phrasing) {
			q[t] = true
		}
		type scored struct {
			d *lexDoc
			s float64
		}
		var ranked []scored
		for i := range corpus {
			d := &corpus[i]
			if !d.keep {
				continue
			}
			s := 0.0
			for t := range q {
				tf := d.tf[t]
				if tf == 0 {
					continue
				}
				idf := math.Log(1 + (n-df[t]+0.5)/(df[t]+0.5))
				s += idf * tf * (bm25K1 + 1) / (tf + bm25K1*(1-bm25B+bm25B*d.len/avg))
			}
			if s > 0 {
				ranked = append(ranked, scored{d, s})
			}
		}
		sort.SliceStable(ranked, func(i, j int) bool {
			if ranked[i].s != ranked[j].s {
				return ranked[i].s > ranked[j].s
			}
			return ranked[i].d.post.Created.After(ranked[j].d.post.Created)
		})
		for rank, r := range ranked {
			fused[r.d] += 1.0 / float64(rrfK+rank+1)
		}
	}
	out := make([]*lexDoc, 0, len(fused))
	for d := range fused {
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if fused[out[i]] != fused[out[j]] {
			return fused[out[i]] > fused[out[j]]
		}
		if !out[i].post.Created.Equal(out[j].post.Created) {
			return out[i].post.Created.After(out[j].post.Created)
		}
		return out[i].post.ID < out[j].post.ID
	})
	return out
}
