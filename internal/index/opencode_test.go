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
	"time"

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

// A message OpenCode is still streaming is not archived: its chunk IDs hash
// the text, so a truncated copy would stay in the archive for good, and a key
// cut off before its END line slips past the PEM rule. Once the message
// completes, every chunk ever written is one a fresh capture would write.
func TestOpenCodeArchivesOnlyCompletedMessages(t *testing.T) {
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
	const (
		session = "ses_e48a2f02bffeHbDXxurkNl3k8y"
		body    = "MIIEowIBAAKCAQEAOPENCODESTREAMEDKEYBODY"
	)
	now := time.Now().UnixMilli()
	exec := func(statements ...string) {
		t.Helper()
		for _, s := range statements {
			if _, err := db.Exec(s); err != nil {
				t.Fatal(err)
			}
		}
	}
	exec("PRAGMA journal_mode = WAL", string(fixture),
		fmt.Sprintf(`INSERT INTO message VALUES('msg_zz_ask','%s',%d,%d,'{"role":"user","time":{"created":%d},"model":{"providerID":"opencode","modelID":"big-pickle"}}')`, session, now, now, now),
		fmt.Sprintf(`INSERT INTO part VALUES('prt_zz_ask','msg_zz_ask','%s',%d,%d,'{"type":"text","text":"OPENCODE_STREAM_PROMPT"}')`, session, now, now),
		fmt.Sprintf(`INSERT INTO message VALUES('msg_zz_reply','%s',%d,%d,'{"role":"assistant","parentID":"msg_zz_ask","modelID":"big-pickle","time":{"created":%d},"tokens":{"input":1,"output":1,"reasoning":0,"cache":{"read":0,"write":0}}}')`, session, now+1, now+1, now+1),
		fmt.Sprintf(`INSERT INTO part VALUES('prt_zz_text','msg_zz_reply','%s',%d,%d,'{"type":"text","text":"OPENCODE_STREAM_REPLY\n-----BEGIN RSA PRIVATE KEY-----\n%s"}')`, session, now+1, now+1, body),
		fmt.Sprintf(`INSERT INTO part VALUES('prt_zz_tool','msg_zz_reply','%s',%d,%d,'{"type":"tool","tool":"read","callID":"call_zz","state":{"status":"pending","input":{},"raw":""}}')`, session, now+2, now+2),
	)

	var mu sync.Mutex
	written := map[string]string{}
	srv := httptest.NewServer(withSessionPersistence(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			io.WriteString(w, `{}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		json.Unmarshal(b, &body)
		if strings.HasSuffix(r.URL.Path, "/query") {
			io.WriteString(w, `{"rows":[]}`)
			return
		}
		mu.Lock()
		rows, _ := body["upsert_rows"].([]any)
		for _, row := range rows {
			row := row.(map[string]any)
			if strings.HasSuffix(r.URL.Path, "/namespace") {
				written[fmt.Sprint(row["id"])] = fmt.Sprint(row["text"])
			}
		}
		mu.Unlock()
		io.WriteString(w, `{"status":"OK","rows_upserted":1}`)
	})))
	defer srv.Close()
	cl := layer.New(srv.URL, "key", "namespace", "")
	src := &trace.OpenCodeSource{Root: root}
	opt := Options{Tiers: trace.AllTiers[:], RepoURL: func(string) string { return "" }}
	state := &State{Units: map[string]string{}}
	run := func(state *State) {
		t.Helper()
		rep, err := Run(src, cl, state, opt)
		if err != nil || len(rep.Errors) > 0 {
			t.Fatalf("run: %+v %v", rep, err)
		}
	}
	texts := func() string {
		var all []string
		for _, text := range written {
			all = append(all, text)
		}
		return strings.Join(all, "\n")
	}

	run(state)
	mid := texts()
	if !strings.Contains(mid, "OPENCODE_STREAM_PROMPT") {
		t.Fatal("the prompt before the streaming reply was not archived")
	}
	if strings.Contains(mid, "OPENCODE_STREAM_REPLY") || strings.Contains(mid, body) || strings.Contains(mid, "read {}") {
		t.Fatal("a streaming message reached the archive")
	}
	streamed := map[string]bool{}
	for id := range written {
		streamed[id] = true
	}

	exec(
		fmt.Sprintf(`UPDATE part SET time_updated=%d, data='{"type":"text","text":"OPENCODE_STREAM_REPLY\n-----BEGIN RSA PRIVATE KEY-----\n%s\n-----END RSA PRIVATE KEY-----"}' WHERE id='prt_zz_text'`, now+3, body),
		fmt.Sprintf(`UPDATE part SET time_updated=%d, data='{"type":"tool","tool":"read","callID":"call_zz","state":{"status":"completed","input":{"filePath":"/tmp/ocproj/README.md"},"output":"ok","title":"","metadata":{},"time":{"start":%d,"end":%d}}}' WHERE id='prt_zz_tool'`, now+4, now+3, now+4),
		fmt.Sprintf(`UPDATE message SET time_updated=%d, data=json_set(data,'$.time.completed',%d) WHERE id='msg_zz_reply'`, now+5, now+5),
	)
	run(state)
	final := texts()
	if !strings.Contains(final, "OPENCODE_STREAM_REPLY") || !strings.Contains(final, `read {"filePath"`) || strings.Contains(final, body) {
		t.Fatalf("completed reply archived wrongly:\n%s", final)
	}

	// Every chunk written along the way is one a from-scratch capture writes.
	ever := map[string]bool{}
	for id := range written {
		ever[id] = true
	}
	for id := range written {
		delete(written, id)
	}
	run(&State{Units: map[string]string{}})
	for id := range ever {
		if _, ok := written[id]; !ok {
			t.Errorf("orphaned chunk %s (streamed=%v)", id, streamed[id])
		}
	}
}
