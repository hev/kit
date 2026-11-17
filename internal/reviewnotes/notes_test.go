package reviewnotes

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestParseAppliesHunkRules(t *testing.T) {
	cases := []struct {
		name, doc, want string
	}{
		{"valid full", `{"version":1,"summary":"s","head":"abcdef1","files":[{"path":"a.go","summary":"f","annotations":[{"summary":"x","newRange":[2,4],"oldRange":[1,1],"rationale":"r","author":"run opus","id":"n1","tags":["finding","fail"],"confidence":"high","source":"run","createdAt":"2026-11-17T00:00:00Z","extra":true}]}],"unknown":1}`, ""},
		{"valid minimal", `{"files":[{"path":"a.go"}]}`, ""},
		{"valid empty", `{}`, ""},
		{"missing path", `{"files":[{"summary":"x"}]}`, "files[0].path: required"},
		{"blank path", `{"files":[{"path":"  "}]}`, "files[0].path: required"},
		{"missing annotation summary", `{"files":[{"path":"a","annotations":[{"newRange":[1,1]}]}]}`, "files[0].annotations[0].summary: required"},
		{"end before start", `{"files":[{"path":"a","annotations":[{"summary":"x","newRange":[3,2]}]}]}`, "newRange: end 2 is before start 3"},
		{"old end before start", `{"files":[{"path":"a","annotations":[{"summary":"x","oldRange":[9,1]}]}]}`, "oldRange: end 1 is before start 9"},
		{"zero line", `{"files":[{"path":"a","annotations":[{"summary":"x","newRange":[0,1]}]}]}`, "1-based"},
		{"one number", `{"files":[{"path":"a","annotations":[{"summary":"x","newRange":[2]}]}]}`, "want [start, end]"},
		{"fractional", `{"files":[{"path":"a","annotations":[{"summary":"x","newRange":[2.5,3]}]}]}`, "cannot unmarshal"},
		{"bad confidence", `{"files":[{"path":"a","annotations":[{"summary":"x","confidence":"sure"}]}]}`, "confidence"},
		{"bad version", `{"version":2,"files":[]}`, "unsupported 2"},
		{"empty input", ``, "empty"},
		{"not json", `nope`, "review notes:"},
	}
	for _, c := range cases {
		_, err := Parse([]byte(c.doc))
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want %q", c.name, err, c.want)
		}
	}
}

func TestValidateReportsEveryProblem(t *testing.T) {
	_, err := Parse([]byte(`{"files":[{"path":""},{"path":"b","annotations":[{"summary":"","newRange":[5,1]}]}]}`))
	var v *ValidationError
	if !errors.As(err, &v) || len(v.Problems) != 3 {
		t.Fatalf("want 3 problems, got %v", err)
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	n := Notes{Summary: "s", Files: []File{{Path: "a", Annotations: []Annotation{{Summary: "x", NewRange: Range{1, 2}}}}}}
	b, err := Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "oldRange") || !strings.Contains(string(b), `"version": 1`) {
		t.Fatalf("unexpected rendering:\n%s", b)
	}
	back, err := Parse(b)
	if err != nil || back.Files[0].Annotations[0].NewRange.Start() != 1 {
		t.Fatalf("round trip: %+v %v", back, err)
	}
}

func TestInspectStates(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, FileName)
	if s := Inspect(p); s.State != "missing" || !strings.Contains(s.Line(), "none") {
		t.Fatalf("missing: %+v", s)
	}
	os.WriteFile(p, []byte(`{"files":[{"path":""}]}`), 0600)
	if s := Inspect(p); s.State != "invalid" || !strings.Contains(s.Line(), "invalid") {
		t.Fatalf("invalid: %+v", s)
	}
	os.WriteFile(p, []byte(`{"head":"0123456789abcdef","files":[{"path":"a","annotations":[{"summary":"x"},{"summary":"y"}]}]}`), 0600)
	if s := Inspect(p); s.State != "valid" || s.Annotations != 2 || s.Files != 1 || !strings.Contains(s.Line(), "(1 file, 2 annotations at 0123456789ab)") {
		t.Fatalf("valid: %+v %s", s, s.Line())
	}
}

// The schema and the Go type describe the same fields.
func TestSchemaMatchesType(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "schemas", "review-notes.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Defs       map[string]struct {
			Required   []string                   `json:"required"`
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	check := func(name string, props map[string]json.RawMessage, typ reflect.Type) {
		var have, want []string
		for k := range props {
			have = append(have, k)
		}
		for i := 0; i < typ.NumField(); i++ {
			want = append(want, strings.Split(typ.Field(i).Tag.Get("json"), ",")[0])
		}
		sort.Strings(have)
		sort.Strings(want)
		if !reflect.DeepEqual(have, want) {
			t.Errorf("%s: schema %v, Go %v", name, have, want)
		}
	}
	check("notes", s.Properties, reflect.TypeOf(Notes{}))
	check("file", s.Defs["file"].Properties, reflect.TypeOf(File{}))
	check("annotation", s.Defs["annotation"].Properties, reflect.TypeOf(Annotation{}))
	if !reflect.DeepEqual(s.Defs["file"].Required, []string{"path"}) || !reflect.DeepEqual(s.Defs["annotation"].Required, []string{"summary"}) {
		t.Errorf("required fields drifted: %+v", s.Defs)
	}
}

func TestMergeOrderingAuthorshipAndStaleHeads(t *testing.T) {
	head := "1111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	build := Notes{Summary: "Adds review notes.", Head: head, Files: []File{
		{Path: "b.go", Summary: "b change", Annotations: []Annotation{{Summary: "why b", NewRange: Range{3, 4}, Author: "build opus", Source: "build"}}},
		{Path: "a.go", Summary: "a change"},
	}}
	review := Notes{Summary: "One finding.", Head: head[:7], Files: []File{
		{Path: "a.go", Summary: "review says", Annotations: []Annotation{{Summary: "bug", NewRange: Range{9, 9}, Tags: []string{"finding", "fail"}, Confidence: "high", Author: "review sonnet", Source: "review"}}},
		{Path: "c.go", Annotations: []Annotation{{Summary: "unattributed"}}},
		{Path: "b.go", Annotations: []Annotation{{Summary: "nit", Tags: []string{"finding", "note"}}}},
	}}
	stale := Notes{Head: "2222222", Files: []File{{Path: "a.go", Annotations: []Annotation{{Summary: "old finding"}}}}}
	unpinned := Notes{Files: []File{{Path: "d.go", Annotations: []Annotation{{Summary: "no head"}}}}}

	got, warnings := Merge(head, []Part{
		{Run: "build", Head: build.Head, Notes: build},
		{Run: "build-review", Review: true, Head: review.Head, Notes: review},
		{Run: "build-review-old", Review: true, Head: stale.Head, Notes: stale},
		{Run: "build-review-x", Review: true, Notes: unpinned},
	})
	var paths []string
	for _, f := range got.Files {
		paths = append(paths, f.Path)
	}
	if want := []string{"b.go", "a.go", "c.go", "d.go"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("file order %v, want %v", paths, want)
	}
	b := got.Files[0]
	if len(b.Annotations) != 2 || b.Annotations[0].Summary != "why b" || b.Annotations[1].Summary != "nit" {
		t.Fatalf("b.go annotations: %+v", b.Annotations)
	}
	if b.Annotations[1].Author != "build-review" || b.Annotations[1].Source != "build-review" {
		t.Fatalf("empty author/source not filled with run: %+v", b.Annotations[1])
	}
	a := got.Files[1]
	if a.Summary != "a change" || len(a.Annotations) != 1 || a.Annotations[0].Author != "review sonnet" || a.Annotations[0].Source != "review" {
		t.Fatalf("a.go: %+v", a)
	}
	for _, f := range got.Files {
		for _, an := range f.Annotations {
			if an.Summary == "old finding" {
				t.Fatal("stale review notes were merged")
			}
		}
	}
	if got.Head != head || got.Summary != "Adds review notes.\n\nbuild-review: One finding." {
		t.Fatalf("head/summary: %q %q", got.Head, got.Summary)
	}
	if len(warnings) != 2 || !strings.Contains(warnings[0], "dropped review notes from build-review-old") || !strings.Contains(warnings[1], "build-review-x") {
		t.Fatalf("warnings: %v", warnings)
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSameHead(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"abcdef0123", "ABCDEF0", true},
		{"abcdef0", "abcdef1", false},
		{"abc", "abc", true},
		{"abc", "abcdef0", false},
		{"", "", false},
	} {
		if SameHead(c.a, c.b) != c.want {
			t.Errorf("SameHead(%q,%q) != %v", c.a, c.b, c.want)
		}
	}
}

func TestReportVerdict(t *testing.T) {
	v, h := ReportVerdict("\nVERDICT: FAIL head=0123abcd4567 extra\nfindings")
	if v != "FAIL" || h != "0123abcd4567" {
		t.Fatalf("%q %q", v, h)
	}
	if v, h = ReportVerdict("VERDICT: PASS"); v != "PASS" || h != "" {
		t.Fatalf("%q %q", v, h)
	}
	if v, _ = ReportVerdict("# Report\nVERDICT: PASS"); v != "" {
		t.Fatal("verdict must be the first line")
	}
}

// Captured from hunk 0.25.0: `hunk session comment list --type all --json`
// with a sidecar note, a `session comment add` note, a note typed in the
// window with c, and a reply typed with R.
const hunkAllJSON = `{
  "comments": [
    {"noteId": "b1", "source": "ai", "filePath": "a.txt", "newRange": [2, 2], "body": "n1", "author": "bot", "createdAt": "2026-11-17T00:00:00Z", "editable": false},
    {"noteId": "mcp:0171bef4", "source": "agent", "filePath": "a.txt", "hunkIndex": 0, "newRange": [4, 4], "body": "agent added", "createdAt": "2026-11-17T03:25:36.202Z", "editable": false},
    {"noteId": "user:1794885937847-1", "source": "user", "filePath": "a.txt", "hunkIndex": 0, "newRange": [1, 1], "body": "human says rename", "author": "user", "createdAt": "2026-11-17T03:25:37.847Z", "editable": true},
    {"noteId": "user:1794885952124-2", "parentId": "b1", "source": "user", "filePath": "a.txt", "hunkIndex": 0, "newRange": [2, 2], "body": "this finding is wrong", "author": "user", "createdAt": "2026-11-17T03:25:52.124Z", "editable": true},
    {"noteId": "user:3", "source": "user", "filePath": "z.go", "oldRange": [7, 9], "body": "why delete\nthis?", "author": "user", "createdAt": "2026-11-17T03:26:00Z", "editable": true}
  ]
}`

func TestParseHunkComments(t *testing.T) {
	items, err := ParseHunkComments([]byte(hunkAllJSON), false)
	if err != nil {
		t.Fatal(err)
	}
	var anchors []string
	for _, it := range items {
		anchors = append(anchors, it.Anchor())
	}
	if want := []string{"a.txt:1", "a.txt:2", "z.go:7-9 (old side)"}; !reflect.DeepEqual(anchors, want) {
		t.Fatalf("anchors %v, want %v", anchors, want)
	}
	if items[1].ReplyTo == nil || items[1].ReplyTo.Body != "n1" || items[1].ReplyTo.Author != "bot" {
		t.Fatalf("reply parent: %+v", items[1].ReplyTo)
	}
	msg := FormatItems("scratch", "0123456789abcdef0", items)
	for _, want := range []string{
		"Review notes for scratch at 0123456789ab from hunk (3):",
		"1. a.txt:1: human says rename",
		`2. a.txt:2 (reply to bot note "n1"): this finding is wrong`,
		"3. z.go:7-9 (old side): why delete\n   this?",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	live, _ := ParseHunkComments([]byte(hunkAllJSON), true)
	if len(live) != 4 || live[2].Text != "agent added" {
		t.Fatalf("include live: %+v", live)
	}
}

func TestParseHunkCommentsErrors(t *testing.T) {
	if _, err := ParseHunkComments([]byte(`{"error":{"code":"daemon-build-mismatch"}}`), false); err == nil || !strings.Contains(err.Error(), "daemon-build-mismatch") {
		t.Fatalf("in-band error: %v", err)
	}
	if _, err := ParseHunkComments([]byte(`hunk: no session`), false); err == nil {
		t.Fatal("non-JSON accepted")
	}
	items, err := ParseHunkComments([]byte(`{"comments":[]}`), false)
	if err != nil || len(items) != 0 {
		t.Fatal(items, err)
	}
}
