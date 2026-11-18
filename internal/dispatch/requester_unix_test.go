//go:build !windows

package dispatch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/attention"
)

// kitConfig points HEV_CONFIG at a [kit] sign-in (or none) and clears the
// cached /v1/whoami answer.
func kitConfig(t *testing.T, body string) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_CONFIG", cfg)
	t.Setenv("HEV_API_KEY", "")
	attention.ResetIdentity()
	t.Cleanup(attention.ResetIdentity)
}

func fakeWhoami(t *testing.T, id, name string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/whoami" || r.Header.Get("Authorization") != "Bearer kit_test_key" {
			http.Error(w, "no", 401)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"author": map[string]string{"id": id, "name": name}})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// requested_by is inherited through a rework, by a validator, by a run
// dispatched from inside the run's worktree, and by an orchestrator applied
// there and the runs it dispatches; each notice names it, as the Clerk user
// ID on a signed-in host and as the sender on a logged-out one.
func TestRequesterInheritance(t *testing.T) {
	t.Setenv("HEV_REQUESTED_BY", "")
	f := newTraceFlow(t)
	kitConfig(t, "")
	t.Setenv("HEV_ORCHESTRATOR", "")
	// The run was requested by another local account on this host.
	m := f.meta()
	m.Sender, m.RequestedBy = "local:adam", "local:adam"
	if err := WriteJSON(filepath.Join(f.dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}

	// Rework, asked by someone else, keeps who the run is for.
	f.finish()
	f.do(Request{Action: "rework", Note: "again", Meta: Meta{Sender: "local:someone", RequestedBy: "local:someone"}})
	if got := f.meta(); got.RequestedBy != "local:adam" || got.Sender != "local:adam" {
		t.Fatalf("rework: sender=%q requested_by=%q", got.Sender, got.RequestedBy)
	}

	// A validator takes the source run's requester.
	f.finish()
	wt := filepath.Join(f.dir, "wt")
	if _, err := command(wt, "git", "push", "origin", f.meta().Branch); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(os.Getenv("HEV_DISPATCH_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	caller := f.caller
	if _, err := validateAs(context.Background(), cfg, f.name, "v", false, "here", &caller); err != nil {
		t.Fatal(err)
	}
	var v Meta
	if err := readJSON(filepath.Join(Root(), f.name+"-v1", "meta.json"), &v); err != nil || v.RequestedBy != "local:adam" {
		t.Fatalf("validator requested_by %q: %v", v.RequestedBy, err)
	}

	// A run dispatched from inside the run's worktree is a child run.
	t.Chdir(wt)
	child := strings.Replace(strings.Replace(good, "name: example", "name: traced-child", 1), "repo: kit", "repo: "+filepath.ToSlash(f.meta().Repo), 1)
	child = strings.Replace(child, "host: buildbox", "host: here", 1)
	o, err := Parse(child)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rpcTransport(context.Background(), Host{Local: true}, Request{Action: "start", Name: o.Name, Order: child, Meta: Meta{Order: o}}); err != nil {
		t.Fatal(err)
	}
	var c Meta
	if err := readJSON(filepath.Join(Root(), "traced-child", "meta.json"), &c); err != nil || c.RequestedBy != "local:adam" || c.Sender != LocalSender() {
		t.Fatalf("child sender=%q requested_by=%q: %v", c.Sender, c.RequestedBy, err)
	}

	// An orchestrator applied from there works for the same person, and so
	// does what it dispatches.
	s, err := ApplyOrchestrator(orchestratorExample, false)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := orchestratorState(s.Name); st.RequestedBy != "local:adam" {
		t.Fatalf("orchestrator requested_by %q", st.RequestedBy)
	}
	t.Chdir(t.TempDir())
	if got := inheritedRequester(); got != LocalSender() {
		t.Fatalf("outside any run or orchestrator: %q", got)
	}
	// A worker anywhere carries its run's requester in HEV_REQUESTED_BY.
	t.Setenv("HEV_REQUESTED_BY", "bo")
	if got := inheritedRequester(); got != "bo" {
		t.Fatalf("worker child run: %q", got)
	}
	t.Setenv("HEV_REQUESTED_BY", "")
	t.Setenv("HEV_ORCHESTRATOR", s.Name)
	if got := inheritedRequester(); got != "local:adam" {
		t.Fatalf("orchestrator-dispatched run: %q", got)
	}
	t.Setenv("HEV_ORCHESTRATOR", "")

	// Logged out: notices name the sender, addressed to the requester.
	r := Result{End: time.Now().UTC(), Outcome: OutcomeBlocked, Reason: "no token"}
	n, ok := runAttention(c, r)
	if !ok || n.RequestedBy == nil || *n.RequestedBy != (attention.Person{Person: "local:adam", Name: "local:adam"}) || !reflect.DeepEqual(n.Addressee, []string{"local:adam"}) {
		t.Fatalf("logged out run notice %+v %v", n.RequestedBy, n.Addressee)
	}
	on := orchestratorAttention(s, attention.KindNeedsYou, "k", "waiting", "choose")
	if on == nil || on.RequestedBy == nil || on.RequestedBy.Person != "local:adam" {
		t.Fatalf("orchestrator notice %+v", on)
	}

	// Signed in: the same notices name the Clerk user.
	kitConfig(t, "[kit]\napi = \""+fakeWhoami(t, "user_2adam", "Adam")+"\"\nkey = \"kit_test_key\"\n")
	n, _ = runAttention(c, r)
	if *n.RequestedBy != (attention.Person{Person: "user_2adam", Name: "Adam"}) || !reflect.DeepEqual(n.Addressee, []string{"user_2adam"}) {
		t.Fatalf("signed in run notice %+v %v", n.RequestedBy, n.Addressee)
	}
	if on = orchestratorAttention(s, attention.KindNeedsYou, "k", "waiting", "choose"); on.RequestedBy.Person != "user_2adam" {
		t.Fatalf("signed in orchestrator notice %+v", on.RequestedBy)
	}
	// A teammate's run stays the teammate's on a signed-in host.
	tm := Meta{Order: Order{Name: "theirs", Host: "oven"}, Sender: "bo", RequestedBy: "bo", Start: time.Now().UTC()}
	if n, _ = runAttention(tm, r); n.RequestedBy.Person != "bo" || n.Addressee[0] != "bo" {
		t.Fatalf("teammate notice %+v", n.RequestedBy)
	}
}
