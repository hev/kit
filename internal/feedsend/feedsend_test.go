package feedsend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/feed"
)

// fakeSecret is a GitHub-token-shaped string, split so scanners do not
// flag the fixture.
var fakeSecret = "ghp_" + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"

// slackPost is one chat.postMessage the fake received.
type slackPost struct {
	Channel, Text, ThreadTS string
}

type slackUpload struct {
	Name, ThreadTS, Channel string
	Bytes                   int
}

// fakeSlack is Slack's Web API: chat.postMessage and the external upload
// flow.
type fakeSlack struct {
	srv          *httptest.Server
	mu           sync.Mutex
	posts        []slackPost
	uploads      []slackUpload
	pending      map[string]slackUpload
	missingScope bool
	n            int
}

func newFakeSlack(t *testing.T) *fakeSlack {
	f := &fakeSlack{pending: map[string]slackUpload{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSlack) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer xoxb-test" && !strings.HasPrefix(r.URL.Path, "/upload/") {
		fmt.Fprint(w, `{"ok":false,"error":"invalid_auth"}`)
		return
	}
	switch {
	case r.URL.Path == "/chat.postMessage":
		var m struct {
			Channel  string `json:"channel"`
			Text     string `json:"text"`
			ThreadTS string `json:"thread_ts"`
		}
		_ = json.NewDecoder(r.Body).Decode(&m)
		f.n++
		f.posts = append(f.posts, slackPost{m.Channel, m.Text, m.ThreadTS})
		fmt.Fprintf(w, `{"ok":true,"channel":"C0FEED","ts":"1700000000.%06d"}`, f.n)
	case r.URL.Path == "/files.getUploadURLExternal":
		if f.missingScope {
			fmt.Fprint(w, `{"ok":false,"error":"missing_scope","needed":"files:write"}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(b))
		f.n++
		id := fmt.Sprintf("F%d", f.n)
		f.pending[id] = slackUpload{Name: form.Get("filename")}
		fmt.Fprintf(w, `{"ok":true,"upload_url":%q,"file_id":%q}`, f.srv.URL+"/upload/"+id, id)
	case strings.HasPrefix(r.URL.Path, "/upload/"):
		id := strings.TrimPrefix(r.URL.Path, "/upload/")
		b, _ := io.ReadAll(r.Body)
		u := f.pending[id]
		u.Bytes = len(b)
		f.pending[id] = u
		fmt.Fprint(w, "OK")
	case r.URL.Path == "/files.completeUploadExternal":
		var c struct {
			Files     []struct{ ID string } `json:"files"`
			ChannelID string                `json:"channel_id"`
			ThreadTS  string                `json:"thread_ts"`
		}
		_ = json.NewDecoder(r.Body).Decode(&c)
		for _, file := range c.Files {
			u := f.pending[file.ID]
			u.Channel, u.ThreadTS = c.ChannelID, c.ThreadTS
			f.uploads = append(f.uploads, u)
		}
		fmt.Fprint(w, `{"ok":true}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeSlack) all() ([]slackPost, []slackUpload) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]slackPost(nil), f.posts...), append([]slackUpload(nil), f.uploads...)
}

// fakeModel records what it was asked.
type fakeModel struct {
	mu      sync.Mutex
	prompts []string
	answer  string
	err     error
}

func (m *fakeModel) Summarize(_ context.Context, prompt string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prompts = append(m.prompts, prompt)
	if m.err != nil {
		return "", m.err
	}
	return m.answer, nil
}

type harness struct {
	t     *testing.T
	dir   string
	cfg   feed.Config
	slack *fakeSlack
	model *fakeModel
	obs   feed.Observer
	root  string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	root := t.TempDir()
	h := &harness{t: t, dir: dir, slack: newFakeSlack(t), model: &fakeModel{answer: "Added workspace rows to the picker; tests pass."}, root: root}
	h.cfg = feed.Config{Enabled: true, Channel: "C0FEED", Token: "secret://slack", Events: feed.AllKinds,
		Boards: []string{"feedback"}, ClientRoots: []string{filepath.Join(root, "clients")}, MaxUploadBytes: 50 << 20}
	h.obs = feed.Observer{Config: &h.cfg, Dir: dir, DispatchRoot: filepath.Join(root, "dispatch"), Host: "oven", Kick: func(string) {}}
	return h
}

func (h *harness) send() {
	h.t.Helper()
	s := &Sender{Dir: h.dir, Config: h.cfg,
		Post:     attention.SlackHTTP{Base: h.slack.srv.URL, Client: h.slack.srv.Client()},
		Token:    func(context.Context, string) (string, error) { return "xoxb-test", nil },
		Composer: Composer{Model: h.model, MaxUpload: h.cfg.MaxUploadBytes},
		Gap:      time.Millisecond, Idle: time.Millisecond, Sleep: func(time.Duration) {}}
	if err := s.Run(context.Background()); err != nil {
		h.t.Fatal(err)
	}
	if left := feed.Pending(h.dir); len(left) > 0 {
		log, _ := os.ReadFile(filepath.Join(h.dir, "sender.log"))
		h.t.Fatalf("%d records left; log:\n%s", len(left), log)
	}
}

// transcript writes the fixture with the fake secret in place.
func transcript(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(strings.ReplaceAll(string(b), "{{SECRET}}", fakeSecret)), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

// stopEvent is the recorded stop event, pointed at a transcript.
func stopEvent(t *testing.T, transcriptPath, cwd, target string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "stop-event.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ev map[string]any
	if err := json.Unmarshal(b, &ev); err != nil {
		t.Fatal(err)
	}
	ev["transcript_path"], ev["cwd"], ev["target"] = transcriptPath, cwd, target
	out, _ := json.Marshal(ev)
	return out
}

func TestLastTurnStopsAtRecordedSize(t *testing.T) {
	p := transcript(t, "claude-transcript.jsonl")
	size := feed.TranscriptOffset(p, time.Date(2026, 11, 19, 10, 1, 1, 0, time.UTC))
	turn, err := LastTurn(p, size)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(turn.Prompt, "Add workspace rows") || strings.Contains(turn.Reply, "Later reply") {
		t.Fatalf("turn = %+v", turn)
	}
	if !strings.Contains(turn.Reply, "Looking at the picker.") || !strings.Contains(turn.Reply, "go test ./... passes") {
		t.Fatalf("reply = %q", turn.Reply)
	}
	codex, err := LastTurn(filepath.Join("testdata", "codex-transcript.jsonl"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if codex.Prompt != "Fix the flaky test." || codex.Reply != "Fixed the race in TestPicker." {
		t.Fatalf("codex turn = %+v", codex)
	}
}

func TestStopSummaryPromptFromRecordedEvent(t *testing.T) {
	h := newHarness(t)
	p := transcript(t, "claude-transcript.jsonl")
	// The turn stopped before the later turn was written.
	cut := feed.TranscriptOffset(p, time.Date(2026, 11, 19, 10, 1, 1, 0, time.UTC))
	full, _ := os.Stat(p)
	if err := os.Truncate(p, cut); err != nil {
		t.Fatal(err)
	}
	h.obs.ObserveHook(stopEvent(t, p, "/home/u/dispatch/kit-pick-workspaces/wt", "dispatch:kit-pick-workspaces"))
	// The transcript grows before the sender runs; the record's size holds.
	b, _ := os.ReadFile(filepath.Join("testdata", "claude-transcript.jsonl"))
	_ = os.WriteFile(p, []byte(strings.ReplaceAll(string(b), "{{SECRET}}", fakeSecret)), 0600)
	if st, _ := os.Stat(p); st.Size() != full.Size() {
		t.Fatal("fixture rewrite")
	}
	h.send()

	if len(h.model.prompts) != 1 {
		t.Fatalf("model asked %d times", len(h.model.prompts))
	}
	q := h.model.prompts[0]
	for _, want := range []string{`dispatch run "kit-pick-workspaces"`, "<prompt>\nAdd workspace rows to the picker.", "go test ./... passes", "[REDACTED:"} {
		if !strings.Contains(q, want) {
			t.Errorf("prompt lacks %q:\n%s", want, q)
		}
	}
	for _, never := range []string{fakeSecret, "Later reply", "must not be read", "tool_result", "system-reminder"} {
		if strings.Contains(q, never) {
			t.Errorf("prompt has %q:\n%s", never, q)
		}
	}
	posts, _ := h.slack.all()
	if len(posts) != 2 {
		t.Fatalf("posts = %+v", posts)
	}
	if posts[0].ThreadTS != "" || !strings.Contains(posts[0].Text, "*kit-pick-workspaces* · dispatch run · claude on oven") {
		t.Fatalf("parent = %+v", posts[0])
	}
	if posts[1].ThreadTS != "1700000000.000001" || posts[1].Text != ":speech_balloon: Added workspace rows to the picker; tests pass." {
		t.Fatalf("reply = %+v", posts[1])
	}
	for _, p := range posts {
		if strings.Contains(p.Text, "_receives:_") || strings.Contains(p.Text, "_hook:_") {
			t.Fatalf("field dump in %q", p.Text)
		}
	}
}

func TestModelErrorPostsCompactFallback(t *testing.T) {
	h := newHarness(t)
	h.model.err = errors.New("model: HTTP 529 overloaded_error")
	p := transcript(t, "claude-transcript.jsonl")
	h.obs.ObserveHook(stopEvent(t, p, "/home/u/work", ""))
	h.send()
	posts, _ := h.slack.all()
	if len(posts) != 2 {
		t.Fatalf("posts = %+v", posts)
	}
	got := posts[1].Text
	if got != ":white_check_mark: Finished a turn: Later reply." {
		t.Fatalf("fallback = %q", got)
	}
	log, _ := os.ReadFile(filepath.Join(h.dir, "sender.log"))
	if !strings.Contains(string(log), "summary fell back: model: HTTP 529") {
		t.Fatalf("log = %s", log)
	}
}

func TestModelErrorFallbackNeverLeaksSecret(t *testing.T) {
	h := newHarness(t)
	h.model.err = errors.New("timeout")
	dir := t.TempDir()
	p := filepath.Join(dir, "t.jsonl")
	line := fmt.Sprintf(`{"type":"user","message":{"role":"user","content":"deploy it"}}`+"\n"+`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Pushed with token %s."}]}}`+"\n", fakeSecret)
	_ = os.WriteFile(p, []byte(line), 0600)
	h.obs.ObserveHook(stopEvent(t, p, "/home/u/work", ""))
	h.send()
	posts, _ := h.slack.all()
	for _, p := range posts {
		if strings.Contains(p.Text, fakeSecret) {
			t.Fatalf("secret posted: %q", p.Text)
		}
	}
	if !strings.Contains(posts[1].Text, "[REDACTED:") {
		t.Fatalf("fallback = %q", posts[1].Text)
	}
}

func TestClientSessionSendsNoContent(t *testing.T) {
	h := newHarness(t)
	p := transcript(t, "claude-transcript.jsonl")
	cwd := filepath.Join(h.root, "clients", "acme", "app")
	start, _ := json.Marshal(map[string]any{"schema": "hev.hook.event/v1", "event": "session-start", "harness": "claude", "session_id": "c1", "cwd": cwd, "source": "startup", "transcript_path": p})
	prompt, _ := json.Marshal(map[string]any{"schema": "hev.hook.event/v1", "event": "prompt-submit", "harness": "claude", "session_id": "c1", "cwd": cwd, "prompt": "acme secret plan", "transcript_path": p})
	h.cfg.Skip = []string{feed.KindPreTool, feed.KindPostTool}
	h.obs.ObserveHook(start)
	h.obs.ObserveHook(prompt)
	stop := map[string]any{}
	_ = json.Unmarshal(stopEvent(t, p, cwd, ""), &stop)
	stop["session_id"] = "c1"
	b, _ := json.Marshal(stop)
	h.obs.ObserveHook(b)
	// A dispatch run whose repository is client work is client work too.
	run := filepath.Join(h.root, "dispatch", "acme-fix")
	_ = os.MkdirAll(run, 0700)
	_ = os.WriteFile(filepath.Join(run, "meta.json"), []byte(fmt.Sprintf(`{"resolved_repo":%q}`, filepath.Join(h.root, "clients", "acme"))), 0600)
	_ = os.WriteFile(filepath.Join(run, "order.md"), []byte("# Acme private order"), 0600)
	h.obs.ObserveHook(stopEvent(t, p, filepath.Join(run, "wt"), "dispatch:acme-fix"))
	// Evidence from a client session is not even spooled.
	ev, _ := json.Marshal(map[string]any{"schema": "hev.hook.event/v1", "event": "post-tool", "harness": "claude", "session_id": "c1", "cwd": cwd, "tool": map[string]any{"name": "Bash", "command": "node record.cjs flow.cjs out"}})
	h.obs.ObserveHook(ev)

	// Nothing private is even written to the spool.
	for _, f := range feed.Pending(h.dir) {
		b, _ := os.ReadFile(f)
		for _, never := range []string{"acme secret plan", p, "clients", fakeSecret} {
			if strings.Contains(string(b), never) {
				t.Fatalf("spool record has %q: %s", never, b)
			}
		}
	}
	h.send()
	if len(h.model.prompts) != 0 {
		t.Fatalf("client content went to the model: %q", h.model.prompts)
	}
	posts, uploads := h.slack.all()
	if len(uploads) != 0 {
		t.Fatalf("uploads = %+v", uploads)
	}
	var texts []string
	for _, p := range posts {
		texts = append(texts, p.Text)
		for _, never := range []string{"acme", "picker", "Add workspace", "Acme private", "clients"} {
			if strings.Contains(p.Text, never) && !strings.Contains(p.Text, "*acme-fix*") {
				t.Fatalf("client content %q posted: %q", never, p.Text)
			}
		}
	}
	want := []string{
		":thread: *c1* · client session · claude on oven",
		":lock: Client session active.",
		":lock: Client session active.",
		":lock: Client session: turn ended.",
		":thread: *acme-fix* · dispatch run · client session · claude on oven",
		":lock: Client session: turn ended.",
	}
	if strings.Join(texts, "\n") != strings.Join(want, "\n") {
		t.Fatalf("posts:\n%s\nwant:\n%s", strings.Join(texts, "\n"), strings.Join(want, "\n"))
	}
}

func TestOrchestratorRoundsShareOneThread(t *testing.T) {
	h := newHarness(t)
	orch := filepath.Join(h.root, "dispatch", "orch-kit-07-train2")
	for _, n := range []string{"173", "174"} {
		d := filepath.Join(orch, "rounds", n)
		_ = os.MkdirAll(d, 0700)
		_ = os.WriteFile(filepath.Join(d, "reply.md"), []byte("Round "+n+": checked 3 runs, nudged one.\n"), 0600)
	}
	h.model.answer = "Checked three runs and nudged the stalled one."
	for _, n := range []string{"173", "174"} {
		cwd := filepath.Join(orch, "rounds", n)
		start, _ := json.Marshal(map[string]any{"schema": "hev.hook.event/v1", "event": "session-start", "harness": "claude", "session_id": "s" + n, "cwd": cwd, "target": "orch:kit-07-train2", "source": "startup"})
		h.obs.ObserveHook(start)
		p := transcript(t, "claude-transcript.jsonl")
		ev := stopEvent(t, p, cwd, "orch:kit-07-train2")
		var m map[string]any
		_ = json.Unmarshal(ev, &m)
		m["session_id"] = "s" + n
		ev, _ = json.Marshal(m)
		h.obs.ObserveHook(ev)
	}
	// A notice about the orchestrator lands in the same thread.
	n := attention.New(attention.KindNeedsYou, attention.SourceOrchestrator, "oven", "kit-07-train2", "k", "needs-decision", "needs-decision merge: Merge PR #300 now or wait for the soak?", attention.Links{})
	b, _ := json.Marshal(n)
	h.obs.ObserveLifecycle("needs-you", b)
	h.send()

	posts, _ := h.slack.all()
	if len(posts) != 4 {
		t.Fatalf("want 1 parent + 2 rounds + 1 notice, got %+v", posts)
	}
	parent := posts[0]
	if parent.ThreadTS != "" || parent.Text != ":thread: *kit-07-train2* · orchestrator on oven" {
		t.Fatalf("parent = %+v", parent)
	}
	for i, want := range []string{"*Round 173:* Checked three runs", "*Round 174:* Checked three runs", ":rotating_light: *Needs you* — decision `merge`: Merge PR #300 now or wait for the soak?"} {
		p := posts[i+1]
		if p.ThreadTS != "1700000000.000001" || !strings.HasPrefix(p.Text, want) {
			t.Fatalf("post %d = %+v, want prefix %q in the orchestrator thread", i+1, p, want)
		}
	}
	if !strings.Contains(h.model.prompts[0], "Round 173: checked 3 runs") {
		t.Fatalf("round prompt = %s", h.model.prompts[0])
	}
}

func TestNoticeInRunThreadWithQuestionVerbatim(t *testing.T) {
	h := newHarness(t)
	p := transcript(t, "claude-transcript.jsonl")
	h.obs.ObserveHook(stopEvent(t, p, "/home/u/dispatch/kit-pick-workspaces/wt", "dispatch:kit-pick-workspaces"))
	q := "Postgres or SQLite for the picker cache? SQLite keeps it local (recommended); Postgres shares it across hosts but needs the gateway up."
	n := attention.New(attention.KindNeedsYou, attention.SourceRun, "oven", "kit-pick-workspaces", "k", "needs-review", "needs-decision db: "+q, attention.Links{PR: "https://github.com/hev/kit/pull/1", Run: "hev dispatch show kit-pick-workspaces --host oven"})
	b, _ := json.Marshal(n)
	h.obs.ObserveLifecycle("needs-you", b)
	done := attention.New(attention.KindDone, attention.SourceRun, "oven", "kit-pick-workspaces", "k2", "failed", "failed: tests did not pass", attention.Links{})
	b, _ = json.Marshal(done)
	h.obs.ObserveLifecycle("done", b)
	h.send()
	posts, _ := h.slack.all()
	if len(posts) != 4 {
		t.Fatalf("posts = %+v", posts)
	}
	want := ":rotating_light: *Needs you* — decision `db`: " + q + "\nPR: https://github.com/hev/kit/pull/1\n`hev dispatch show kit-pick-workspaces --host oven`"
	if posts[2].Text != want || posts[2].ThreadTS != posts[1].ThreadTS {
		t.Fatalf("notice = %+v\nwant %q in the run thread", posts[2], want)
	}
	if posts[3].Text != ":checkered_flag: *Done* (failed) — failed: tests did not pass" || posts[3].ThreadTS != posts[1].ThreadTS {
		t.Fatalf("done = %+v", posts[3])
	}
}

// fakeBoard serves one thread with an image on its first post.
func fakeBoard(t *testing.T, png []byte) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/threads/7":
			fmt.Fprintf(w, `{"thread":{"id":7},"posts":[{"n":1,"images":[{"url":"/blobs/abc","name":"before.png","size":%d}]},{"n":2,"images":[]}]}`, len(png))
		case "/blobs/abc":
			_, _ = w.Write(png)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestBoardPostOwnThreadWithImage(t *testing.T) {
	h := newHarness(t)
	png := []byte("\x89PNG fake image bytes")
	board := fakeBoard(t, png)
	h.model.answer = "The picker hides archived workspaces, and the poster asks for a toggle."
	body := "The picker hides archived workspaces. Could we get a toggle?"
	post, _ := json.Marshal(map[string]any{"schema": "hev.board.post/v1", "id": "7", "event": "board-post", "board": "feedback", "kind": "post", "title": "Picker hides archived workspaces", "body": body, "url": board.URL + "/t/7", "author": "adam", "tags": []string{}})
	reply, _ := json.Marshal(map[string]any{"schema": "hev.board.post/v1", "id": "p2", "event": "board-post", "board": "feedback", "kind": "reply", "thread_id": 7, "title": "Picker hides archived workspaces", "body": "Same here.", "url": board.URL + "/t/7#p2", "author": "claude"})
	other, _ := json.Marshal(map[string]any{"schema": "hev.board.post/v1", "id": "9", "event": "board-post", "board": "research", "kind": "post", "title": "not fed", "url": board.URL + "/t/9"})
	h.obs.ObserveLifecycle("board-post", post)
	h.obs.ObserveLifecycle("board-post", reply)
	h.obs.ObserveLifecycle("board-post", other)
	h.send()
	posts, uploads := h.slack.all()
	if len(posts) != 2 {
		t.Fatalf("posts = %+v", posts)
	}
	wantParent := ":clipboard: *feedback #7:* Picker hides archived workspaces\n_by adam_ — The picker hides archived workspaces, and the poster asks for a toggle.\n" + board.URL + "/t/7"
	if posts[0].Text != wantParent || posts[0].ThreadTS != "" {
		t.Fatalf("thread = %q", posts[0].Text)
	}
	if !strings.HasPrefix(posts[1].Text, ":speech_balloon: *claude* replied: ") || posts[1].ThreadTS != "1700000000.000001" || !strings.HasSuffix(posts[1].Text, "/t/7#p2") {
		t.Fatalf("reply = %+v", posts[1])
	}
	if len(uploads) != 1 || uploads[0].Name != "before.png" || uploads[0].Bytes != len(png) || uploads[0].ThreadTS != "1700000000.000001" || uploads[0].Channel != "C0FEED" {
		t.Fatalf("uploads = %+v", uploads)
	}
}

func writeEvidence(t *testing.T, verdict string) string {
	dir := filepath.Join(t.TempDir(), "evidence-001")
	_ = os.MkdirAll(dir, 0700)
	res := map[string]any{"title": "Picker shows workspace rows", "revision": "abc1234", "expected": "Workspace rows appear under Recent.", "actual": "Three workspace rows rendered.", "verdict": verdict, "studio": "http://100.126.12.95:8800/#uat/42"}
	b, _ := json.Marshal(res)
	_ = os.WriteFile(filepath.Join(dir, "result.json"), b, 0600)
	_ = os.WriteFile(filepath.Join(dir, "final.png"), []byte("png-bytes"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "recording.mp4"), make([]byte, 3<<20), 0600)
	return dir
}

func TestEvidenceInRunThreadWithMedia(t *testing.T) {
	h := newHarness(t)
	h.cfg.MaxUploadBytes = 2 << 20 // the 3 MB video is over the cap
	dir := writeEvidence(t, "pass")
	p := transcript(t, "claude-transcript.jsonl")
	h.obs.ObserveHook(stopEvent(t, p, "/home/u/dispatch/kit-pick-workspaces/wt", "dispatch:kit-pick-workspaces"))
	cmd, _ := json.Marshal(map[string]any{"schema": "hev.hook.event/v1", "event": "post-tool", "harness": "claude", "session_id": "x", "cwd": filepath.Dir(dir), "target": "dispatch:kit-pick-workspaces",
		"tool": map[string]any{"name": "Bash", "kind": "other", "command": "node ~/workspace/shell/hosts/mini/acceptance-recorder/record.cjs flow.cjs evidence-001 http://100.126.12.95:8800"}})
	h.obs.ObserveHook(cmd)
	h.send()
	posts, uploads := h.slack.all()
	if len(posts) != 4 {
		t.Fatalf("posts = %+v", posts)
	}
	ev := posts[2]
	want := ":white_check_mark: *Acceptance evidence: PASS* — Picker shows workspace rows\n*Expected:* Workspace rows appear under Recent.\n*Actual:* Three workspace rows rendered.\n*Revision:* `abc1234`\n*Studio:* http://100.126.12.95:8800/#uat/42"
	if ev.Text != want || ev.ThreadTS != "1700000000.000001" {
		t.Fatalf("evidence = %+v", ev)
	}
	if len(uploads) != 1 || uploads[0].Name != "final.png" || uploads[0].ThreadTS != "1700000000.000001" {
		t.Fatalf("uploads = %+v", uploads)
	}
	if !strings.Contains(posts[3].Text, "recording.mp4 not attached (3.0 MB, over the 2 MB cap): http://100.126.12.95:8800/#uat/42") {
		t.Fatalf("cap note = %q", posts[3].Text)
	}
}

func TestEvidenceOwnThreadAndLinkFallbackWithoutFilesWrite(t *testing.T) {
	h := newHarness(t)
	h.slack.missingScope = true
	dir := writeEvidence(t, "fail")
	if !h.obs.ObserveEvidence(dir, "") {
		t.Fatal("not spooled")
	}
	h.send()
	posts, uploads := h.slack.all()
	if len(uploads) != 0 || len(posts) != 2 {
		t.Fatalf("posts = %+v uploads = %+v", posts, uploads)
	}
	if !strings.HasPrefix(posts[0].Text, ":x: *Acceptance evidence: FAIL*") || posts[0].ThreadTS != "" {
		t.Fatalf("thread = %+v", posts[0])
	}
	note := posts[1].Text
	for _, want := range []string{"final.png not attached (the bot token lacks files:write): http://100.126.12.95:8800/#uat/42", "recording.mp4 not attached (the bot token lacks files:write)"} {
		if !strings.Contains(note, want) {
			t.Fatalf("note = %q, want %q", note, want)
		}
	}
	if posts[1].ThreadTS != "1700000000.000001" {
		t.Fatalf("note not in thread: %+v", posts[1])
	}
}

func TestSlackDownKeepsRecordsForLater(t *testing.T) {
	h := newHarness(t)
	h.obs.ObserveHook(stopEvent(t, "", "/home/u/work", ""))
	s := &Sender{Dir: h.dir, Config: h.cfg, Post: attention.SlackHTTP{Base: "http://127.0.0.1:1"},
		Token:    func(context.Context, string) (string, error) { return "xoxb-test", nil },
		Composer: Composer{Model: h.model}, Gap: time.Millisecond, Idle: time.Millisecond, Sleep: func(time.Duration) {}}
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(feed.Pending(h.dir)) != 1 {
		t.Fatal("record dropped while Slack was unreachable")
	}
	if _, paused := s.BackedOff(); !paused {
		t.Fatal("no backoff")
	}
}

func TestAnthropicRequest(t *testing.T) {
	var got map[string]any
	var hdr http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&got)
		fmt.Fprint(w, `{"stop_reason":"end_turn","content":[{"type":"thinking","thinking":""},{"type":"text","text":"Started kit-pick:\n adding rows."}]}`)
	}))
	defer srv.Close()
	a := &Anthropic{Base: srv.URL, Model: feed.DefaultModel, Key: func(context.Context) (string, error) { return "sk-test", nil }, Client: srv.Client()}
	s, err := a.Summarize(context.Background(), "hello")
	if err != nil || s != "Started kit-pick: adding rows." {
		t.Fatalf("summary = %q, %v", s, err)
	}
	if hdr.Get("x-api-key") != "sk-test" || hdr.Get("anthropic-version") == "" || got["model"] != "claude-haiku-5-5" {
		t.Fatalf("request = %v %v", hdr, got)
	}
	if _, ok := got["temperature"]; ok {
		t.Fatal("temperature is rejected by current models")
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(529)
		fmt.Fprint(w, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
	}))
	defer srv2.Close()
	a.Base, a.Client = srv2.URL, srv2.Client()
	if _, err := a.Summarize(context.Background(), "hello"); err == nil || !strings.Contains(err.Error(), "529 overloaded_error") {
		t.Fatalf("err = %v", err)
	}
}

// Claude Code may run the Stop hook before the turn's last reply reaches
// the transcript; that reply still belongs to the turn.
func TestLastTurnKeepsReplyWrittenAfterStop(t *testing.T) {
	p := transcript(t, "claude-transcript.jsonl")
	// The hook saw the transcript before the final assistant line.
	size := feed.TranscriptOffset(p, time.Date(2026, 11, 19, 10, 0, 30, 0, time.UTC))
	turn, err := LastTurn(p, size)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(turn.Reply, "go test ./... passes") || strings.Contains(turn.Reply, "Later reply") {
		t.Fatalf("reply = %q", turn.Reply)
	}
}
