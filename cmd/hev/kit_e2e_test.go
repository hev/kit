package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/board/cli"
	"github.com/hev/kit/internal/board/settings"
	boardstore "github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/pkg/kitapi"
	"github.com/hev/kit/pkg/kitapi/kitapitest"
)

var e2eIdentity = kitapitest.Identity{KeyID: "key_42", AuthorID: "user_9", AuthorName: "Ada", Host: "box", TeamID: "team_1", TeamSlug: "acme"}

// kitService is the sign-in half of a kit service (exchange and logout),
// handing out a key the fake kit API knows.
type kitService struct {
	srv       *httptest.Server
	mu        sync.Mutex
	challenge string
	logouts   int
}

func newKitService(t *testing.T, api *kitapitest.Server) *kitService {
	s := &kitService{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /cli/exchange", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Code, Verifier, Hostname string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Code != "code-1" || body.Verifier == "" {
			http.Error(w, "bad exchange", 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"api": api.URL, "key": fakeKitKey, "key_id": e2eIdentity.KeyID,
			"team":   map[string]string{"id": e2eIdentity.TeamID, "slug": e2eIdentity.TeamSlug},
			"author": map[string]string{"id": e2eIdentity.AuthorID, "name": e2eIdentity.AuthorName},
		})
	})
	mux.HandleFunc("POST /cli/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeKitKey {
			http.Error(w, "unauthorized", 401)
			return
		}
		api.Revoke(fakeKitKey)
		s.mu.Lock()
		s.logouts++
		s.mu.Unlock()
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	old := openBrowser
	t.Cleanup(func() { openBrowser = old })
	openBrowser = func(raw string) {
		u, err := url.Parse(raw)
		if err != nil {
			t.Errorf("login URL %q", raw)
			return
		}
		q := u.Query()
		resp, err := http.Get("http://127.0.0.1:" + q.Get("port") + "/callback?code=code-1&state=" + url.QueryEscape(q.Get("state")))
		if err == nil {
			resp.Body.Close()
		}
	}
	return s
}

// kitE2EHome is a machine with no prior target (files mode), no board
// server and fast kit retries.
func kitE2EHome(t *testing.T) string {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, ".hev", "config.toml")
	t.Setenv("HEV_CONFIG", path)
	t.Setenv(kitServiceEnv, "")
	t.Setenv("HEV_KIT_OUTBOX", "")
	t.Setenv("HEV_BOARD_DIR", filepath.Join(home, ".hev", "board"))
	t.Setenv("FACTORY_BOARD_URL", "")
	for _, name := range layerenv.Names() {
		t.Setenv(name, "")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[capture]\nscan_interval = '1h'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := settings.Load(); err != nil {
		t.Fatal(err)
	}
	old := kitClient
	t.Cleanup(func() { kitClient = old; cli.KitAPI, cli.KitKey = "", "" })
	kitClient = func(k *daemon.KitConfig) *kitapi.Client {
		c := old(k)
		c.Backoff = func(int) time.Duration { return time.Millisecond }
		return c
	}
	return home
}

// writeClaudeTranscript writes a minimal Claude Code session whose prompt
// says phrase.
func writeClaudeTranscript(t *testing.T, home, session, phrase string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", "-work-demo")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-10 * time.Minute).UTC()
	var lines []string
	for i, v := range []map[string]any{
		{"type": "user", "message": map[string]any{"role": "user", "content": "Please investigate the " + phrase + " in the demo service."}},
		{"type": "assistant", "message": map[string]any{"role": "assistant", "model": "claude-test", "id": "msg-1", "content": []map[string]any{{"type": "text", "text": "Looked into the " + phrase + "; it was a stale cache."}}, "usage": map[string]any{"input_tokens": 10, "output_tokens": 5}}},
	} {
		v["sessionId"], v["cwd"], v["gitBranch"] = session, "/work/demo", "main"
		v["timestamp"] = start.Add(time.Duration(i) * time.Minute).Format(time.RFC3339Nano)
		v["uuid"] = fmt.Sprintf("%s-%02d", session, i)
		if i > 0 {
			v["parentUuid"] = fmt.Sprintf("%s-%02d", session, i-1)
		}
		b, _ := json.Marshal(v)
		lines = append(lines, string(b))
	}
	if err := os.WriteFile(filepath.Join(dir, session+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// runCaptureCycle runs hevd for one scan and waits for its outbox to drain.
func runCaptureCycle(t *testing.T) {
	t.Helper()
	old := daemon.AfterCycle
	t.Cleanup(func() { daemon.AfterCycle = old })
	drained := false
	daemon.AfterCycle = func(ctx context.Context, _ *slog.Logger) bool {
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			ob, err := os.ReadDir(daemon.KitOutboxDir())
			n := 0
			for _, e := range ob {
				if strings.HasSuffix(e.Name(), ".json") && !strings.HasPrefix(e.Name(), ".") {
					n++
				}
			}
			if err == nil && n == 0 {
				drained = true
				return true
			}
			time.Sleep(20 * time.Millisecond)
		}
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := daemon.Run(ctx); err != nil && !errors.Is(err, daemon.ErrRestart) {
		t.Fatalf("hevd: %v", err)
	}
	if !drained {
		t.Fatal("hevd's outbox did not drain to the kit API")
	}
}

// login → capture a fixture session → search finds it → board post
// round-trips → logout, all in process against a fake kit API.
func TestKitEndToEnd(t *testing.T) {
	home := kitE2EHome(t)
	api := kitapitest.New(t, fakeKitKey, e2eIdentity)
	svc := newKitService(t, api)
	ctx := context.Background()

	var out bytes.Buffer
	if err := runLogin(ctx, &out, svc.srv.URL, ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), fakeKitKey) {
		t.Fatal("login printed the key")
	}
	k := kitSignedIn()
	if k == nil || k.API != api.URL {
		t.Fatalf("not signed in to the fake API: %+v", k)
	}

	// Capture.
	const session = "e2e00000-0000-4000-8000-000000000001"
	writeClaudeTranscript(t, home, session, "zebra mango handshake")
	runCaptureCycle(t)
	rows, err := api.Traces.ListSessionRows(-1, nil)
	if err != nil || len(rows) != 1 || rows[0].SessionID != session {
		t.Fatalf("API sessions = %+v, %v", rows, err)
	}
	if rows[0].AuthorID != "user_9" || rows[0].Host != "box" {
		t.Fatalf("session not stamped by the key: author=%q host=%q", rows[0].AuthorID, rows[0].Host)
	}
	if blocks, _ := api.Traces.ListBlockRows(session); len(blocks) == 0 {
		t.Fatal("no blocks reached the API")
	}

	// Search through the API, the way `hev traces search --json` does.
	out.Reset()
	queryJSON = true
	t.Cleanup(func() { queryJSON = false })
	queryCmd.SetContext(ctx)
	queryCmd.SetOut(&out)
	t.Cleanup(func() { queryCmd.SetOut(nil) })
	if err := queryCmd.RunE(queryCmd, []string{"zebra", "mango", "handshake"}); err != nil {
		t.Fatal(err)
	}
	var hits []kitapi.SearchHit
	if err := json.Unmarshal(out.Bytes(), &hits); err != nil || len(hits) != 1 || hits[0].ID != session || !strings.Contains(hits[0].Snippet, "zebra mango handshake") {
		t.Fatalf("search = %s (%v)", out.String(), err)
	}
	if api.Requests("GET /v1/traces/search") == 0 {
		t.Fatal("search did not go through the kit API")
	}

	// The local dashboard reads through the API.
	dash := httptest.NewServer(kitDashboard(kitClient(k)))
	defer dash.Close()
	resp, err := http.Get(dash.URL + "/api/session/" + session)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("dashboard session: %v %v", resp, err)
	}
	resp.Body.Close()

	// Board post round-trips through the API.
	stdout := captureStdout(t, func() {
		if err := runBoard([]string{"post", "--json", "--board", "floor", "--title", "kit e2e", "-m", "posted through the kit API"}); err != nil {
			t.Fatal(err)
		}
	})
	var th kitapi.Thread
	if err := json.Unmarshal([]byte(stdout), &th); err != nil || th.ID == 0 || th.Author != "Ada" {
		t.Fatalf("post = %s (%v)", stdout, err)
	}
	stdout = captureStdout(t, func() {
		if err := runBoard([]string{"read", "--json", fmt.Sprint(th.ID)}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(stdout, "posted through the kit API") {
		t.Fatalf("read = %s", stdout)
	}
	if got, _ := api.Board.Threads(ctx, boardstore.ListOpts{}); len(got) != 1 {
		t.Fatalf("API board threads = %d", len(got))
	}

	// hev open asks the API for a one-time link.
	old := inviteOpenURL
	t.Cleanup(func() { inviteOpenURL = old })
	var opened string
	inviteOpenURL = func(_ io.Writer, u string) error { opened = u; return nil }
	if err := runOpen(ctx, &out); err != nil || !strings.HasPrefix(opened, api.URL+"/s/") {
		t.Fatalf("open = %q, %v", opened, err)
	}

	// Logout revokes the key and leaves the machine signed out.
	out.Reset()
	if err := runLogout(ctx, &out, false); err != nil {
		t.Fatal(err)
	}
	if kitSignedIn() != nil || svc.logouts != 1 {
		t.Fatal("still signed in after logout")
	}
	if _, err := kitapi.New(api.URL, fakeKitKey).Whoami(ctx); !errors.Is(err, kitapi.ErrUnauthorized) {
		t.Fatalf("key still valid after logout: %v", err)
	}
}

// hev login --import copies the local archive and board, and running it
// again sends nothing new.
func TestKitLoginImportsHistory(t *testing.T) {
	home := kitE2EHome(t)
	api := kitapitest.New(t, fakeKitKey, e2eIdentity)
	svc := newKitService(t, api)
	ctx := context.Background()

	// History from before sign-in: a session in the file archive and a
	// thread with a reply on the file board.
	files, err := filearchive.Open(filearchive.Root(""), "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if _, err := files.WriteSessions([]trace.SessionRow{{ID: "old-1", SessionID: "old-1", Harness: "codex", Host: "box", Start: now, End: now, Summary: "older work"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := files.WriteBlocks([]trace.BlockRow{{ID: "old-1:1", SessionID: "old-1", Seq: 1, Role: "user", BlockType: "text", Text: "the quokka migration"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Write([]layer.Row{{ID: "old-1:c", SessionID: "old-1", Text: "the quokka migration", Harness: "codex"}}); err != nil {
		t.Fatal(err)
	}
	local, err := boardstore.NewFiles(filepath.Join(home, ".hev", "board"))
	if err != nil {
		t.Fatal(err)
	}
	lt, err := local.CreateThread(ctx, boardstore.NewThread{Board: "learnings", Title: "local finding", Body: "found locally", Poster: boardstore.Poster{Author: "bob", Role: "agent", Host: "box"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.Reply(ctx, boardstore.NewPost{ThreadID: lt.ID, Body: "and a reply", Poster: boardstore.Poster{Author: "carol", Role: "human"}}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runLogin(ctx, &out, svc.srv.URL, ""); err != nil {
		t.Fatal(err)
	}
	offerKitImport(&out)
	if !strings.Contains(out.String(), "hev login --import") {
		t.Fatalf("login did not offer the import: %s", out.String())
	}
	out.Reset()
	if err := runKitImport(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "sent 3/3 batches") || !strings.Contains(out.String(), "1 threads and 2 posts") {
		t.Fatalf("progress: %s", out.String())
	}
	rows, _ := api.Traces.ListSessionRows(-1, nil)
	if len(rows) != 1 || rows[0].SessionID != "old-1" {
		t.Fatalf("API sessions = %+v", rows)
	}
	if hits, _, _ := api.Traces.PhraseHits("quokka migration", 10, nil); len(hits) != 1 {
		t.Fatalf("API chunks = %d", len(hits))
	}
	threads, _ := api.Board.Threads(ctx, boardstore.ListOpts{})
	if len(threads) != 1 || threads[0].Title != "local finding" || threads[0].Author != "Ada" {
		t.Fatalf("API threads = %+v", threads)
	}
	posts, _ := api.Board.Posts(ctx, threads[0].ID)
	if len(posts) != 2 || !strings.Contains(posts[0].Body, "posted by bob") || !strings.Contains(posts[1].Body, "and a reply") {
		t.Fatalf("API posts = %+v", posts)
	}

	// Resume after a new local reply: only the new post goes.
	if _, err := local.Reply(ctx, boardstore.NewPost{ThreadID: lt.ID, Body: "a later reply", Poster: boardstore.Poster{Author: "bob"}}); err != nil {
		t.Fatal(err)
	}
	before := api.Requests("POST /v1/traces/sessions")
	out.Reset()
	if err := runKitImport(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if api.Requests("POST /v1/traces/sessions") != before {
		t.Fatal("second import resent sessions")
	}
	threads, _ = api.Board.Threads(ctx, boardstore.ListOpts{})
	posts, _ = api.Board.Posts(ctx, threads[0].ID)
	if len(threads) != 1 || len(posts) != 3 {
		t.Fatalf("after resume: %d threads, %d posts", len(threads), len(posts))
	}
}

// captureStdout runs fn with os.Stdout redirected and returns what it wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()
	func() {
		defer func() { os.Stdout = old; w.Close() }()
		fn()
	}()
	return <-done
}
