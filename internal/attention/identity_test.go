package attention

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// fakeKit is a kit service that answers /v1/whoami for one key.
type fakeKit struct {
	mu     sync.Mutex
	hits   int
	status int
	*httptest.Server
}

func newFakeKit(t *testing.T, key, id, name string) *fakeKit {
	t.Helper()
	f := &fakeKit{status: 200}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits++
		status := f.status
		f.mu.Unlock()
		if r.Method != "GET" || r.URL.Path != "/v1/whoami" || r.Header.Get("Authorization") != "Bearer "+key {
			http.Error(w, `{"error":"unauthorized"}`, 401)
			return
		}
		if status != 200 {
			http.Error(w, `{"error":"down"}`, status)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"key_id": "k1", "team": map[string]string{"id": "t1", "slug": "hev"}, "author": map[string]string{"id": id, "name": name}})
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeKit) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits
}

// signIn points this test's config at body and clears every cached answer.
func signIn(t *testing.T, body string) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_CONFIG", cfg)
	t.Setenv("HEV_API_KEY", "")
	t.Setenv("HEV_KIT_SERVICE", "")
	ResetIdentity()
	t.Cleanup(ResetIdentity)
}

// A signed-in host names a local requester by its Clerk user ID from
// /v1/whoami; a teammate stays the teammate.
func TestRequesterSignedIn(t *testing.T) {
	kit := newFakeKit(t, "kit_key_1", "user_2pQx7Lr0", "Adam")
	signIn(t, "[kit]\nservice = \""+kit.URL+"\"\nkey = \"kit_key_1\"\n")
	if got := Requester("local:hev"); got != (Person{Person: "user_2pQx7Lr0", Name: "Adam"}) {
		t.Fatalf("%+v", got)
	}
	if got := Requester(""); got.Person != "user_2pQx7Lr0" {
		t.Fatalf("this account: %+v", got)
	}
	if got := Requester("bo"); got != (Person{Person: "bo", Name: "bo"}) {
		t.Fatalf("teammate: %+v", got)
	}
	if kit.count() != 1 {
		t.Fatalf("whoami is cached per process: %d calls", kit.count())
	}
}

// HEV_API_KEY signs a host in without a [kit] key.
func TestRequesterEnvKey(t *testing.T) {
	kit := newFakeKit(t, "env_key", "user_env", "Env")
	signIn(t, "")
	t.Setenv("HEV_API_KEY", "env_key")
	t.Setenv("HEV_KIT_SERVICE", kit.URL)
	if got := Requester("local:hev"); got.Person != "user_env" {
		t.Fatalf("%+v", got)
	}
}

// A logged-out host keeps the sender and never calls the service.
func TestRequesterLoggedOut(t *testing.T) {
	kit := newFakeKit(t, "k", "user_x", "X")
	signIn(t, "[kit]\nservice = \""+kit.URL+"\"\n[notify]\nclient = false\n")
	if got := Requester("local:hev"); got != (Person{Person: "local:hev", Name: "local:hev"}) {
		t.Fatalf("%+v", got)
	}
	if got := Requester(""); got.Person != LocalSender() {
		t.Fatalf("%+v", got)
	}
	if kit.count() != 0 {
		t.Fatalf("logged out host called whoami %d times", kit.count())
	}
}

// When whoami fails, the author the last sign-in recorded stands in; with
// none recorded the sender does.
func TestRequesterWhoamiDown(t *testing.T) {
	kit := newFakeKit(t, "k", "user_live", "Live")
	kit.status = 503
	signIn(t, "[kit]\napi = \""+kit.URL+"\"\nkey = \"k\"\nauthor_id = \"user_saved\"\nauthor_name = \"Saved\"\n")
	if got := Requester("local:hev"); got != (Person{Person: "user_saved", Name: "Saved"}) {
		t.Fatalf("%+v", got)
	}
	signIn(t, "[kit]\napi = \""+kit.URL+"\"\nkey = \"k\"\n")
	if got := Requester("local:hev"); got.Person != "local:hev" {
		t.Fatalf("%+v", got)
	}
}

// Every emitted notice and its hook event say who it is for; the addressee
// defaults to the requester and an explicit one is kept.
func TestEmittedNoticeNamesRequesterAndAddressee(t *testing.T) {
	kit := newFakeKit(t, "k", "user_adam", "Adam")
	signIn(t, "[kit]\napi = \""+kit.URL+"\"\nkey = \"k\"\n")
	e, p, hooked := newEmitter(t, t.TempDir())
	ctx := context.Background()

	if err := e.Emit(ctx, sample(KindNeedsYou, "for").For("local:hev"), ""); err != nil {
		t.Fatal(err)
	}
	bare := sample(KindNeedsYou, "bare") // built without For: this account
	if err := e.Emit(ctx, bare, ""); err != nil {
		t.Fatal(err)
	}
	roles := sample(KindNeedsYou, "roles")
	roles.Addressee = []string{"role:oncall", "user_bo"}
	if err := e.Emit(ctx, roles.For("bo"), ""); err != nil {
		t.Fatal(err)
	}
	if len(*hooked) != 3 {
		t.Fatalf("hooks %v", *hooked)
	}
	want := []struct {
		by Person
		to []string
	}{
		{Person{"user_adam", "Adam"}, []string{"user_adam"}},
		{Person{"user_adam", "Adam"}, []string{"user_adam"}},
		{Person{"bo", "bo"}, []string{"role:oncall", "user_bo"}},
	}
	for i, h := range *hooked {
		_, payload, _ := strings.Cut(h, " ")
		var n Notice
		if err := json.Unmarshal([]byte(payload), &n); err != nil {
			t.Fatal(err)
		}
		if n.RequestedBy == nil || *n.RequestedBy != want[i].by || !reflect.DeepEqual(n.Addressee, want[i].to) {
			t.Fatalf("hook %d: %+v %v", i, n.RequestedBy, n.Addressee)
		}
		var rec Record
		if err := readJSON(e.recordPath(n.ID), &rec); err != nil || rec.Notice.RequestedBy == nil || *rec.Notice.RequestedBy != want[i].by {
			t.Fatalf("outbox record %d: %+v %v", i, rec.Notice.RequestedBy, err)
		}
	}
	// Delivery is unchanged: one post each, first to the configured channel,
	// and no requester text.
	if len(p.posts) != 3 || p.posts[0].Channel != "U0TEST" {
		t.Fatalf("posts %+v", p.posts)
	}
	for _, m := range p.posts {
		if strings.Contains(m.Text, "user_") || strings.Contains(m.Text, "role:") {
			t.Fatalf("%+v", m)
		}
	}
}

// Pulled notices are bounded like every other field.
func TestCleanBoundsRequester(t *testing.T) {
	n := sample(KindNeedsYou, "k")
	n.RequestedBy = &Person{Person: strings.Repeat("p", 500), Name: "a\nb"}
	for i := 0; i < 40; i++ {
		n.Addressee = append(n.Addressee, "x")
	}
	n.Addressee = append(n.Addressee, "")
	c := n.Clean()
	if len([]rune(c.RequestedBy.Person)) != personLimit || c.RequestedBy.Name != "a b" || len(c.Addressee) != addresseeLimit {
		t.Fatalf("%+v %d", c.RequestedBy, len(c.Addressee))
	}
	if len(n.RequestedBy.Person) != 500 {
		t.Fatal("Clean changed its input")
	}
}
