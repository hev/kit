package store

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Filesystem mode has no embeddings, so search is grep: the archive's
// lexical approach, kept here until the two share one helper (internal/grep
// on the files-archive branch).
//
//   - A phrasing is its terms: lowercased, split on anything that is not a
//     letter or digit, stopwords dropped, simple plurals folded.
//   - A term matches text holding the folded term or the word as typed, as a
//     case-insensitive substring, so "ports" finds "port" and "policies"
//     still finds "policies".
//   - A post matches a phrasing when it holds every term; --also phrasings
//     widen the search (any phrasing matches).
//   - Matches rank by how many distinct terms they hit across all phrasings,
//     then newest, then id.
//
// rg, when it is on PATH, narrows the post files to read to those holding
// some term; without it every post is read. Either way the matching itself
// is the Go code below, so both give the same answer.

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields("a an and are as at be but by did do does for from has have how i in is it its of on or that the this to was we were what when where which who why will with you") {
		stopwords[w] = true
	}
}

// term is one query word: folded, and as typed when folding changed it.
type term struct{ folded, raw string }

func (t term) in(lower string) bool {
	return strings.Contains(lower, t.folded) || (t.raw != "" && strings.Contains(lower, t.raw))
}

func termsOf(text string) []term {
	var out []term
	seen := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if stopwords[w] || seen[w] {
			continue
		}
		seen[w] = true
		t := term{folded: fold(w)}
		if t.folded != w {
			t.raw = w
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

// grepDoc is one post as search sees it.
type grepDoc struct {
	thread *fileThread
	post   Post
	path   string // the post's file, for the rg prefilter
	text   string // lowercased searchable text
}

// grepRank returns the docs that match some phrasing, best first.
func grepRank(docs []grepDoc, phrasings [][]term) []*grepDoc {
	type scored struct {
		d    *grepDoc
		hits int
	}
	var out []scored
	for i := range docs {
		d := &docs[i]
		matched := false
		hit := map[string]bool{}
		for _, ts := range phrasings {
			all := len(ts) > 0
			for _, t := range ts {
				if t.in(d.text) {
					hit[t.folded] = true
				} else {
					all = false
				}
			}
			matched = matched || all
		}
		if matched {
			out = append(out, scored{d, len(hit)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.hits != b.hits {
			return a.hits > b.hits
		}
		if !a.d.post.Created.Equal(b.d.post.Created) {
			return a.d.post.Created.After(b.d.post.Created)
		}
		return a.d.post.ID < b.d.post.ID
	})
	ds := make([]*grepDoc, len(out))
	for i, s := range out {
		ds[i] = s.d
	}
	return ds
}

// rgPath is rg when it is on PATH; tests and hosts without it get "".
func rgPath() string {
	p, err := exec.LookPath("rg")
	if err != nil {
		return ""
	}
	return p
}

// rgFiles lists the files under roots that hold any of words, case
// insensitively. ok is false when rg could not answer, and the caller reads
// everything instead.
func rgFiles(ctx context.Context, rg string, words []string, roots []string) (files map[string]bool, ok bool) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	args := []string{"--files-with-matches", "--ignore-case", "--fixed-strings", "--no-messages", "--no-ignore", "--hidden", "--glob", "*.json"}
	for _, w := range words {
		args = append(args, "-e", w)
	}
	args = append(args, "--")
	args = append(args, roots...)
	cmd := exec.CommandContext(ctx, rg, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		// rg exits 1 when nothing matched; anything else is not an answer.
		if ee, isExit := err.(*exec.ExitError); !isExit || ee.ExitCode() != 1 {
			return nil, false
		}
	}
	files = map[string]bool{}
	sc := bufio.NewScanner(&stdout)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			files[filepath.Clean(line)] = true
		}
	}
	return files, sc.Err() == nil
}
