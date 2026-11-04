package dispatch

import (
	"fmt"
	"sort"
)

// InventoryEvidence is the shared code-built inventory step for follow-through
// agents. An unavailable host fails the snapshot rather than hiding its work.
type InventoryEvidence struct {
	Host string
	Row  Row
}

func SelectInventoryEvidence(inv map[string]Response, include func(Row) bool) ([]InventoryEvidence, error) {
	out := []InventoryEvidence{}
	for host, r := range inv {
		if r.Error != "" {
			return nil, fmt.Errorf("cannot snapshot %s: %s", host, r.Error)
		}
		for _, row := range r.Rows {
			if include(row) {
				out = append(out, InventoryEvidence{host, row})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host+"/"+out[i].Row.Name < out[j].Host+"/"+out[j].Row.Name })
	return out, nil
}
