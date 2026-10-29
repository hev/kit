package layer

import (
	"encoding/json"
	"github.com/hev/kit/internal/trace"
	"reflect"
	"testing"
)

func TestSkillAttributes(t *testing.T) {
	for _, arrays := range []bool{true, false} {
		def := sessionSchema(arrays)["skills"].(map[string]any)
		if def["filterable"] != true {
			t.Fatal("skills must be explicitly filterable")
		}
		want := "[]string"
		if !arrays {
			want = "string"
		}
		if def["type"] != want {
			t.Fatalf("skills type %v", def)
		}
	}
	if blockSchema()["skill_loads"].(map[string]any)["filterable"] != false {
		t.Fatal("load evidence must not be filterable")
	}
	cl, got := capture(t, `{"rows_upserted":1}`)
	loads := trace.SkillLoads{{Name: "peers", Path: "/repo/skills/peers/SKILL.md", Source: "/repo/skills", TurnUUID: "t", Seq: 2, TS: "2026-10-29T10:00:00Z"}}
	if _, err := cl.WriteBlocks([]trace.BlockRow{{ID: "b", SkillLoads: loads}}); err != nil {
		t.Fatal(err)
	}
	obj := (*got)["upsert_rows"].([]any)[0].(map[string]any)
	raw, ok := obj["skill_loads"].(string)
	if !ok {
		t.Fatal("block evidence must be JSON encoded on wire")
	}
	var decoded trace.SkillLoads
	quoted, _ := json.Marshal(raw)
	if err := json.Unmarshal(quoted, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, loads) {
		t.Fatalf("round trip: %+v", decoded)
	}
	if _, err := cl.WriteSessions([]trace.SessionRow{{ID: "s", Skills: trace.StringList{"peers"}}}); err != nil {
		t.Fatal(err)
	}
	def := (*got)["schema"].(map[string]any)["skills"].(map[string]any)
	if def["filterable"] != true {
		t.Fatalf("wire schema %v", def)
	}
}
