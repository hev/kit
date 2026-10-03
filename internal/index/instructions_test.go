package index

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/trace"
)

func TestInstructionVersionHistory(t *testing.T) {
	versions := map[string]layer.InstructionVersion{}
	chunks := map[string]layer.Row{}
	writes := 0
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Rows json.RawMessage `json:"upsert_rows"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/query") {
			rows := []layer.InstructionVersion{}
			for _, v := range versions {
				rows = append(rows, v)
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
			json.NewEncoder(w).Encode(map[string]any{"rows": rows})
			return
		}
		if strings.HasSuffix(r.URL.Path, "-instructions") {
			if fail {
				http.Error(w, "retry", 500)
				return
			}
			var rows []layer.InstructionVersion
			json.Unmarshal(body.Rows, &rows)
			for _, v := range rows {
				versions[v.ID] = v
			}
		} else {
			var rows []layer.Row
			json.Unmarshal(body.Rows, &rows)
			for _, v := range rows {
				chunks[v.ID] = v
			}
			writes++
		}
		w.Write([]byte(`{"status":"OK","rows_upserted":1}`))
	}))
	defer server.Close()
	cl := layer.New(server.URL, "fixture", "fixture", "")
	scrubber, err := redact.New([]byte(strings.Repeat("a", 32)))
	if err != nil {
		t.Fatal(err)
	}
	rep := &Report{Redactions: redact.Counts{}}
	secret := "password=" + "SuperSecretFixtureValue123!"
	f := trace.InstructionFile{Path: "/fixture/CLAUDE.md", Project: "/fixture", Text: "policy\n" + secret, Hash: "hash-one", Mtime: "2026-10-02T00:00:00Z"}
	if err := indexInstruction(f, cl, scrubber, rep); err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || len(chunks) != 1 {
		t.Fatalf("versions=%v chunks=%v", versions, chunks)
	}
	for _, c := range chunks {
		if strings.Contains(c.Text, "SuperSecretFixtureValue") || c.Path != f.Path || c.Project != f.Project || c.Host == "" {
			t.Fatalf("unsafe/missing metadata: %+v", c)
		}
	}
	if err := indexInstruction(f, cl, scrubber, rep); err != nil {
		t.Fatal(err)
	}
	if writes != 1 || rep.UnitsSkipped != 1 {
		t.Fatal("unchanged scan wrote")
	}
	f.Hash = "hash-two"
	f.Text = "changed policy"
	fail = true
	if err := indexInstruction(f, cl, scrubber, rep); err == nil {
		t.Fatal("failed metadata publish accepted")
	}
	fail = false
	if err := indexInstruction(f, cl, scrubber, rep); err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || len(chunks) != 2 {
		t.Fatal("retry duplicated versions or removed history")
	}
	current, closed := 0, 0
	for _, v := range versions {
		if v.ValidTo == "" {
			current++
		} else {
			closed++
		}
		if v.ValidFrom == "" || v.Mtime == "" || len(v.ChunkIDs) != 1 {
			t.Fatal("version metadata missing")
		}
	}
	if current != 1 || closed != 1 {
		t.Fatal("validity")
	}
	f.Hash = "hash-one"
	f.Text = "original policy"
	if err := indexInstruction(f, cl, scrubber, rep); err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 {
		t.Fatal("revert overwrote history")
	}
}

// A store that accepts a write and never answers must fail that instruction
// within the client timeout, and the next instruction must still be indexed.
func TestInstructionWriteHangDoesNotStallCycle(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Rows json.RawMessage `json:"upsert_rows"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/query") {
			w.Write([]byte(`{"rows":[]}`))
			return
		}
		if strings.Contains(string(body.Rows), "hang me") {
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		w.Write([]byte(`{"status":"OK","rows_upserted":1}`))
	}))
	defer server.Close()
	defer close(release)
	cl := layer.New(server.URL, "fixture", "fixture", "")
	cl.Timeout = 300 * time.Millisecond
	cl.HTTP = &http.Client{} // only the per-request deadline can fire
	scrubber, err := redact.New([]byte(strings.Repeat("a", 32)))
	if err != nil {
		t.Fatal(err)
	}
	rep := &Report{Redactions: redact.Counts{}}
	files := []trace.InstructionFile{
		{Path: "/fixture/a/CLAUDE.md", Project: "/fixture/a", Text: "hang me", Hash: "h1", Mtime: "2026-10-02T00:00:00Z"},
		{Path: "/fixture/b/CLAUDE.md", Project: "/fixture/b", Text: "fine", Hash: "h2", Mtime: "2026-10-02T00:00:00Z"},
	}
	start := time.Now()
	var errs []error
	for _, f := range files {
		if err := indexInstruction(f, cl, scrubber, rep); err != nil {
			errs = append(errs, err)
		}
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cycle took %v", elapsed)
	}
	if len(errs) != 1 || rep.UnitsIndexed != 1 {
		t.Fatalf("errs=%v indexed=%d", errs, rep.UnitsIndexed)
	}
}
