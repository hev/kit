// Package facets is the five filterable subjects a board thread carries:
// repo, host, tool, area and client. A Set is what a thread row stores; a
// Taxonomy is the vocabulary every value is checked against before a write.
//
// kit ships only a generic example taxonomy (default.json). A real one is a
// private file named by board.taxonomy in kit's config or by --taxonomy.
package facets

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Names are the five facets, in the order they are shown.
var Names = []string{"repo", "host", "tool", "area", "client"}

const (
	// MaxValues is the most values one facet may hold on a thread row.
	MaxValues = 12
	// MaxBytes is the longest one value may be. 12 x 64 B keeps a facet
	// under 1 KB, far inside the store's 4 KB filterable limit.
	MaxBytes = 64
	// MaxByBytes bounds facets_by, which says who last set a row's facets.
	MaxByBytes = 64
)

// Who set a row's facets. A person's edit (Author, or Triage+session) wins
// over the inventory and the tag map: the backfill never overwrites it.
const (
	ByAuthor = "author"
	ByTagMap = "tag-map"
	Triage   = "triage:"
	// LowSuffix marks facets copied from an inventory row its author rated
	// low-confidence; the review page lists them.
	LowSuffix = ":low"
)

// Set is one thread's facets. Every field is omitted from JSON when empty,
// and a Set with nothing in it is not written at all.
type Set struct {
	Repo   []string `json:"repo,omitempty"`
	Host   []string `json:"host,omitempty"`
	Tool   []string `json:"tool,omitempty"`
	Area   []string `json:"area,omitempty"`
	Client []string `json:"client,omitempty"`
}

// Get is the values of one facet by name; nil for an unknown name.
func (s Set) Get(name string) []string {
	if p := s.ptr(name); p != nil {
		return *p
	}
	return nil
}

func (s *Set) ptr(name string) *[]string {
	switch name {
	case "repo":
		return &s.Repo
	case "host":
		return &s.Host
	case "tool":
		return &s.Tool
	case "area":
		return &s.Area
	case "client":
		return &s.Client
	}
	return nil
}

// Put replaces one facet's values.
func (s *Set) Put(name string, values []string) {
	if p := s.ptr(name); p != nil {
		*p = values
	}
}

// Add appends values to one facet.
func (s *Set) Add(name string, values ...string) {
	if p := s.ptr(name); p != nil {
		*p = append(*p, values...)
	}
}

// Empty reports whether no facet has a value.
func (s Set) Empty() bool {
	for _, n := range Names {
		if len(s.Get(n)) > 0 {
			return false
		}
	}
	return true
}

// Normal lowercases, trims, drops blanks and duplicates and sorts every facet,
// so two sets with the same values compare equal.
func (s Set) Normal() Set {
	var out Set
	for _, n := range Names {
		seen := map[string]bool{}
		var vs []string
		for _, v := range s.Get(n) {
			v = strings.ToLower(strings.TrimSpace(v))
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			vs = append(vs, v)
		}
		sort.Strings(vs)
		out.Put(n, vs)
	}
	return out
}

// Equal compares two sets after normalizing both.
func (s Set) Equal(o Set) bool {
	a, b := s.Normal(), o.Normal()
	for _, n := range Names {
		x, y := a.Get(n), b.Get(n)
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if x[i] != y[i] {
				return false
			}
		}
	}
	return true
}

// Union is s with every value of o added.
func (s Set) Union(o Set) Set {
	var out Set
	for _, n := range Names {
		out.Put(n, append(append([]string{}, s.Get(n)...), o.Get(n)...))
	}
	return out.Normal()
}

// Without is s with o's values removed.
func (s Set) Without(o Set) Set {
	var out Set
	for _, n := range Names {
		drop := map[string]bool{}
		for _, v := range o.Normal().Get(n) {
			drop[v] = true
		}
		for _, v := range s.Normal().Get(n) {
			if !drop[v] {
				out.Add(n, v)
			}
		}
	}
	return out
}

// Fill is s with each facet s leaves empty taken from o.
func (s Set) Fill(o Set) Set {
	out := s.Normal()
	o = o.Normal()
	for _, n := range Names {
		if len(out.Get(n)) == 0 {
			out.Put(n, o.Get(n))
		}
	}
	return out
}

// Ptr is s, or nil when s is empty: the form a Thread carries on the wire so
// that a thread without facets keeps the JSON it always had.
func (s Set) Ptr() *Set {
	n := s.Normal()
	if n.Empty() {
		return nil
	}
	return &n
}

// String is "repo:a repo:b area:c", for previews and logs.
func (s Set) String() string {
	var parts []string
	for _, n := range Names {
		for _, v := range s.Get(n) {
			parts = append(parts, n+":"+v)
		}
	}
	return strings.Join(parts, " ")
}

// CheckBounds rejects a set past MaxValues values or MaxBytes per value in
// any facet. It never truncates: a write that does not fit is refused.
func (s Set) CheckBounds() error {
	for _, n := range Names {
		vs := s.Get(n)
		if len(vs) > MaxValues {
			return fmt.Errorf("%s: %d values, at most %d", n, len(vs), MaxValues)
		}
		for _, v := range vs {
			if len(v) > MaxBytes {
				return fmt.Errorf("%s: value of %d bytes, at most %d", n, len(v), MaxBytes)
			}
		}
	}
	return nil
}

// CheckBy rejects a facets_by value past MaxByBytes.
func CheckBy(by string) error {
	if len(by) > MaxByBytes {
		return fmt.Errorf("facets_by of %d bytes, at most %d", len(by), MaxByBytes)
	}
	return nil
}

// UserSet reports whether by names a person (an author or a triage session)
// rather than a machine source. The backfill never overwrites those.
func UserSet(by string) bool {
	return by == ByAuthor || strings.HasPrefix(by, Triage)
}

// Taxonomy is the vocabulary: a closed list per facet (repo is checked
// against its list and then, where a checker is supplied, the org), and the
// tag map that turns a thread's tags into facets.
type Taxonomy struct {
	Version string
	Org     string
	Path    string // where it was read from; "" for the built-in example
	values  map[string]map[string]bool
	tags    map[string][]tagTarget
}

type tagTarget struct{ facet, value string }

type fileFormat struct {
	Version string `json:"version"`
	Org     string `json:"org"`
	Facets  map[string]struct {
		Values []json.RawMessage `json:"values"`
	} `json:"facets"`
	TagMapping []struct {
		Tag    string `json:"tag"`
		MapsTo string `json:"maps_to"`
	} `json:"tag_mapping"`
}

//go:embed default.json
var defaultJSON []byte

// Default is the generic example taxonomy kit ships.
func Default() *Taxonomy {
	t, err := Parse(defaultJSON)
	if err != nil {
		panic("facets: built-in taxonomy: " + err.Error())
	}
	return t
}

// Load reads a taxonomy file; an empty path is the built-in example.
func Load(path string) (*Taxonomy, error) {
	if path == "" {
		return Default(), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("taxonomy: %w", err)
	}
	t, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("taxonomy %s: %w", path, err)
	}
	t.Path = path
	return t, nil
}

// Parse reads the taxonomy format: facets.NAME.values is a list of strings or
// of objects with a "value" field; tag_mapping maps a tag to "facet:value"
// (other targets, such as "drop" or "kind:…", are ignored here).
func Parse(b []byte) (*Taxonomy, error) {
	var f fileFormat
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	t := &Taxonomy{Version: f.Version, Org: f.Org, values: map[string]map[string]bool{}, tags: map[string][]tagTarget{}}
	for _, n := range Names {
		t.values[n] = map[string]bool{}
		for _, raw := range f.Facets[n].Values {
			var v string
			if err := json.Unmarshal(raw, &v); err != nil {
				var obj struct {
					Value string `json:"value"`
				}
				if err := json.Unmarshal(raw, &obj); err != nil || obj.Value == "" {
					return nil, fmt.Errorf("facets.%s.values: want a string or {\"value\": …}", n)
				}
				v = obj.Value
			}
			v = strings.ToLower(strings.TrimSpace(v))
			if v == "" {
				continue
			}
			if len(v) > MaxBytes {
				return nil, fmt.Errorf("facets.%s: value %q is over %d bytes", n, v, MaxBytes)
			}
			t.values[n][v] = true
		}
	}
	for _, m := range f.TagMapping {
		facet, value, ok := strings.Cut(m.MapsTo, ":")
		if !ok || !isFacet(facet) {
			continue
		}
		tag := strings.ToLower(strings.TrimSpace(m.Tag))
		value = strings.ToLower(strings.TrimSpace(value))
		if tag == "" || value == "" {
			continue
		}
		t.tags[tag] = append(t.tags[tag], tagTarget{facet, value})
	}
	return t, nil
}

func isFacet(name string) bool {
	for _, n := range Names {
		if n == name {
			return true
		}
	}
	return false
}

// Values is the sorted list of known values for one facet.
func (t *Taxonomy) Values(facet string) []string {
	var out []string
	for v := range t.values[facet] {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// Known reports whether value is in facet's list.
func (t *Taxonomy) Known(facet, value string) bool {
	return t.values[facet][strings.ToLower(strings.TrimSpace(value))]
}

// RepoChecker answers whether a repository outside the taxonomy's repo list
// exists in the org. nil means no check is available: unknown repos are
// rejected.
type RepoChecker func(org, repo string) (bool, error)

var repoName = regexp.MustCompile(`^[a-z0-9._-]+$`)

// Reject is one value that failed validation, and why.
type Reject struct {
	Facet  string `json:"facet"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

func (r Reject) String() string { return r.Facet + ":" + r.Value + ": " + r.Reason }

// Check validates every value of s against the taxonomy and the bounds. It
// returns the normalized set and every rejected value; it never drops or
// shortens a value to make a set fit.
func (t *Taxonomy) Check(s Set, repos RepoChecker) (Set, []Reject) {
	n := s.Normal()
	var rejects []Reject
	for _, name := range Names {
		vs := n.Get(name)
		if len(vs) > MaxValues {
			rejects = append(rejects, Reject{name, strings.Join(vs, ","), fmt.Sprintf("%d values, at most %d", len(vs), MaxValues)})
		}
		for _, v := range vs {
			if len(v) > MaxBytes {
				rejects = append(rejects, Reject{name, v, fmt.Sprintf("%d bytes, at most %d", len(v), MaxBytes)})
				continue
			}
			if t.Known(name, v) {
				continue
			}
			if name != "repo" {
				rejects = append(rejects, Reject{name, v, "not in the taxonomy's closed " + name + " list"})
				continue
			}
			if !repoName.MatchString(v) {
				rejects = append(rejects, Reject{name, v, "not a repository name"})
				continue
			}
			if repos == nil || t.Org == "" {
				rejects = append(rejects, Reject{name, v, "not in the taxonomy's repo list"})
				continue
			}
			ok, err := repos(t.Org, v)
			switch {
			case err != nil:
				rejects = append(rejects, Reject{name, v, "org check failed: " + err.Error()})
			case !ok:
				rejects = append(rejects, Reject{name, v, "not in the taxonomy's repo list or the " + t.Org + " org"})
			}
		}
	}
	return n, rejects
}

// Keep is s with every rejected value removed: what an inferred (not
// explicitly given) set keeps. Explicit values are never filtered this way;
// they are rejected as a whole.
func (t *Taxonomy) Keep(s Set) Set {
	n := s.Normal()
	var out Set
	for _, name := range Names {
		for _, v := range n.Get(name) {
			if t.Known(name, v) && len(out.Get(name)) < MaxValues {
				out.Add(name, v)
			}
		}
	}
	return out
}

// FromTags maps a thread's tags through the taxonomy's tag map.
func (t *Taxonomy) FromTags(tags []string) Set {
	var out Set
	for _, tag := range tags {
		for _, tt := range t.tags[strings.ToLower(strings.TrimSpace(tag))] {
			out.Add(tt.facet, tt.value)
		}
	}
	return t.Keep(out)
}

// TagCount is how many tags the map knows, for status lines.
func (t *Taxonomy) TagCount() int { return len(t.tags) }

// Parse a "facet:value" string, as chips and forms send them.
func ParsePair(s string) (facet, value string, err error) {
	facet, value, ok := strings.Cut(strings.TrimSpace(s), ":")
	facet = strings.ToLower(strings.TrimSpace(facet))
	value = strings.ToLower(strings.TrimSpace(value))
	if !ok || !isFacet(facet) || value == "" {
		return "", "", fmt.Errorf("want facet:value with facet one of %s, got %q", strings.Join(Names, ", "), s)
	}
	return facet, value, nil
}
