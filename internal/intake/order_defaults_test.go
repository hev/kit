package intake

import "testing"

func TestOrderLeavesRoleDefaultsForExecutingHost(t *testing.T) {
	o, err := parseOrder("---\nname: defaults\nrepo: app\n---\n## Acceptance\n- Done\n")
	if err != nil || o.Harness != "" || o.Model != "" {
		t.Fatalf("%+v %v", o, err)
	}
}
