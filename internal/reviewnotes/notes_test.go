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

func TestParseAppliesSidecarRules(t *testing.T) {
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
