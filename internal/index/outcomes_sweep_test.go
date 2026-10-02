package index

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSweepCursorRetriesAndIdempotence(t *testing.T) {
	g, fixture, _ := fixtureOutcome(t, mergedTime.Add(15*24*time.Hour), "mixed")
	rows := map[string]trace.SessionRow{}
	for _, id := range []string{"a", "b", "c"} {
		r := *fixture
		r.ID = id
		r.End = mergedTime.UnixMilli()
		rows[id] = r
	}
	writes := 0
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `{}`)
			return
		}
		var b map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&b)
		if strings.HasSuffix(r.URL.Path, "/query") {
			var filters any
			json.Unmarshal(b["filters"], &filters)
			if strings.Contains(fmt.Sprint(filters), "In") {
				var selected []trace.SessionRow
				for _, id := range []string{"a", "b", "c"} {
					if strings.Contains(fmt.Sprint(filters), "["+id+"]") {
						selected = append(selected, rows[id])
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"rows": selected})
				return
			}
			var selected []trace.SessionRow
			for _, id := range []string{"a", "b", "c"} {
				f := fmt.Sprint(filters)
				if strings.Contains(f, "Gt b") && id <= "b" || strings.Contains(f, "Gt a") && id <= "a" {
					continue
				}
				selected = append(selected, rows[id])
				if len(selected) == 2 {
					break
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"rows": selected})
			return
		}
		if fail {
			http.Error(w, "retry", 503)
			return
		}
		var patches []map[string]json.RawMessage
		json.Unmarshal(b["patch_rows"], &patches)
		for _, patch := range patches {
			var id string
			json.Unmarshal(patch["id"], &id)
			raw, _ := json.Marshal(rows[id])
			var all map[string]json.RawMessage
			json.Unmarshal(raw, &all)
			for k, v := range patch {
				all[k] = v
			}
			raw, _ = json.Marshal(all)
			var next trace.SessionRow
			json.Unmarshal(raw, &next)
			rows[id] = next
			writes++
		}
		fmt.Fprint(w, `{"status":"OK"}`)
	}))
	defer srv.Close()
	cl := layer.New(srv.URL, "", "ns", "")
	next, n, e := SweepOutcomes(context.Background(), cl, g, "", 0, 2)
	if e != nil || next != "b" || n != 2 {
		t.Fatalf("%s %d %v", next, n, e)
	}
	next, n, e = SweepOutcomes(context.Background(), cl, g, next, 0, 2)
	if e != nil || next != "" || n != 1 {
		t.Fatalf("%s %d %v", next, n, e)
	}
	_, n, e = SweepOutcomes(context.Background(), cl, g, "", 0, 2)
	if e != nil || n != 0 || writes != 3 {
		t.Fatalf("repeat %d writes %d %v", n, writes, e)
	}
	fail = true
	g.Now = func() time.Time { return mergedTime.Add(10 * 24 * time.Hour) }
	next, n, e = SweepOutcomes(context.Background(), cl, g, "", 0, 2)
	if e == nil || next != "" || n != 0 {
		t.Fatalf("failed patch advanced cursor: %s %d %v", next, n, e)
	}
	fail = false
	next, n, e = SweepOutcomes(context.Background(), cl, g, next, 0, 2)
	if e != nil || next != "b" || n != 2 {
		t.Fatalf("retry %s %d %v", next, n, e)
	}
}
