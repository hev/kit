package layer

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hev/kit/internal/trace"
)

type sessionSourceStore struct {
	mu            sync.Mutex
	rows          map[string]map[string]any
	schema        map[string]any
	afterRead     func(*sessionSourceStore)
	beforeWrite   func(*sessionSourceStore)
	writes        []map[string]any
	zero          bool
	badReadback   bool
	normalizeCost bool
}

func sourceCondition(cond any, row map[string]any) bool {
	a := cond.([]any)
	if a[0] == "And" {
		for _, c := range a[1].([]any) {
			if !sourceCondition(c, row) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(row[a[0].(string)], a[2])
}
func (s *sessionSourceStore) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method == "GET" {
		json.NewEncoder(w).Encode(s.schema)
		return
	}
	var b map[string]any
	json.NewDecoder(r.Body).Decode(&b)
	if strings.HasSuffix(r.URL.Path, "/query") {
		ids := b["filters"].([]any)[2].([]any)
		rows := []any{}
		for _, id := range ids {
			if row := s.rows[id.(string)]; row != nil {
				rows = append(rows, row)
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"rows": rows})
		if s.afterRead != nil {
			f := s.afterRead
			s.afterRead = nil
			f(s)
		}
		return
	}
	s.writes = append(s.writes, b)
	if s.beforeWrite != nil {
		f := s.beforeWrite
		s.beforeWrite = nil
		f(s)
	}
	key, condition := "patch_rows", "patch_condition"
	if b[key] == nil {
		key, condition = "upsert_rows", "upsert_condition"
	}
	row := b[key].([]any)[0].(map[string]any)
	id := row["id"].(string)
	current := s.rows[id]
	if s.zero || !sourceCondition(b[condition], current) {
		fmt.Fprint(w, `{"rows_affected":0}`)
		return
	}
	if key == "upsert_rows" {
		s.rows[id] = row
	} else {
		for k, v := range row {
			current[k] = v
		}
	}
	if s.normalizeCost {
		s.rows[id]["cost"] = 102.83891100000004
	}
	if s.badReadback {
		s.rows[id]["tool_count"] = float64(999)
	}
	json.NewEncoder(w).Encode(map[string]any{"rows_affected": 1, "rows_upserted": 1, "patched_ids": []string{id}})
}
func sourceTestClient(t *testing.T, s *sessionSourceStore) *Client {
	t.Helper()
	if s.rows == nil {
		s.rows = map[string]map[string]any{}
	}
	if s.schema == nil {
		s.schema = map[string]any{}
	}
	srv := httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(srv.Close)
	return New(srv.URL, "", "ns", "")
}
func TestSessionSourceInterleavingPreservesCurrentEnrichment(t *testing.T) {
	for _, changedLinkage := range []bool{false, true} {
		t.Run(fmt.Sprint(changedLinkage), func(t *testing.T) {
			sha := strings.Repeat("a", 40)
			other := strings.Repeat("b", 40)
			s := &sessionSourceStore{rows: map[string]map[string]any{"s": {"id": "s", "session_id": "session", "end": float64(10), "commits": "[]", "summary": "old", "custom": "keep"}}, schema: map[string]any{"commits": map[string]any{"type": "string", "filterable": true}, "summary": map[string]any{"type": "string", "filterable": false}}}
			s.afterRead = func(s *sessionSourceStore) {
				r := s.rows["s"]
				r["outcome_checked"] = float64(20)
				r["ci_state"] = "complete"
				r["summary"] = "newer summary"
				r["outcome_details"] = `{"ci_workflow_test_conclusion":"success"}`
				if changedLinkage {
					r["pr"] = "42"
					r["commits"] = `["` + other + `"]`
				}
			}
			cl := sourceTestClient(t, s)
			res, err := cl.WriteSessions([]trace.SessionRow{{ID: "s", SessionID: "session", End: 11, ToolCount: 5, Commits: trace.StringList{sha}, Summary: "stale summary"}})
			if err != nil || res.RowsUpserted != 1 {
				t.Fatalf("result=%+v err=%v", res, err)
			}
			r := s.rows["s"]
			if r["summary"] != "newer summary" || r["ci_state"] != "complete" || r["custom"] != "keep" || r["end"] != float64(11) {
				t.Fatal(r)
			}
			var commits trace.StringList
			b, _ := json.Marshal(r["commits"])
			json.Unmarshal(b, &commits)
			want := trace.StringList{sha}
			if changedLinkage {
				want = append(want, other)
				if r["pr"] != "42" || len(s.writes) != 2 {
					t.Fatal(r, s.writes)
				}
			}
			if !reflect.DeepEqual(commits, want) {
				t.Fatal(commits)
			}
			for _, w := range s.writes {
				p := w["patch_rows"].([]any)[0].(map[string]any)
				for _, k := range []string{"summary", "ci_state", "outcome_checked", "outcome_details"} {
					if _, ok := p[k]; ok {
						t.Fatal("replayed independently owned", k)
					}
				}
			}
		})
	}
}
func TestSessionSourceAdvanceAndAbsentIDRace(t *testing.T) {
	for _, absent := range []bool{false, true} {
		t.Run(fmt.Sprint(absent), func(t *testing.T) {
			s := &sessionSourceStore{rows: map[string]map[string]any{}}
			if !absent {
				s.rows["s"] = map[string]any{"id": "s", "end": float64(10), "summary": "keep"}
			}
			s.beforeWrite = func(s *sessionSourceStore) {
				s.rows["s"] = map[string]any{"id": "s", "end": float64(12), "summary": "raced summary", "ci_state": "pending"}
			}
			cl := sourceTestClient(t, s)
			res, err := cl.WriteSessions([]trace.SessionRow{{ID: "s", End: 11, Summary: "stale"}})
			if err != nil || res.RowsUpserted != 0 || len(s.writes) != 1 || s.rows["s"]["end"] != float64(12) || s.rows["s"]["summary"] != "raced summary" {
				t.Fatal(res, err, s.rows)
			}
			if absent && fmt.Sprint(s.writes[0]["upsert_condition"]) != "[id Eq <nil>]" {
				t.Fatal(s.writes)
			}
		})
	}
	// A concurrent creator with an older source is subsequently patched, never replaced.
	s := &sessionSourceStore{}
	s.beforeWrite = func(s *sessionSourceStore) {
		s.rows["s"] = map[string]any{"id": "s", "end": float64(9), "summary": "created elsewhere", "ci_state": "complete"}
	}
	cl := sourceTestClient(t, s)
	res, err := cl.WriteSessions([]trace.SessionRow{{ID: "s", End: 11}})
	if err != nil || res.RowsUpserted != 1 || len(s.writes) != 2 || s.rows["s"]["summary"] != "created elsewhere" || s.rows["s"]["ci_state"] != "complete" {
		t.Fatal(res, err, s.rows)
	}
}
func TestSessionSourceConflictAndReadbackFailures(t *testing.T) {
	for _, badReadback := range []bool{false, true} {
		t.Run(fmt.Sprint(badReadback), func(t *testing.T) {
			s := &sessionSourceStore{zero: !badReadback, badReadback: badReadback}
			cl := sourceTestClient(t, s)
			res, err := cl.WriteSessions([]trace.SessionRow{{ID: "s", End: 10}})
			if err == nil || res.RowsUpserted != 0 {
				t.Fatal(res, err)
			}
			want := 3
			if badReadback {
				want = 1
			}
			if len(s.writes) != want {
				t.Fatal(len(s.writes))
			}
		})
	}
}

// Wire-only tests still need real session persistence for mandatory readbacks.
// Other namespace requests retain their canned response.
func capturedSessionReply(w http.ResponseWriter, r *http.Request, body map[string]any, rows map[string]map[string]any) bool {
	if !strings.Contains(r.URL.Path, "-sessions") {
		return false
	}
	if strings.HasSuffix(r.URL.Path, "/query") {
		f, ok := body["filters"].([]any)
		if !ok || len(f) != 3 || f[0] != "id" || f[1] != "In" {
			return false
		}
		out := []any{}
		for _, id := range f[2].([]any) {
			if row := rows[id.(string)]; row != nil {
				out = append(out, row)
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"rows": out})
		return true
	}
	key := "upsert_rows"
	if body[key] == nil {
		key = "patch_rows"
	}
	if body[key] == nil {
		return false
	}
	for _, raw := range body[key].([]any) {
		row := raw.(map[string]any)
		id := row["id"].(string)
		if key == "upsert_rows" {
			rows[id] = row
		} else {
			for k, v := range row {
				rows[id][k] = v
			}
		}
	}
	fmt.Fprint(w, `{"status":"OK","rows_affected":1,"rows_upserted":1}`)
	return true
}

func TestSessionSourceLifecycleAndSchemaRemainCompatible(t *testing.T) {
	s := &sessionSourceStore{rows: map[string]map[string]any{"s": {"id": "s", "session_id": "session", "start": float64(1), "end": float64(10), "summary": "independent", "outcome_details": `{"ci_workflow_test_conclusion":"success"}`, "custom": "unknown"}}, schema: map[string]any{"prompt_ts": map[string]any{"type": "string", "filterable": false}, "tool_names": map[string]any{"type": "string", "filterable": false}, "repo_url": map[string]any{"type": "string", "filterable": true, "full_text_search": true}}}
	s.afterRead = func(s *sessionSourceStore) { s.rows["s"]["end"] = float64(11); s.rows["s"]["tool_count"] = float64(2) }
	cl := sourceTestClient(t, s)
	_, err := cl.WriteSessions([]trace.SessionRow{{ID: "s", SessionID: "session", Start: 1, End: 12, Harness: "codex", Host: "host", ToolCount: 3, RepoURL: "url", WallMS: 11, InputTokens: 44, PromptTS: trace.UintList{1, 2}, ToolNames: trace.StringList{"Bash"}, ToolCounts: trace.ToolCounts{"Bash": 3}, FirstPrompt: "source prompt", Summary: "stale"}})
	if err != nil {
		t.Fatal(err)
	}
	r := s.rows["s"]
	if r["end"] != float64(12) || r["tool_count"] != float64(3) || r["wall_ms"] != float64(11) || r["input_tokens"] != float64(44) || r["prompt_ts"] != "[1,2]" || r["first_prompt"] != "source prompt" || r["summary"] != "independent" || r["outcome_details"] != `{"ci_workflow_test_conclusion":"success"}` || r["custom"] != "unknown" || len(s.writes) != 2 {
		t.Fatal(r, s.writes)
	}
	for _, w := range s.writes {
		schema := w["schema"].(map[string]any)
		for k, v := range s.schema {
			if !reflect.DeepEqual(schema[k], v) {
				t.Fatal("changed schema settings", k, schema[k])
			}
		}
	}
}

func TestSessionSourceScalarAcknowledgmentNormalization(t *testing.T) {
	for _, tc := range []struct {
		want, got, typ string
		equal          bool
	}{
		{"102.83891100000005", "102.83891100000004", "float", true},
		{"1", "1.0", "uint", true},
		{"9007199254740993", "9007199254740992", "uint", false},
		{"102.83891100000005", "102.83891100000004", "uint", false},
		{"1", "1.01", "float", false},
	} {
		if got := sessionSourceScalarEqual(json.Number(tc.want), json.Number(tc.got), tc.typ); got != tc.equal {
			t.Fatalf("%+v got=%v", tc, got)
		}
	}
	x := 102.83891100000005
	two := math.Nextafter(math.Nextafter(x, math.Inf(1)), math.Inf(1))
	if sessionSourceScalarEqual(json.Number(strconv.FormatFloat(x, 'g', -1, 64)), json.Number(strconv.FormatFloat(two, 'g', -1, 64)), "float") {
		t.Fatal("accepted more than one observed float step")
	}
	if sessionSourceScalarEqual(json.Number("1"), "1", "float") {
		t.Fatal("accepted wrong numeric type")
	}
}
func TestSessionSourceAcknowledgesActualFloatRepresentation(t *testing.T) {
	s := &sessionSourceStore{schema: map[string]any{"cost": map[string]any{"type": "float", "filterable": true}}, normalizeCost: true}
	cl := sourceTestClient(t, s)
	result, err := cl.WriteSessions([]trace.SessionRow{{ID: "s", SessionID: "session", End: 1, Cost: 102.83891100000005}})
	if err != nil || result.RowsUpserted != 1 {
		t.Fatalf("%+v %v", result, err)
	}
}
