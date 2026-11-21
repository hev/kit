package attention

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakePoster struct {
	mu    sync.Mutex
	posts []SlackMessage
	fail  error
}

func (f *fakePoster) Post(_ context.Context, m SlackMessage) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return "", "", f.fail
	}
	f.posts = append(f.posts, m)
	return "D0DM", "1700000000." + strings.Repeat("0", 5-len(f.posts)) + string(rune('0'+len(f.posts))), nil
}

func newEmitter(t *testing.T, root string, kinds ...string) (*Emitter, *fakePoster, *[]string) {
	t.Helper()
	p := &fakePoster{}
	var hooked []string
	var mu sync.Mutex
	e := &Emitter{
		Root:   root,
		Config: Config{Slack: &Slack{Channel: "U0TEST", Token: "secret://slack-bot-token", Kinds: kinds}},
		Post:   p,
		Token:  func(context.Context, string) (string, error) { return "xoxb-test", nil },
		Hooks: func(_ context.Context, event string, payload []byte) {
			mu.Lock()
			defer mu.Unlock()
			hooked = append(hooked, event+" "+string(payload))
		},
	}
	if len(kinds) == 0 {
		e.Config.Slack.Kinds = []string{KindNeedsYou}
	}
	return e, p, &hooked
}

func sample(kind, key string) Notice {
	return New(kind, SourceRun, "oven", "kit-x", key, "needs-review", "needs-decision db: which database?", Links{PR: "https://github.com/hev/kit/pull/1", Run: "hev dispatch show kit-x --host oven"})
}

func TestMessageShapeLengthAndRedaction(t *testing.T) {
	n := sample(KindNeedsYou, "k")
	msg := n.Message("")
	first, rest, _ := strings.Cut(msg, "\n")
	if first != "[needs you] kit-x on oven: needs-decision db: which database?" {
		t.Fatalf("first line %q", first)
	}
	if rest != "PR: https://github.com/hev/kit/pull/1\nRun: hev dispatch show kit-x --host oven" {
		t.Fatalf("links %q", rest)
	}
	if got := n.Message("[test]"); !strings.HasPrefix(got, "[test] [needs you] kit-x on oven: ") {
		t.Fatalf("tag: %q", got)
	}
	long := New(KindNeedsYou, SourceRun, "oven", strings.Repeat("n", 120), "k", "failed", strings.Repeat("word ", 400), Links{PR: "https://github.com/hev/kit/pull/" + strings.Repeat("9", 150), Run: strings.Repeat("r", 150), Board: "https://board/" + strings.Repeat("b", 180)})
	if m := long.Message("[test]"); len([]rune(m)) >= 400 {
		t.Fatalf("message is %d characters", len([]rune(m)))
	}
	// Assembled so push protection does not mistake the fixture for a token.
	fake := strings.Join([]string{"xoxb", "123456789012", "123456789012", "abcdefghijklmnopqrstuvwx"}, "-")
	secret := New(KindNeedsYou, SourceRun, "oven", "kit-x", "k", "failed", "auth failed with token "+fake+" and\nnewline", Links{})
	if strings.Contains(secret.Message(""), "xoxb-123456789012") || strings.Contains(secret.Text, "\n") {
		t.Fatalf("secret or newline leaked: %q", secret.Message(""))
	}
	client := sample(KindNeedsYou, "k")
	client.Client = true
	cm := client.Message("")
	if cm != "[needs you] kit-x on oven: needs-review\nRun: hev dispatch show kit-x --host oven" {
		t.Fatalf("client message %q", cm)
	}
}

func TestIDStableAndDistinct(t *testing.T) {
	a, b := sample(KindNeedsYou, "start|end"), sample(KindNeedsYou, "start|end")
	if a.ID != b.ID || !strings.HasPrefix(a.ID, "att_") || len(a.ID) != 28 {
		t.Fatalf("ids %q %q", a.ID, b.ID)
	}
	for _, other := range []Notice{sample(KindNeedsYou, "start|end2"), sample(KindDone, "start|end"), New(KindNeedsYou, SourceRun, "mini", "kit-x", "start|end", "", "", Links{})} {
		if other.ID == a.ID {
			t.Fatal("distinct moments share an ID")
		}
	}
}

func TestDedupeAcrossRestart(t *testing.T) {
	root := t.TempDir()
	state := t.TempDir()
	e, p, hooked := newEmitter(t, root)
	n := sample(KindNeedsYou, "k")
	if err := e.Emit(context.Background(), n, state); err != nil {
		t.Fatal(err)
	}
	// A new process (fresh emitter, same outbox) re-emits the same moment.
	e2, p2, hooked2 := newEmitter(t, root)
	n2 := sample(KindNeedsYou, "k")
	if err := e2.Emit(context.Background(), n2, state); err != nil {
		t.Fatal(err)
	}
	if len(p.posts) != 1 || len(p2.posts) != 0 || len(*hooked) != 1 || len(*hooked2) != 0 {
		t.Fatalf("posts %d+%d hooks %d+%d", len(p.posts), len(p2.posts), len(*hooked), len(*hooked2))
	}
	st := ReadStatus(state)
	if len(st) != 1 || st[0].Slack != StatusDelivered || StatusLine(state) != "" {
		t.Fatalf("status %+v", st)
	}
	if e2.Flush(context.Background()) != 0 || len(p2.posts) != 0 {
		t.Fatal("flush reposted a delivered notice")
	}
}

func TestKindsFiltering(t *testing.T) {
	e, p, hooked := newEmitter(t, t.TempDir())
	_ = e.Emit(context.Background(), sample(KindDone, "a"), "")
	_ = e.Emit(context.Background(), sample(KindReady, "a"), "")
	_ = e.Emit(context.Background(), sample(KindNeedsYou, "a"), "")
	if len(p.posts) != 1 || !strings.HasPrefix(p.posts[0].Text, "[needs you]") {
		t.Fatalf("posts %+v", p.posts)
	}
	if len(*hooked) != 3 {
		t.Fatalf("hooks see every kind; got %d", len(*hooked))
	}
	wide, wp, _ := newEmitter(t, t.TempDir(), KindNeedsYou, KindReady, KindDone)
	_ = wide.Emit(context.Background(), sample(KindDone, "a"), "")
	_ = wide.Emit(context.Background(), sample(KindReady, "a"), "")
	if len(wp.posts) != 2 || !strings.HasPrefix(wp.posts[0].Text, "[done]") || !strings.HasPrefix(wp.posts[1].Text, "[ready]") {
		t.Fatalf("widened posts %+v", wp.posts)
	}
	none := &Emitter{Root: t.TempDir(), Hooks: func(context.Context, string, []byte) {}}
	if err := none.Emit(context.Background(), sample(KindNeedsYou, "a"), ""); err != nil {
		t.Fatal(err)
	}
}

func TestThreadingPerRun(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	e, p, _ := newEmitter(t, root)
	_ = e.Emit(context.Background(), sample(KindNeedsYou, "first"), state)
	_ = e.Emit(context.Background(), sample(KindNeedsYou, "second"), state)
	other := New(KindNeedsYou, SourceOrchestrator, "oven", "kit-x", "first", "stalled", "stalled", Links{})
	_ = e.Emit(context.Background(), other, t.TempDir())
	if len(p.posts) != 3 {
		t.Fatalf("posts %d", len(p.posts))
	}
	if p.posts[0].ThreadTS != "" || p.posts[0].Channel != "U0TEST" {
		t.Fatalf("first post %+v", p.posts[0])
	}
	// The reply goes to the conversation Slack answered with, in thread.
	if p.posts[1].ThreadTS == "" || p.posts[1].ThreadTS != "1700000000.00001" || p.posts[1].Channel != "D0DM" {
		t.Fatalf("second post %+v", p.posts[1])
	}
	if p.posts[2].ThreadTS != "" {
		t.Fatal("an orchestrator shares a run's thread")
	}
	var th thread
	b, err := os.ReadFile(filepath.Join(state, ThreadFile))
	if err != nil || json.Unmarshal(b, &th) != nil || th.TS != "1700000000.00001" || th.Configured != "U0TEST" {
		t.Fatalf("thread file %s %v", b, err)
	}
	// A pulled notice has no state dir: its thread lives under the outbox.
	pulled := New(KindNeedsYou, SourceRun, "win-oven", "remote", "p1", "blocked", "", Links{})
	_ = e.Emit(context.Background(), pulled, "")
	again := New(KindNeedsYou, SourceRun, "win-oven", "remote", "p2", "failed", "", Links{})
	_ = e.Emit(context.Background(), again, "")
	if p.posts[3].ThreadTS != "" || p.posts[4].ThreadTS == "" {
		t.Fatalf("pulled notices not threaded: %+v", p.posts[3:])
	}
}

func TestThreadGoneStartsNewThread(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	e, _, _ := newEmitter(t, root)
	_ = e.Emit(context.Background(), sample(KindNeedsYou, "first"), state)
	calls := 0
	e.Post = posterFunc(func(m SlackMessage) (string, string, error) {
		calls++
		if m.ThreadTS != "" {
			return "", "", SlackError{Code: "thread_not_found"}
		}
		return "D0DM", "1800000000.00001", nil
	})
	if err := e.Emit(context.Background(), sample(KindNeedsYou, "second"), state); err != nil || calls != 2 {
		t.Fatalf("err %v calls %d", err, calls)
	}
}

type posterFunc func(SlackMessage) (string, string, error)

func (f posterFunc) Post(_ context.Context, m SlackMessage) (string, string, error) { return f(m) }

func TestFailureBacksOffAndRetries(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	e, p, _ := newEmitter(t, root)
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	e.Now = func() time.Time { return now }
	p.fail = SlackError{Code: "ratelimited"}
	n := sample(KindNeedsYou, "k")
	if err := e.Emit(context.Background(), n, state); err == nil {
		t.Fatal("expected the post error to be reported to the caller's log")
	}
	var rec Record
	_ = readJSON(e.recordPath(n.ID), &rec)
	if rec.Slack.Status != StatusQueued || rec.Slack.Attempts != 1 || !rec.Slack.NextAttempt.Equal(now.Add(time.Minute)) || rec.Slack.Error != "slack: ratelimited" {
		t.Fatalf("delivery %+v", rec.Slack)
	}
	if line := StatusLine(state); !strings.Contains(line, "retrying (attempt 1") || !strings.Contains(line, "ratelimited") {
		t.Fatalf("status line %q", line)
	}
	if !strings.Contains(e.Describe(), "1 retrying") {
		t.Fatal(e.Describe())
	}
	// Not yet due: nothing is attempted.
	p.fail = nil
	if e.Flush(context.Background()) != 0 || len(p.posts) != 0 {
		t.Fatal("retried before backoff")
	}
	now = now.Add(time.Minute)
	if e.Flush(context.Background()) != 1 || len(p.posts) != 1 {
		t.Fatal("due retry did not post")
	}
	if StatusLine(state) != "" {
		t.Fatal(StatusLine(state))
	}
	if backoff(1) != time.Minute || backoff(2) != 2*time.Minute || backoff(20) != time.Hour {
		t.Fatal("backoff")
	}
	// After MaxAttempts the post is failed, and says so.
	p.fail = errors.New("slack: HTTP 500")
	m := sample(KindNeedsYou, "k2")
	_ = e.Emit(context.Background(), m, state)
	for i := 0; i < MaxAttempts; i++ {
		now = now.Add(2 * time.Hour)
		e.Flush(context.Background())
	}
	_ = readJSON(e.recordPath(m.ID), &rec)
	if rec.Slack.Status != StatusFailed || !strings.Contains(StatusLine(state), "failed after") {
		t.Fatalf("%+v %q", rec.Slack, StatusLine(state))
	}
}

func TestConfig(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "config.toml")
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	c, err := LoadConfig(write("[notify.slack]\nchannel = \"U0A4L3ENZ39\"\ntoken = \"secret://SLACK_BOT_TOKEN\"\n"))
	if err != nil || c.Slack == nil || !reflect.DeepEqual(c.Slack.Kinds, []string{KindNeedsYou}) {
		t.Fatalf("%+v %v", c.Slack, err)
	}
	if _, err := LoadConfig(write("[notify.slack]\nchannel = \"U1\"\ntoken = \"xoxb-1234\"\n")); err == nil || strings.Contains(err.Error(), "xoxb") {
		t.Fatalf("literal token accepted or echoed: %v", err)
	}
	if _, err := LoadConfig(write("[notify.slack]\nchannel = \"U1\"\ntoken = \"op://v/i/f\"\nkinds = [\"loud\"]\n")); err == nil {
		t.Fatal("unknown kind accepted")
	}
	if c, err := LoadConfig(write("[notify]\nclient = true\n")); err != nil || !c.Client || c.Slack != nil {
		t.Fatalf("%+v %v", c, err)
	}
	if c, err := LoadConfig(filepath.Join(dir, "missing.toml")); err != nil || c.Slack != nil {
		t.Fatal(err)
	}
}

func TestClientHostStripsTextAndLinks(t *testing.T) {
	e, p, hooked := newEmitter(t, t.TempDir())
	e.Config.Client = true
	_ = e.Emit(context.Background(), sample(KindNeedsYou, "k"), "")
	if len(p.posts) != 1 || strings.Contains(p.posts[0].Text, "database") || strings.Contains(p.posts[0].Text, "github.com") {
		t.Fatalf("client post %q", p.posts[0].Text)
	}
	if strings.Contains((*hooked)[0], "database") || !strings.Contains((*hooked)[0], `"client":true`) {
		t.Fatalf("client hook %s", (*hooked)[0])
	}
}

func TestSlackHTTP(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/chat.postMessage" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got["channel"] == "Ubad" {
			w.Write([]byte(`{"ok":false,"error":"channel_not_found"}`))
			return
		}
		w.Write([]byte(`{"ok":true,"channel":"D1","ts":"1.2"}`))
	}))
	defer srv.Close()
	s := SlackHTTP{Base: srv.URL}
	ch, ts, err := s.Post(context.Background(), SlackMessage{Token: "xoxb-secret", Channel: "U1", Text: "hi", ThreadTS: "0.1"})
	if err != nil || ch != "D1" || ts != "1.2" || auth != "Bearer xoxb-secret" {
		t.Fatalf("%s %s %v %s", ch, ts, err, auth)
	}
	if got["unfurl_links"] != false || got["unfurl_media"] != false || got["thread_ts"] != "0.1" {
		t.Fatalf("body %+v", got)
	}
	_, _, err = s.Post(context.Background(), SlackMessage{Token: "xoxb-secret", Channel: "Ubad", Text: "hi"})
	if err == nil || err.Error() != "slack: channel_not_found" || strings.Contains(err.Error(), "xoxb") || threadGone(err) == false {
		t.Fatal(err)
	}
}

// The published schema and the notice must not drift.
func TestSchemaMatchesNotice(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "schemas", "attention_event.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Title      string
		Required   []string
		Properties map[string]struct {
			Const string
			Enum  []string
		}
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s.Title != Schema || s.Properties["schema"].Const != Schema || !reflect.DeepEqual(s.Properties["event"].Enum, Kinds) {
		t.Fatal("schema version or kinds drifted")
	}
	if got := s.Properties["source"].Enum; !reflect.DeepEqual(got, []string{SourceRun, SourceOrchestrator, SourceFollow, SourceSwarm, SourceSteer, SourceJob, SourceLegacyExpedite}) {
		t.Fatal(got)
	}
	n := sample(KindNeedsYou, "k").For("teammate")
	n.Client = true
	n.Links.Board = "b"
	raw, _ := json.Marshal(n)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	for k := range m {
		if _, ok := s.Properties[k]; !ok {
			t.Errorf("notice field %q missing from the schema", k)
		}
	}
	for _, k := range s.Required {
		if _, ok := m[k]; !ok {
			t.Errorf("required %q missing from a notice", k)
		}
	}
}

// A process that stops after Slack accepted a post, before the delivered
// checkpoint, must not post that notice again after a restart: a fresh
// emitter over the same outbox finds the send in flight under an expired
// lease and marks it uncertain instead of reposting.
func TestAcceptedPostCrashRestartNotReposted(t *testing.T) {
	var mu sync.Mutex
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		posts++
		mu.Unlock()
		w.Write([]byte(`{"ok":true,"channel":"Dfake","ts":"1.2"}`))
	}))
	defer srv.Close()
	root, state := t.TempDir(), t.TempDir()
	e, _, _ := newEmitter(t, root)
	api := SlackHTTP{Base: srv.URL}
	e.Post = posterFunc(func(m SlackMessage) (string, string, error) {
		if _, _, err := api.Post(context.Background(), m); err != nil {
			return "", "", err
		}
		panic("simulated crash after Slack accepted, before the delivered checkpoint")
	})
	n := sample(KindNeedsYou, "crash")
	if err := e.Emit(context.Background(), n, state); err == nil {
		t.Fatal("crash injection did not run")
	}
	// Restarted within the lease: still in flight, untouched.
	fresh, _, _ := newEmitter(t, root)
	fresh.Post = api
	if err := fresh.Emit(context.Background(), n, state); err != nil {
		t.Fatal(err)
	}
	// Restarted after the lease: uncertain, never reposted, and visible.
	fresh.Now = func() time.Time { return time.Now().Add(3 * time.Minute) }
	for range 2 {
		if err := fresh.Emit(context.Background(), n, state); err != nil {
			t.Fatal(err)
		}
		fresh.Flush(context.Background())
	}
	mu.Lock()
	got := posts
	mu.Unlock()
	if got != 1 {
		t.Fatalf("notice %s accepted %d times across restart; want 1", n.ID, got)
	}
	var rec Record
	if err := readJSON(fresh.recordPath(n.ID), &rec); err != nil || rec.Slack.Status != StatusUncertain || rec.Slack.Sending.IsZero() {
		t.Fatalf("record %+v %v", rec.Slack, err)
	}
	if line := StatusLine(state); !strings.Contains(line, "uncertain") {
		t.Fatalf("status line %q", line)
	}
	if d := fresh.Describe(); !strings.Contains(d, "1 uncertain") {
		t.Fatalf("describe %q", d)
	}
}

// A failure before the request is written (token, connection refused) is a
// definite miss and retries; one after it (a dropped answer) is uncertain
// and is not retried.
func TestDeliveryMissVersusUncertain(t *testing.T) {
	t.Run("token failure before send retries", func(t *testing.T) {
		root := t.TempDir()
		e, p, _ := newEmitter(t, root)
		calls := 0
		e.Token = func(context.Context, string) (string, error) {
			calls++
			if calls == 1 {
				return "", errors.New("vault locked")
			}
			return "xoxb-test", nil
		}
		n := sample(KindNeedsYou, "token")
		if err := e.Emit(context.Background(), n, ""); err == nil {
			t.Fatal("want the token error")
		}
		e.Now = func() time.Time { return time.Now().Add(2 * time.Minute) }
		if e.Flush(context.Background()) != 1 || len(p.posts) != 1 {
			t.Fatalf("posts=%d", len(p.posts))
		}
	})
	t.Run("connection refused retries", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		base := srv.URL
		srv.Close()
		root := t.TempDir()
		e, _, _ := newEmitter(t, root)
		e.Post = SlackHTTP{Base: base}
		n := sample(KindNeedsYou, "refused")
		if err := e.Emit(context.Background(), n, ""); err == nil {
			t.Fatal("want a connection error")
		}
		var rec Record
		if err := readJSON(e.recordPath(n.ID), &rec); err != nil || rec.Slack.Status != StatusQueued || rec.Slack.NextAttempt.IsZero() {
			t.Fatalf("record %+v %v", rec.Slack, err)
		}
	})
	t.Run("answer lost after send is uncertain", func(t *testing.T) {
		var mu sync.Mutex
		posts := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			posts++
			mu.Unlock()
			// Slack took the request; the answer never arrives intact.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("no hijacker")
				return
			}
			c, _, _ := hj.Hijack()
			c.Close()
		}))
		defer srv.Close()
		root := t.TempDir()
		e, _, _ := newEmitter(t, root)
		e.Post = SlackHTTP{Base: srv.URL}
		n := sample(KindNeedsYou, "dropped")
		if err := e.Emit(context.Background(), n, ""); err == nil {
			t.Fatal("want an error")
		}
		e.Now = func() time.Time { return time.Now().Add(2 * time.Hour) }
		e.Flush(context.Background())
		if err := e.Emit(context.Background(), n, ""); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		got := posts
		mu.Unlock()
		if got != 1 {
			t.Fatalf("posts=%d, want 1", got)
		}
		var rec Record
		if err := readJSON(e.recordPath(n.ID), &rec); err != nil || rec.Slack.Status != StatusUncertain {
			t.Fatalf("record %+v %v", rec.Slack, err)
		}
	})
}
