package layer

import "testing"

func TestSkillsSchema(t *testing.T) {
	for _, arrays := range []bool{false, true} {
		s := sessionSchema(arrays)
		if s["skills"].(map[string]any)["filterable"] != true {
			t.Fatal("skills must be filterable")
		}
	}
	if blockSchema()["skill_loads"].(map[string]any)["filterable"] != false {
		t.Fatal("event JSON must be unfilterable")
	}
}
