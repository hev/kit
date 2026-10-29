package layer

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func assertIndexPolicy(t *testing.T, schema map[string]any, kind string) {
	t.Helper()
	wanted := map[string]bool{}
	for _, field := range IndexedAttributes[kind] {
		wanted[field] = true
	}
	for field, raw := range schema {
		definition := raw.(map[string]any)
		enabled, explicit := definition["filterable"].(bool)
		if !explicit || enabled != wanted[field] {
			t.Errorf("%s.%s filterable=%v; expected explicit %v", kind, field, definition["filterable"], wanted[field])
		}
		if strings.HasPrefix(field, "commit_outcome_") || strings.HasPrefix(field, "ci_workflow_") || strings.HasPrefix(field, "mark_") {
			t.Errorf("dynamic column declared: %s", field)
		}
	}
}

func TestDeclaredIndexPolicy(t *testing.T) {
	var policy map[string][]string
	raw, err := os.ReadFile("../../docs/schema-cut/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &policy); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(policy, IndexedAttributes) {
		t.Fatal("documented index policy differs from code")
	}
	for _, arrays := range []bool{false, true} {
		assertIndexPolicy(t, New("", "", "", "").schema(), "chunks")
		assertIndexPolicy(t, blockSchema(), "blocks")
		assertIndexPolicy(t, sessionSchema(arrays), "sessions")
		assertIndexPolicy(t, outcomeSchema(arrays), "sessions")
	}
	for kind, fields := range policy {
		ns := "hev-traces-" + kind
		if kind == "chunks" {
			ns = "hev-traces"
		}
		raw, err := os.ReadFile("../../docs/schema-cut/" + ns + ".schema.json")
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err = json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		assertIndexPolicy(t, schema, kind)
		for _, field := range fields {
			if schema[field] == nil {
				t.Errorf("repair baseline missing %s.%s", kind, field)
			}
		}
	}
}
