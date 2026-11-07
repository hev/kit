package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
	_ "modernc.org/sqlite"
)

// OpenCode capture keeps every archive guarantee: secrets are scrubbed before
// any write, unchanged sessions are skipped, and a session that grows is
// re-upserted under the same chunk IDs plus the new turn's, never duplicated.
func TestOpenCodeCaptureScrubsAndIndexesIncrementally(t *testing.T) {
	fixture, err := os.ReadFile("../trace/testdata/opencode/opencode-1.19.5.sql")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(root, "opencode.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	secret, _ := json.Marshal(fixtureText())
	for _, s := range []string{"PRAGMA journal_mode = WAL", string(fixture),
		`UPDATE part SET data=json_set(data,'$.state.output',json('` + strings.ReplaceAll(string(secret), "'", "''") + `')) WHERE id='prt_1b75d1eb60010crOevkGbcUWHK'`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	chunkIDs := map[string]int{}
	harnesses := map[string]bool{}
	srv := httptest.NewServer(withSessionPersistence(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			io.WriteString(w, `{}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(b, &body); err != nil {
			t.Error(err)
		}
		assertClean(t, stringValues(body))
		if strings.HasSuffix(r.URL.Path, "/query") {
			io.WriteString(w, `{"rows":[]}`)
			return
		}
		mu.Lock()
		rows, _ := body["upsert_rows"].([]any)
		for _, row := range rows {
			row := row.(map[string]any)
			if strings.HasSuffix(r.URL.Path, "/namespace") {
				chunkIDs[fmt.Sprint(row["id"])]++
			}
			if h, ok := row["harness"].(string); ok {
				harnesses[h] = true
			}
		}
		mu.Unlock()
		io.WriteString(w, `{"status":"OK","rows_upserted":1}`)
	})))
	defer srv.Close()
	cl := layer.New(srv.URL, "key", "namespace", "")
	state := &State{Units: map[string]string{}}
	src := &trace.OpenCodeSource{Root: root}
	opt := Options{Tiers: trace.AllTiers[:], RepoURL: func(string) string { return "" }}

	first, err := Run(src, cl, state, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Errors) > 0 || first.UnitsIndexed != 5 || len(first.Redactions) < 7 {
		t.Fatalf("first report %+v", first)
	}
	if len(harnesses) != 1 || !harnesses["opencode"] {
		t.Fatalf("harness labels %v", harnesses)
	}
	firstIDs := map[string]bool{}
	for id := range chunkIDs {
		firstIDs[id] = true
	}

	for _, s := range []string{
		`INSERT INTO message VALUES('msg_zz_more','ses_e48a2f02bffeHbDXxurkNl3k8y',1794078000000,1794078000000,'{"role":"user","time":{"created":1794078000000},"model":{"providerID":"opencode","modelID":"big-pickle"}}')`,
		`INSERT INTO part VALUES('prt_zz_more','msg_zz_more','ses_e48a2f02bffeHbDXxurkNl3k8y',1794078000001,1794078000001,'{"type":"text","text":"OPENCODE_CAPTURE_INCREMENTAL"}')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	for id := range chunkIDs {
		delete(chunkIDs, id)
	}
	second, err := Run(src, cl, state, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Errors) > 0 || second.UnitsIndexed != 1 || second.UnitsSkipped != 4 {
		t.Fatalf("second report %+v", second)
	}
	var added int
	for id, n := range chunkIDs {
		if n != 1 {
			t.Fatalf("chunk %s written %d times in one pass", id, n)
		}
		if !firstIDs[id] {
			added++
		}
	}
	if added != 1 {
		t.Fatalf("re-scan added %d new chunk IDs, want only the new turn's", added)
	}
	third, err := Run(src, cl, state, opt)
	if err != nil || third.UnitsIndexed != 0 {
		t.Fatalf("unchanged re-scan %+v %v", third, err)
	}
}
