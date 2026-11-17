package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// The published schema and the Go event must not drift.
func TestSchemaMatchesEvent(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "schemas", "hook_event.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Title      string
		Required   []string
		Properties map[string]struct {
			Const      string
			Enum       []string
			Properties map[string]struct{ Enum []string }
		}
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s.Title != Schema || s.Properties["schema"].Const != Schema {
		t.Fatalf("schema version %q", s.Title)
	}
	if !reflect.DeepEqual(s.Properties["event"].Enum, Events) || !reflect.DeepEqual(s.Properties["harness"].Enum, Harnesses) {
		t.Fatal("event or harness enum drifted")
	}
	if got := s.Properties["tool"].Properties["kind"].Enum; !reflect.DeepEqual(got, []string{KindRead, KindMutate, KindSteer, KindOther}) {
		t.Fatal(got)
	}
	ev, _ := Normalize("codex", "pre-tool", []byte(codexPre), time.Now())
	raw, _ := json.Marshal(ev)
	var m map[string]any
	json.Unmarshal(raw, &m)
	for k := range m {
		if _, ok := s.Properties[k]; !ok {
			t.Errorf("event field %q missing from the schema", k)
		}
	}
	for _, k := range s.Required {
		if _, ok := m[k]; !ok {
			t.Errorf("required %q missing from an event", k)
		}
	}
}
