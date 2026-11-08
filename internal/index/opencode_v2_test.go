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
)

type openCodeChunk struct{ session, text string }

// openCodeArchive captures root into a fake archive and returns the trace
// chunks written, keyed by chunk ID.
func openCodeArchive(t *testing.T, root string, state *State) map[string]openCodeChunk {
	t.Helper()
	rep, written, err := openCodeCapture(t, root, state)
	if err != nil || len(rep.Errors) > 0 {
		t.Fatalf("run: %+v %v", rep, err)
	}
	return written
}

func openCodeCapture(t *testing.T, root string, state *State) (*Report, map[string]openCodeChunk, error) {
	t.Helper()
	var mu sync.Mutex
	written := map[string]openCodeChunk{}
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
				written[fmt.Sprint(row["id"])] = openCodeChunk{fmt.Sprint(row["session_id"]), fmt.Sprint(row["text"])}
			}
		}
		mu.Unlock()
		io.WriteString(w, `{"status":"OK","rows_upserted":1}`)
	})))
	defer srv.Close()
	rep, err := Run(&trace.OpenCodeSource{Root: root}, layer.New(srv.URL, "key", "namespace", ""), state, Options{Tiers: trace.AllTiers[:], RepoURL: func(string) string { return "" }})
	return rep, written, err
}

func openCodeFixtureDB(t *testing.T, root, fixture string, statements ...string) *sql.DB {
	t.Helper()
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "opencode.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	for _, s := range append([]string{"PRAGMA journal_mode = WAL", string(data)}, statements...) {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// A database whose tables match neither layout (what a future OpenCode storage
// change looks like) is a capture error every cycle, so it shows in the
// daemon's last error, while the other databases are still archived.
func TestOpenCodeUnknownLayoutReported(t *testing.T) {
	root := t.TempDir()
	openCodeFixtureDB(t, root, "../trace/testdata/opencode/opencode-2.2.6.sql")
	db, err := sql.Open("sqlite", filepath.Join(root, "opencode-future.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE session_v3(id TEXT)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	state := &State{Units: map[string]string{}}
	rep, written, err := openCodeCapture(t, root, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Errors) != 1 || !strings.Contains(rep.Errors[0], "opencode-future.db") || !strings.Contains(rep.Errors[0], "no OpenCode session layout Kit recognises") {
		t.Fatalf("errors=%q", rep.Errors)
	}
	if len(written) == 0 || rep.UnitsIndexed == 0 {
		t.Fatalf("good database not archived: %+v", rep)
	}
	again, _, err := openCodeCapture(t, root, state)
	if err != nil || len(again.Errors) != 1 {
		t.Fatalf("not reported again: %+v %v", again, err)
	}
}

// Upgrading OpenCode 1.x to 2.x copies every session into the v2 tables. The
// archive must not gain a second copy of what 1.x capture already wrote: every
// chunk a fresh capture of the upgraded store writes for those sessions is
// either one 1.x capture wrote or comes from a turn taken after the upgrade.
func TestOpenCodeUpgradeArchivesNoDuplicates(t *testing.T) {
	v1Root, v2Root := t.TempDir(), t.TempDir()
	openCodeFixtureDB(t, v1Root, "../trace/testdata/opencode/opencode-1.19.5.sql")
	openCodeFixtureDB(t, v2Root, "../trace/testdata/opencode/opencode-2.2.6.sql")
	before := openCodeArchive(t, v1Root, &State{Units: map[string]string{}})
	after := openCodeArchive(t, v2Root, &State{Units: map[string]string{}})
	sessions := map[string]bool{}
	for id, c := range before {
		sessions[c.session] = true
		if _, ok := after[id]; !ok {
			t.Errorf("upgrade orphaned chunk %s of %s: %.60q", id, c.session, c.text)
		}
	}
	if len(sessions) != 5 {
		t.Fatalf("v1 sessions=%d", len(sessions))
	}
	var added []string
	for id, c := range after {
		if _, ok := before[id]; ok || !sessions[c.session] {
			continue
		}
		if c.session != "ses_e48a2f02bffeHbDXxurkNl3k8y" || strings.Contains(c.text, "available tools have changed") {
			t.Errorf("upgrade added chunk to %s: %.80q", c.session, c.text)
		}
		added = append(added, c.text)
	}
	if all := strings.Join(added, "\n"); !strings.Contains(all, "continued-in-2x") {
		t.Fatalf("the turn taken after the upgrade was not archived: %q", all)
	}
}

// The v2 counterpart of TestOpenCodeArchivesOnlyCompletedMessages: a 2.x
// assistant message is archived only once it has time.completed.
func TestOpenCodeV2ArchivesOnlyCompletedMessages(t *testing.T) {
	root := t.TempDir()
	const (
		session = "ses_e47ae7268ffeU0nY0ozzLJ5ThP"
		body    = "MIIEowIBAAKCAQEAOPENCODEV2STREAMEDKEYBODY"
	)
	now := time.Now().UnixMilli()
	db := openCodeFixtureDB(t, root, "../trace/testdata/opencode/opencode-2.2.6.sql",
		fmt.Sprintf(`INSERT INTO session_message VALUES('msg_zz_ask','%s','user',100,%d,%d,'{"time":{"created":%d},"text":"OPENCODE_V2_STREAM_PROMPT","files":[]}')`, session, now, now, now),
		fmt.Sprintf(`INSERT INTO session_message VALUES('msg_zz_reply','%s','assistant',101,%d,%d,'{"time":{"created":%d},"agent":"build","model":{"id":"big-pickle","providerID":"opencode"},"content":[{"type":"text","text":"OPENCODE_V2_STREAM_REPLY\n-----BEGIN RSA PRIVATE KEY-----\n%s"},{"type":"tool","id":"call_zz","name":"read","state":{"status":"streaming","input":""},"time":{"created":%d}}]}')`, session, now+1, now+1, now+1, body, now+1),
	)
	state := &State{Units: map[string]string{}}
	written := openCodeArchive(t, root, state)
	var mid []string
	for _, c := range written {
		mid = append(mid, c.text)
	}
	if all := strings.Join(mid, "\n"); !strings.Contains(all, "OPENCODE_V2_STREAM_PROMPT") || strings.Contains(all, "OPENCODE_V2_STREAM_REPLY") || strings.Contains(all, body) || strings.Contains(all, "read {}") {
		t.Fatal("a streaming 2.x message reached the archive, or the prompt before it did not")
	}

	if _, err := db.Exec(fmt.Sprintf(`UPDATE session_message SET time_updated=%d, data=json_set(data,'$.content[0].text',json_extract(data,'$.content[0].text') || char(10) || '-----END RSA PRIVATE KEY-----','$.content[1].state',json('{"status":"completed","input":{"path":"/tmp/ocproj/README.md"},"content":[{"type":"text","text":"ok"}]}'),'$.time.completed',%d) WHERE id='msg_zz_reply'`, now+3, now+3)); err != nil {
		t.Fatal(err)
	}
	more := openCodeArchive(t, root, state)
	var final []string
	for id, c := range more {
		written[id] = c
	}
	for _, c := range written {
		final = append(final, c.text)
	}
	if all := strings.Join(final, "\n"); !strings.Contains(all, "OPENCODE_V2_STREAM_REPLY") || !strings.Contains(all, `read {"path"`) || strings.Contains(all, body) {
		t.Fatalf("completed reply archived wrongly")
	}
	fresh := openCodeArchive(t, root, &State{Units: map[string]string{}})
	for id := range written {
		if _, ok := fresh[id]; !ok {
			t.Errorf("orphaned chunk %s", id)
		}
	}
}
