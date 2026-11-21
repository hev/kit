package store

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

var stopwords = func() map[string]bool {
	words := map[string]bool{}
	for _, word := range strings.Fields("a an and are as at be but by did do does for from has have how i in is it its of on or that the this to was we were what when where which who why will with you") {
		words[word] = true
	}
	return words
}()

func tokens(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

type term struct{ folded, raw string }

func termsOf(text string) []term {
	var out []term
	seen := map[string]bool{}
	for _, word := range tokens(text) {
		key := fold(word)
		if stopwords[word] || seen[key] {
			continue
		}
		seen[key] = true
		t := term{folded: key}
		if key != word {
			t.raw = word
		}
		out = append(out, t)
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

func documentWords(body, heading string) map[string]int {
	words := map[string]int{}
	for _, word := range tokens(body) {
		if !stopwords[word] {
			words[fold(word)]++
		}
	}
	for _, word := range tokens(heading) {
		if !stopwords[word] {
			words[fold(word)] += 2
		}
	}
	return words
}

type bm25Doc struct {
	thread *fileThread
	post   Post
	words  map[string]int
	score  float64
}

func bm25Rank(docs []bm25Doc, phrasings [][]term) []*bm25Doc {
	lengths := make([]float64, len(docs))
	total := 0.0
	df := map[string]int{}
	for i, doc := range docs {
		for word, count := range doc.words {
			df[word]++
			lengths[i] += float64(count)
		}
		total += lengths[i]
	}
	if total == 0 {
		return nil
	}
	avg := total / float64(len(docs))
	newer := func(a, b *bm25Doc) bool {
		if !a.post.Created.Equal(b.post.Created) {
			return a.post.Created.After(b.post.Created)
		}
		return a.post.ID < b.post.ID
	}
	type ranked struct {
		doc   *bm25Doc
		score float64
	}
	for _, phrase := range phrasings {
		if len(phrase) == 0 {
			continue
		}
		var matches []ranked
		for i := range docs {
			doc := &docs[i]
			score := 0.0
			for _, word := range phrase {
				tf := float64(doc.words[word.folded])
				if tf == 0 {
					score = 0
					break
				}
				freq := float64(df[word.folded])
				idf := math.Log1p((float64(len(docs)) - freq + 0.5) / (freq + 0.5))
				const k1, b = 1.2, 0.75
				score += idf * tf * (k1 + 1) / (tf + k1*(1-b+b*lengths[i]/avg))
			}
			if score > 0 {
				matches = append(matches, ranked{doc, score})
			}
		}
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].score != matches[j].score {
				return matches[i].score > matches[j].score
			}
			return newer(matches[i].doc, matches[j].doc)
		})
		for i, hit := range matches {
			if len(phrasings) == 1 {
				hit.doc.score = hit.score
			} else {
				hit.doc.score += 1 / float64(61+i)
			}
		}
	}
	var matches []*bm25Doc
	for i := range docs {
		if docs[i].score > 0 {
			matches = append(matches, &docs[i])
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return newer(matches[i], matches[j])
	})
	return matches
}
