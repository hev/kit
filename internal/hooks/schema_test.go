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

func TestActivitySchemasMatchEvents(t *testing.T) {
	content := "activity"
	peer := PeerMessage{Kind: "note", Text: &content, ReplyTo: "parent"}
	peer.Activity.complete("hev.peer.message/v1", "peer-message")
	post := BoardPost{Kind: "reply", Body: &content, ThreadID: 1}
	post.Activity.complete("hev.board.post/v1", "board-post")

	for _, tt := range []struct {
		name, schema, event string
		value               any
		kinds               []string
	}{
		{"peer_message", peer.Schema, peer.Event, peer, []string{"note", "urgent", "halt", "chat"}},
		{"board_post", post.Schema, post.Event, post, []string{"post", "reply"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "schemas", tt.name+"_event.schema.json"))
			if err != nil {
				t.Fatal(err)
			}
			var spec struct {
				Title      string
				Required   []string
				Properties map[string]struct {
					Const string
					Enum  []string
				}
			}
			if err := json.Unmarshal(data, &spec); err != nil {
				t.Fatal(err)
			}
			if spec.Title != tt.schema || spec.Properties["schema"].Const != tt.schema ||
				spec.Properties["event"].Const != tt.event || !reflect.DeepEqual(spec.Properties["kind"].Enum, tt.kinds) {
				t.Fatalf("activity schema drift: %+v", spec)
			}
			raw, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			for field := range spec.Properties {
				if _, ok := fields[field]; !ok {
					t.Errorf("missing event field %q", field)
				}
			}
			for field := range fields {
				if _, ok := spec.Properties[field]; !ok {
					t.Errorf("missing schema property %q", field)
				}
			}
			for _, field := range spec.Required {
				if _, ok := fields[field]; !ok {
					t.Errorf("missing required field %q", field)
				}
			}
		})
	}
}
