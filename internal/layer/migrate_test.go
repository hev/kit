package layer

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

// fakeSessions is a sessions namespace as the gateway keeps it: a schema, the
// rows, and a write that fails where the schema would change type.
type fakeSessions struct {
	schema  string
	rows    []string
	deleted bool
	writes  []string
	failAt  int
	version string
}

func (f *fakeSessions) serve(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch {
		case r.URL.Path == "/health":
			v := f.version
			if v == "" {
				v = "0.7.1"
			}
			io.WriteString(w, `{"status":"ok","version":"`+v+`"}`)
		case r.Method == "GET" && r.URL.Path == "/v1/namespaces/ns-sessions/schema":
			if f.schema == "" {
				w.WriteHeader(404)
				io.WriteString(w, `{"error":"not_found"}`)
				return
			}
			io.WriteString(w, f.schema)
		case r.Method == "POST" && r.URL.Path == "/v2/namespaces/ns-sessions/query":
			io.WriteString(w, `{"rows":[`+strings.Join(f.rows, ",")+`]}`)
		case r.Method == "DELETE" && r.URL.Path == "/v2/namespaces/ns-sessions":
			f.deleted, f.schema, f.rows = true, "", nil
			io.WriteString(w, `{"status":"OK"}`)
		case r.Method == "POST" && r.URL.Path == "/v2/namespaces/ns-sessions":
			f.writes = append(f.writes, string(raw))
			if len(f.writes) == f.failAt {
				w.WriteHeader(500)
				io.WriteString(w, `{"error":"boom"}`)
				return
			}
			f.schema = `{"tool_names":{"type":"[]string"},"prompt_ts":{"type":"[]uint"}}`
			io.WriteString(w, `{"status":"OK","rows_upserted":1}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

const legacySessionSchema = `{"start":{"type":"int"},"tool_names":{"filterable":false,"type":"string"},"prompt_ts":{"filterable":false,"type":"string"}}`

// The namespace kit v0.3.0 wrote on Postgres: its rows are read back from the
// string form, the namespace is deleted, and the rows are written again as
// arrays, whole, with nothing re-parsed.
func TestMigrateSessionListsRewritesAStringTypedNamespace(t *testing.T) {
	f := &fakeSessions{schema: legacySessionSchema, rows: []string{
		`{"id":"s1","session_id":"s1","summary":"kept","start":5,"end":9,"prompt_ts":"[1,2]","tool_names":"[\"Bash\",\"Read\"]","tool_counts":"{\"Bash\":2}"}`,
	}}
	cl, _ := New(f.serve(t).URL, "local", "ns", "").WithStore(StorePgvector)
	backup := filepath.Join(t.TempDir(), "sessions-migration.json")
	n, err := cl.MigrateSessionLists(backup)
	if err != nil || n != 1 {
		t.Fatalf("n = %d, err = %v", n, err)
	}
	if !f.deleted || len(f.writes) != 1 {
		t.Fatalf("deleted %v, writes %d", f.deleted, len(f.writes))
	}
	for _, want := range []string{`"tool_names":["Bash","Read"]`, `"prompt_ts":[1,2]`, `"tool_names":{"type":"[]string"}`, `"summary":"kept"`, `"tool_counts":"{\"Bash\":2}"`} {
		if !strings.Contains(f.writes[0], want) {
			t.Fatalf("write lacks %s:\n%s", want, f.writes[0])
		}
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("backup left behind: %v", err)
	}
	// Run again, it finds the array schema and does nothing.
	if n, err := cl.MigrateSessionLists(backup); err != nil || n != 0 || len(f.writes) != 1 {
		t.Fatalf("second run: n = %d, err = %v, writes %d", n, err, len(f.writes))
	}
}

// A migration that fails after the delete keeps the rows on disk, and the next
// run writes them from there.
func TestMigrateSessionListsResumesFromBackup(t *testing.T) {
	f := &fakeSessions{schema: legacySessionSchema, failAt: 1, rows: []string{
		`{"id":"s1","start":5,"end":9,"tool_names":"[\"Edit\"]"}`,
	}}
	cl, _ := New(f.serve(t).URL, "local", "ns", "").WithStore(StorePgvector)
	backup := filepath.Join(t.TempDir(), "sessions-migration.json")
	if _, err := cl.MigrateSessionLists(backup); err == nil || !strings.Contains(err.Error(), backup) {
		t.Fatalf("err = %v", err)
	}
	var kept []trace.SessionRow
	if raw, err := os.ReadFile(backup); err != nil || json.Unmarshal(raw, &kept) != nil || len(kept) != 1 {
		t.Fatalf("backup %v %v", kept, err)
	}
	n, err := cl.MigrateSessionLists(backup)
	if err != nil || n != 1 || !strings.Contains(f.writes[1], `"tool_names":["Edit"]`) {
		t.Fatalf("n = %d, err = %v, writes %v", n, err, f.writes)
	}
}

// An array-typed namespace, or none at all, is left alone, and so is every
// namespace on a store without arrays.
func TestMigrateSessionListsLeavesOtherNamespacesAlone(t *testing.T) {
	for _, schema := range []string{"", `{"tool_names":{"type":"[]string"},"prompt_ts":{"type":"[]uint"}}`} {
		f := &fakeSessions{schema: schema}
		cl, _ := New(f.serve(t).URL, "local", "ns", "").WithStore(StorePgvector)
		if n, err := cl.MigrateSessionLists(filepath.Join(t.TempDir(), "b.json")); err != nil || n != 0 || f.deleted || len(f.writes) != 0 {
			t.Fatalf("schema %q: n = %d, err = %v, deleted %v", schema, n, err, f.deleted)
		}
	}
	f := &fakeSessions{schema: legacySessionSchema}
	cl, _ := New(f.serve(t).URL, "local", "ns", "").WithStore(StorePgvector)
	cl.Caps.ArrayAttributes = Unsupported
	if n, err := cl.MigrateSessionLists(filepath.Join(t.TempDir(), "b.json")); err != nil || n != 0 || f.deleted {
		t.Fatalf("no arrays: n = %d, err = %v, deleted %v", n, err, f.deleted)
	}
}

// A gateway that does not serve arrays on Postgres would take the delete and
// refuse the rewrite, so a namespace behind one is left as it is. The edge
// mirror's -dev versions count as their release.
func TestMigrateSessionListsWaitsForAGatewayWithArrays(t *testing.T) {
	for version, want := range map[string]bool{"0.7.0": false, "0.6.2": false, "dev": false, "0.7.1-dev": true, "0.7.1": true, "0.8.0": true, "1.0.0": true} {
		f := &fakeSessions{schema: legacySessionSchema, version: version}
		cl, _ := New(f.serve(t).URL, "local", "ns", "").WithStore(StorePgvector)
		if _, err := cl.MigrateSessionLists(filepath.Join(t.TempDir(), "b.json")); err != nil || f.deleted != want {
			t.Fatalf("%s: deleted %v, err %v", version, f.deleted, err)
		}
	}
}
