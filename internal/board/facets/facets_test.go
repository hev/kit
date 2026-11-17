package facets

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const testTaxonomy = `{
  "version": "test",
  "org": "example-org",
  "facets": {
    "repo": {"values": [{"value": "alpha", "definition": "x"}, "beta"]},
    "host": {"values": ["box-one"]},
    "tool": {"values": ["docker", "git"]},
    "area": {"values": ["ci", "deploy"]},
    "client": {"values": ["acme"]}
  },
  "tag_mapping": [
    {"tag": "ci", "maps_to": "area:ci"},
    {"tag": "Docker", "maps_to": "tool:docker"},
    {"tag": "alpha", "maps_to": "repo:alpha"},
    {"tag": "fix", "maps_to": "drop"},
    {"tag": "gotcha", "maps_to": "kind:gotcha"},
    {"tag": "stale", "maps_to": "tool:not-in-the-list"}
  ]
}`

func tax(t *testing.T) *Taxonomy {
	t.Helper()
	x, err := Parse([]byte(testTaxonomy))
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func TestCheckAcceptsKnownValuesAndNormalizes(t *testing.T) {
	set, rejects := tax(t).Check(Set{Repo: []string{" Alpha", "beta", "alpha"}, Area: []string{"CI"}, Client: []string{"acme"}}, nil)
	if len(rejects) > 0 {
		t.Fatalf("rejects: %v", rejects)
	}
	if strings.Join(set.Repo, ",") != "alpha,beta" || set.Area[0] != "ci" {
		t.Fatalf("not normalized: %+v", set)
	}
}

func TestCheckRejectsWithoutTruncating(t *testing.T) {
	long := strings.Repeat("a", MaxBytes+1)
	many := make([]string, MaxValues+1)
	for i := range many {
		many[i] = "v" + string(rune('a'+i))
	}
	cases := []struct {
		name string
		in   Set
		want string
	}{
		{"unknown tool", Set{Tool: []string{"kettle"}}, "closed tool list"},
		{"unknown host", Set{Host: []string{"box-two"}}, "closed host list"},
		{"unknown area", Set{Area: []string{"cooking"}}, "closed area list"},
		{"unknown client", Set{Client: []string{"other"}}, "closed client list"},
		{"unknown repo", Set{Repo: []string{"gamma"}}, "repo list"},
		{"bad repo name", Set{Repo: []string{"not a repo"}}, "not a repository name"},
		{"oversize value", Set{Area: []string{long}}, "bytes, at most 64"},
		{"too many values", Set{Tool: many}, "values, at most 12"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			set, rejects := tax(t).Check(c.in, nil)
			if len(rejects) == 0 || !strings.Contains(rejects[0].Reason, c.want) {
				t.Fatalf("rejects %v, want %q", rejects, c.want)
			}
			// Never truncated: the set handed back still holds every value.
			for _, n := range Names {
				if got, in := len(set.Get(n)), len(c.in.Normal().Get(n)); got != in {
					t.Fatalf("%s: %d values back, %d in", n, got, in)
				}
			}
		})
	}
	if err := (Set{Tool: many}).CheckBounds(); err == nil {
		t.Fatal("bounds accepted 13 values")
	}
	if err := CheckBy(strings.Repeat("x", MaxByBytes+1)); err == nil {
		t.Fatal("oversize facets_by accepted")
	}
}

func TestRepoCheckedAgainstOrg(t *testing.T) {
	x := tax(t)
	asked := ""
	checker := func(org, repo string) (bool, error) {
		asked = org + "/" + repo
		return repo == "gamma", nil
	}
	if _, rejects := x.Check(Set{Repo: []string{"gamma"}}, checker); len(rejects) > 0 || asked != "example-org/gamma" {
		t.Fatalf("org repo rejected: %v (asked %s)", rejects, asked)
	}
	if _, rejects := x.Check(Set{Repo: []string{"delta"}}, checker); len(rejects) != 1 || !strings.Contains(rejects[0].Reason, "example-org org") {
		t.Fatalf("missing repo accepted: %v", rejects)
	}
	broken := func(string, string) (bool, error) { return false, errors.New("offline") }
	if _, rejects := x.Check(Set{Repo: []string{"delta"}}, broken); len(rejects) != 1 || !strings.Contains(rejects[0].Reason, "offline") {
		t.Fatalf("failed check accepted: %v", rejects)
	}
}

func TestFromTagsUsesOnlyFacetTargetsInTheList(t *testing.T) {
	got, err := tax(t).FromTags([]string{"CI", "docker", "fix", "gotcha", "stale", "alpha", "unmapped"})
	if err != nil || got.String() != "repo:alpha tool:docker area:ci" {
		t.Fatalf("from tags: %q %v", got.String(), err)
	}
}

// A tag map that gives a facet more values than it holds is an error with
// the whole mapped set, never the first MaxValues of it.
func TestFromTagsOverCountIsRejectedNotTruncated(t *testing.T) {
	var vals, tags []string
	var m []string
	for i := 0; i <= MaxValues; i++ {
		v := fmt.Sprintf("tool%02d", i)
		vals = append(vals, fmt.Sprintf("%q", v))
		tags = append(tags, "t"+v)
		m = append(m, fmt.Sprintf(`{"tag":"t%s","maps_to":"tool:%s"}`, v, v))
	}
	x, err := Parse([]byte(`{"facets":{"tool":{"values":[` + strings.Join(vals, ",") + `]}},"tag_mapping":[` + strings.Join(m, ",") + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := x.FromTags(tags)
	if err == nil || !strings.Contains(err.Error(), "tool: 13 values, at most 12") {
		t.Fatalf("over-count tag map accepted: %v", err)
	}
	if len(got.Tool) != MaxValues+1 {
		t.Fatalf("mapped set cut to %d values", len(got.Tool))
	}
	if _, err := x.Keep(Set{Tool: tags[:0]}); err != nil {
		t.Fatal(err)
	}
	if kept, err := x.Keep(got); err == nil || len(kept.Tool) != MaxValues+1 {
		t.Fatalf("keep truncated or accepted: %d %v", len(kept.Tool), err)
	}
	if ok, err := x.FromTags(tags[:MaxValues]); err != nil || len(ok.Tool) != MaxValues {
		t.Fatalf("a full facet is fine: %d %v", len(ok.Tool), err)
	}
}

// facets_by records the author's identity; the plain and prefixed forms and
// triage are all a person's, which the backfill never overwrites.
func TestAuthorIdentityInFacetsBy(t *testing.T) {
	if got := Author("tester"); got != "author:tester" {
		t.Fatalf("author: %q", got)
	}
	if got := Author("  "); got != ByAuthor {
		t.Fatalf("no identity: %q", got)
	}
	for _, by := range []string{"author", "author:tester", "triage:s1"} {
		if !UserSet(by) {
			t.Errorf("%q is a person's", by)
		}
	}
	for _, by := range []string{"", "tag-map", "inventory-20990101", "inventory-20990101:low", "authored"} {
		if UserSet(by) {
			t.Errorf("%q is not a person's", by)
		}
	}
	if MaxByBytes != 160 {
		t.Fatalf("facets_by bound %d, the design says 160", MaxByBytes)
	}
	if err := CheckBy(Author(strings.Repeat("x", 160-len(AuthorPrefix)))); err != nil {
		t.Fatalf("160 bytes refused: %v", err)
	}
}

func TestSetAlgebra(t *testing.T) {
	a := Set{Repo: []string{"b", "a"}, Area: []string{"ci"}}
	b := Set{Repo: []string{"a", "c"}, Tool: []string{"git"}}
	if u := a.Union(b); u.String() != "repo:a repo:b repo:c tool:git area:ci" {
		t.Fatalf("union %q", u.String())
	}
	if w := a.Without(Set{Repo: []string{"a"}}); w.String() != "repo:b area:ci" {
		t.Fatalf("without %q", w.String())
	}
	if f := (Set{Repo: []string{"x"}}).Fill(b); f.String() != "repo:x tool:git" {
		t.Fatalf("fill %q", f.String())
	}
	if !a.Equal(Set{Repo: []string{"A", "b"}, Area: []string{"ci"}}) || a.Equal(b) {
		t.Fatal("equal")
	}
	if (Set{}).Ptr() != nil || (Set{Repo: []string{" "}}).Ptr() != nil {
		t.Fatal("empty set should have no wire form")
	}
	if _, _, err := ParsePair("kind:fix"); err == nil {
		t.Fatal("non-facet pair accepted")
	}
}

func TestDefaultTaxonomyIsGeneric(t *testing.T) {
	d := Default()
	if d.Path != "" || len(d.Values("tool")) == 0 || d.TagCount() == 0 {
		t.Fatalf("default taxonomy: %+v", d)
	}
	if _, err := Load("/does/not/exist.json"); err == nil {
		t.Fatal("missing taxonomy file accepted")
	}
}
