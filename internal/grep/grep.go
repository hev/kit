// Package grep is word search over files: no index, no embeddings. A query
// is split into terms; a document matches a phrasing when it contains every
// one of the phrasing's terms (or, in regex mode, matches the expression);
// extra phrasings (--also) are OR. Matches rank by how many distinct terms
// they hit, then newest first.
//
// When ripgrep (`rg`) is on PATH it narrows the files and lines to read
// (`rg --json`, fixed strings, case-insensitive); otherwise a Go walk reads
// every line. Either way the final decision is made in Go on the decoded
// text, so the two paths return the same hits.
package grep

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
)

// stopwords are dropped from queries: they would make an AND of terms match
// almost nothing and add nothing to ranking.
var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields("a an and are as at be but by can could did do does for from had has have how i if in into is it its me my of on or our should so that the their then there these this those to was we were what when where which who why will with would you your") {
		stopwords[w] = true
	}
}

// Terms splits a phrasing into lowercase search terms: split on anything
// that is not a letter, digit, '_', '-', '.' or '/', trim punctuation at the
// ends, drop stopwords, and fold simple plurals so "ports" finds "port". A
// phrasing made only of stopwords keeps its words.
func Terms(q string) []string {
	split := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '.' && r != '/'
	})
	var all, kept []string
	seen := map[string]bool{}
	for _, w := range split {
		w = strings.Trim(w, "-./")
		if w == "" || seen[w] {
			continue
		}
		seen[w] = true
		all = append(all, w)
		if !stopwords[w] {
			kept = append(kept, Fold(w))
		}
	}
	if len(kept) == 0 {
		return all
	}
	return dedupe(kept)
}

// Fold strips a simple plural so a substring search for the result finds
// both forms.
func Fold(w string) string {
	switch {
	case len(w) > 4 && strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us"):
		return w[:len(w)-1]
	}
	return w
}

// Variants are the substrings that count as a folded term: the term, and
// for one ending in y the "ie" stem, so "retry" finds "retries" and
// "retried" as well as "retry".
func Variants(t string) []string {
	if len(t) > 3 && strings.HasSuffix(t, "y") {
		return []string{t, t[:len(t)-1] + "ie"}
	}
	return []string{t}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// Query is a compiled search: phrasings of AND-ed terms, OR-ed together, or
// one regular expression.
type Query struct {
	phrasings [][]string
	terms     []string // distinct terms over all phrasings
	re        *regexp.Regexp
}

// Compile builds a term query from one or more phrasings.
func Compile(phrasings []string) (*Query, error) {
	q := &Query{}
	for _, p := range phrasings {
		t := Terms(p)
		if len(t) == 0 {
			continue
		}
		q.phrasings = append(q.phrasings, t)
		q.terms = append(q.terms, t...)
	}
	if len(q.phrasings) == 0 {
		return nil, fmt.Errorf("the query has no words to search for")
	}
	q.terms = dedupe(q.terms)
	return q, nil
}

// CompileRegex builds a regex query (Go RE2 syntax). Prefix the pattern with
// (?i) to ignore case.
func CompileRegex(pattern string) (*Query, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("--regex: %w", err)
	}
	return &Query{re: re}, nil
}

// Terms are the distinct terms the query looks for (none in regex mode).
func (q *Query) Terms() []string { return q.terms }

// Regex reports whether the query is a regular expression.
func (q *Query) Regex() bool { return q.re != nil }

// Score is how well text matches: the number of distinct terms it contains
// (1 for a regex match). ok is false when no phrasing matches in full.
func (q *Query) Score(text string) (score int, ok bool) {
	if q.re != nil {
		return 1, q.re.MatchString(text)
	}
	lower := strings.ToLower(text)
	has := make(map[string]bool, len(q.terms))
	for _, t := range q.terms {
		for _, v := range Variants(t) {
			if strings.Contains(lower, v) {
				has[t] = true
				break
			}
		}
	}
	for _, p := range q.phrasings {
		all := true
		for _, t := range p {
			if !has[t] {
				all = false
				break
			}
		}
		if all {
			return len(has), true
		}
	}
	return 0, false
}

// jsonSafe reports whether every term is written as itself inside a JSON
// string. A term with quotes, backslashes, <, >, & or control characters is
// escaped in the files, so raw-line checks could miss it.
func (q *Query) jsonSafe() bool {
	if q.re != nil {
		return false
	}
	for _, t := range q.terms {
		b, _ := json.Marshal(t)
		if string(b) != `"`+t+`"` {
			return false
		}
	}
	return true
}

// Needles are fixed strings a matching line must contain at least one of,
// for a prefilter; nil means every line must be read. A line matches a
// phrasing only if it holds every term, so one term per phrasing (the
// longest, the likeliest to be rare) is enough to narrow on.
func (q *Query) Needles() []string {
	if !q.jsonSafe() {
		return nil
	}
	var out []string
	for _, p := range q.phrasings {
		longest := p[0]
		for _, t := range p[1:] {
			if len(t) > len(longest) {
				longest = t
			}
		}
		out = append(out, Variants(longest)...)
	}
	return dedupe(out)
}

// MaybeLine is a cheap necessary condition on a raw stored line: for some
// phrasing, every term appears somewhere in it. A line it rejects cannot
// match; one it accepts must still be decoded and scored.
func (q *Query) MaybeLine(line []byte) bool {
	if !q.jsonSafe() {
		return true
	}
	lower := bytes.ToLower(line)
	for _, p := range q.phrasings {
		all := true
		for _, t := range p {
			found := false
			for _, v := range Variants(t) {
				if bytes.Contains(lower, []byte(v)) {
					found = true
					break
				}
			}
			if !found {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// Snippet is up to width runes of text around the first match, on one line.
func (q *Query) Snippet(text string, width int) string {
	flat := strings.Join(strings.Fields(text), " ")
	if width <= 0 || utf8.RuneCountInString(flat) <= width {
		return flat
	}
	at := -1
	if q.re != nil {
		if loc := q.re.FindStringIndex(flat); loc != nil {
			at = loc[0]
		}
	} else {
		lower := strings.ToLower(flat)
		for _, t := range q.terms {
			for _, v := range Variants(t) {
				if i := strings.Index(lower, v); i >= 0 && (at < 0 || i < at) {
					at = i
				}
			}
		}
		// Lowercasing can change byte lengths; fall back to the start when
		// the offset does not land on a rune boundary.
		if at > len(flat) || (at > 0 && !utf8.RuneStart(flat[at])) {
			at = 0
		}
	}
	runes := []rune(flat)
	center := 0
	if at > 0 {
		center = utf8.RuneCountInString(flat[:at])
	}
	start := max(0, center-width/3)
	end := min(len(runes), start+width)
	start = max(0, end-width)
	out := string(runes[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return out
}

// Ranked is one match, for Sort.
type Ranked struct {
	Score int
	TS    string // RFC3339; later sorts first
	ID    string
}

// Less ranks by distinct terms hit, then newest, then id.
func Less(a, b Ranked) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.TS != b.TS {
		return newer(a.TS, b.TS)
	}
	return a.ID < b.ID
}

// newer compares RFC3339 timestamps; one that does not parse sorts as text.
func newer(a, b string) bool {
	ta, ea := time.Parse(time.RFC3339Nano, a)
	tb, eb := time.Parse(time.RFC3339Nano, b)
	if ea == nil && eb == nil {
		return ta.After(tb)
	}
	return a > b
}

// Sort orders items best first by rank.
func Sort[T any](items []T, rank func(T) Ranked) {
	sort.SliceStable(items, func(i, j int) bool { return Less(rank(items[i]), rank(items[j])) })
}

// RgPath is ripgrep's path, or "" when it is not on PATH. HEV_GREP=go forces
// the Go walk.
func RgPath() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("HEV_GREP")), "go") {
		return ""
	}
	p, err := exec.LookPath("rg")
	if err != nil {
		return ""
	}
	return p
}

// Engine names the search path a query would take: "rg" or "go".
func Engine(q *Query) string {
	if q != nil && len(q.Needles()) > 0 && RgPath() != "" {
		return "rg"
	}
	return "go"
}

// Lines calls fn for each line, in each of paths (files or directories,
// walked recursively for files named *glob), that may match q: with rg the
// lines containing a needle, with the Go walk every line holding a needle
// (or every line, when q has none); either way only lines that pass
// q.MaybeLine. fn is called from several goroutines at once and must be safe
// for that; it returns false to stop. Files that vanish mid-walk are skipped.
func Lines(ctx context.Context, q *Query, paths []string, glob string, fn func(path string, line []byte) bool) error {
	if len(paths) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var stopped atomic.Bool
	type item struct {
		path string
		line []byte
	}
	work := make(chan item, 1024)
	var wg sync.WaitGroup
	for w := 0; w < workers(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range work {
				if stopped.Load() || !q.MaybeLine(it.line) {
					continue
				}
				if !fn(it.path, it.line) {
					stopped.Store(true)
					cancel()
				}
			}
		}()
	}
	emit := func(path string, line []byte) bool {
		if stopped.Load() {
			return false
		}
		select {
		case work <- item{path, line}:
			return true
		case <-ctx.Done():
			return false
		}
	}
	err := func() error {
		needles := q.Needles()
		if rg := RgPath(); rg != "" && len(needles) > 0 {
			err := rgLines(ctx, rg, needles, paths, glob, emit)
			if !errors.Is(err, errRgUnusable) {
				return err
			}
		}
		return goLines(ctx, needles, paths, glob, emit)
	}()
	close(work)
	wg.Wait()
	if stopped.Load() {
		return nil
	}
	return err
}

func workers() int { return max(2, min(runtime.NumCPU(), 16)) }

var errRgUnusable = errors.New("rg unusable")

func rgLines(ctx context.Context, rg string, needles, paths []string, glob string, fn func(string, []byte) bool) error {
	var existing []string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
		}
	}
	if len(existing) == 0 {
		return nil
	}
	args := []string{"--json", "--no-config", "--no-ignore", "--ignore-case", "--fixed-strings", "--no-messages", "--glob", glob, "--glob", "!.*"}
	for _, n := range needles {
		args = append(args, "-e", n)
	}
	args = append(args, "--")
	args = append(args, existing...)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, rg, args...)
	cmd.Env = append(os.Environ(), "RIPGREP_CONFIG_PATH=")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return errRgUnusable
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return errRgUnusable
	}
	// rg's JSON events are decoded on several goroutines: one reader would
	// be slower than rg itself on long lines.
	var stoppedFlag atomic.Bool
	events := make(chan []byte, 256)
	var wg sync.WaitGroup
	for w := 0; w < workers(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for raw := range events {
				if stoppedFlag.Load() {
					continue
				}
				var ev struct {
					Type string `json:"type"`
					Data struct {
						Path  rgText `json:"path"`
						Lines rgText `json:"lines"`
					} `json:"data"`
				}
				if json.Unmarshal(raw, &ev) != nil || ev.Type != "match" {
					continue
				}
				line := bytes.TrimRight(ev.Data.Lines.bytes(), "\r\n")
				if !fn(ev.Data.Path.string(), line) {
					stoppedFlag.Store(true)
					cancel()
				}
			}
		}()
	}
	matchPrefix := []byte(`{"type":"match"`)
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<30)
	for sc.Scan() {
		if stoppedFlag.Load() {
			continue // drain, so rg is not blocked writing
		}
		if b := sc.Bytes(); bytes.HasPrefix(b, matchPrefix) {
			events <- append([]byte(nil), b...)
		}
	}
	close(events)
	wg.Wait()
	stopped := stoppedFlag.Load()
	scanErr := sc.Err()
	waitErr := cmd.Wait()
	if stopped {
		return nil
	}
	if scanErr != nil {
		return scanErr
	}
	if waitErr != nil {
		var exit *exec.ExitError
		// 1 is "no match"; 2 with output already read is a file that
		// vanished mid-walk, which --no-messages keeps quiet.
		if errors.As(waitErr, &exit) && (exit.ExitCode() == 1 || (exit.ExitCode() == 2 && stderr.Len() == 0)) {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("rg: %v: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// rgText is ripgrep's {"text": ...} or {"bytes": base64} value.
type rgText struct {
	Text  *string `json:"text"`
	Bytes *string `json:"bytes"`
}

func (t rgText) bytes() []byte {
	if t.Text != nil {
		return []byte(*t.Text)
	}
	if t.Bytes != nil {
		b, _ := base64.StdEncoding.DecodeString(*t.Bytes)
		return b
	}
	return nil
}

func (t rgText) string() string { return string(t.bytes()) }

// goLines reads every line of the files under paths, several files at a
// time, and hands each to fn; the needle check runs in Lines' workers.
func goLines(ctx context.Context, _ []string, paths []string, glob string, fn func(string, []byte) bool) error {
	var files []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return nil
				}
				return err
			}
			name := d.Name()
			if path != p && strings.HasPrefix(name, ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if ok, _ := filepath.Match(glob, name); ok {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	var stopped atomic.Bool
	var firstErr error
	var mu sync.Mutex
	next := make(chan string)
	var wg sync.WaitGroup
	for w := 0; w < workers(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range next {
				if stopped.Load() {
					continue
				}
				if err := readLines(path, func(line []byte) bool {
					if !fn(path, line) {
						stopped.Store(true)
						return false
					}
					return true
				}); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					stopped.Store(true)
				}
			}
		}()
	}
	for _, f := range files {
		if stopped.Load() || ctx.Err() != nil {
			break
		}
		next <- f
	}
	close(next)
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

// readLines calls fn with each non-empty line of a file (a copy fn may
// keep); a file that is gone is empty.
func readLines(path string, fn func([]byte) bool) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<16)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimRight(line, "\r\n")
			if len(line) > 0 && !fn(line) {
				return nil
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
