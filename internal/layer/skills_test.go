package layer

import (
	"encoding/json"
	"github.com/hev/kit/internal/trace"
	"testing"
)

func TestSkillSchemasAndBlockWire(t *testing.T) {
	for _, arrays := range []bool{true, false} {
		field := sessionSchema(arrays)["skills"].(map[string]any)
		if field["filterable"] != true {
			t.Fatal("skills must be explicitly filterable")
		}
		want := "string"
		if arrays {
			want = "[]string"
		}
		if field["type"] != want {
			t.Fatalf("schema=%v", field)
		}
	}
	if blockSchema()["skill_loads"].(map[string]any)["filterable"] != false {
		t.Fatal("events must not be filterable")
	}
	cl, body := capture(t, `{"status":"OK"}`)
	loads := trace.SkillLoads{{Name: "board", Path: "/repo/skills/board/SKILL.md", Source: "/repo/skills", Turn: "turn", Seq: 2}}
	if _, err := cl.WriteBlocks([]trace.BlockRow{{ID: "block", SkillLoads: loads}}); err != nil {
		t.Fatal(err)
	}
	row := (*body)["upsert_rows"].([]any)[0].(map[string]any)
	if _, ok := row["skill_loads"].(string); !ok {
		t.Fatalf("events must use JSON string: %v", row)
	}
	raw, _ := json.Marshal(row)
	var got trace.BlockRow
	if err := json.Unmarshal(raw, &got); err != nil || len(got.SkillLoads) != 1 || got.SkillLoads[0].Name != "board" {
		t.Fatalf("roundtrip=%+v err=%v", got, err)
	}
}

func TestSkillSessionPersistence(t *testing.T) {
	store := &sessionSourceStore{}
	cl := sourceTestClient(t, store)
	_, err := cl.WriteSessions([]trace.SessionRow{{ID: "session", SessionID: "session", Harness: "codex", End: 42, Skills: trace.StringList{"board", "peers"}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(store.rows["session"])
	var got trace.SessionRow
	if err = json.Unmarshal(raw, &got); err != nil || len(got.Skills) != 2 || got.Skills[0] != "board" {
		t.Fatalf("stored=%s err=%v", raw, err)
	}
	schema := store.writes[0]["schema"].(map[string]any)
	if schema["skills"].(map[string]any)["filterable"] != true {
		t.Fatal("not explicitly filterable on wire")
	}
}
