package asks_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/serve"
	"github.com/hev/kit/internal/tracefixture"
	"github.com/hev/kit/pkg/asks"
)

func fixture(name, id string) *serve.MemoryReader {
	now := time.Now().UnixMilli()
	return &serve.MemoryReader{Spans: []asks.SpanRow{
		{ID: "a1a1a1a1a1a1a1a1", Kind: "ask", TraceID: id, Run: name, RunKey: name, Publisher: "worker", Start: now, AskKind: "intake", Action: "dispatch", Text: name, Caller: `{"session":"host","sender":"Ada"}`},
		{ID: "b1b1b1b1b1b1b1b1", Kind: "attempt", TraceID: id, Run: name, RunKey: name, Publisher: "worker", Start: now, Attempt: 1, Revision: 1, Title: name, Repo: "/home/operator/project", ContextRecipe: "/home/operator/private.json", ParentSpanID: "a1a1a1a1a1a1a1a1", SessionID: name, Result: 1, Outcome: "done", End: now + 10},
	}, Sessions: []asks.SessionRow{{ID: name, SessionID: name, TraceID: id, Summary: name, End: now + 10}}}
}
func hosted(t *testing.T, r asks.Reader, principal func(*http.Request) (asks.Principal, error)) http.Handler {
	t.Helper()
	h, e := asks.NewHosted(r, asks.HostedOptions{BasePath: "/team/asks", TracesPath: "/team/traces", Principal: principal})
	if e != nil {
		t.Fatal(e)
	}
	return h
}
func ada(*http.Request) (asks.Principal, error) { return asks.Principal{ID: "Ada"}, nil }
func get(h http.Handler, p string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
	return w
}
func TestHostedAsksTeamIsolationAndPrincipal(t *testing.T) {
	a := hosted(t, fixture("private A", "11111111111111111111111111111111"), ada)
	b := hosted(t, fixture("private B", "22222222222222222222222222222222"), ada)
	for _, tc := range []struct {
		h          http.Handler
		own, other string
	}{{a, "private A", "private B"}, {b, "private B", "private A"}} {
		w := get(tc.h, "/team/asks/api/asks?window=all&author=forged&team=other")
		if w.Code != 200 || !strings.Contains(w.Body.String(), tc.own) || strings.Contains(w.Body.String(), tc.other) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := get(b, "/team/asks/api/asks/11111111111111111111111111111111"); w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	denied := hosted(t, fixture("private", "11111111111111111111111111111111"), func(*http.Request) (asks.Principal, error) { return asks.Principal{}, errors.New("invalid") })
	for _, p := range []string{"/", "/api/asks", "/ui/theme.css"} {
		w := get(denied, "/team/asks"+p+"?principal=Ada")
		if w.Code != 401 || strings.Contains(w.Body.String(), "private") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestHostedAsksReadOnlyBasePathAndSafeOutput(t *testing.T) {
	r := fixture("private A", "11111111111111111111111111111111")
	h := hosted(t, r, ada)
	for _, p := range []string{"/", "/api/asks?window=all", "/api/asks/11111111111111111111111111111111", "/ui/theme.css", "/ui/list-select.mjs"} {
		w := get(h, "/team/asks"+p)
		if w.Code != 200 {
			t.Fatal(p, w.Code, w.Body.String())
		}
		for _, bad := range []string{"localhost", "127.0.0.1", "/home/", "/Users/", "wire-secret-team", "hk_secret", "tpuf_"} {
			if strings.Contains(w.Body.String(), bad) {
				t.Fatal(p, "leaked", bad)
			}
		}
	}
	w := get(h, "/team/asks/")
	for _, s := range []string{`href="/team/asks/ui/theme.css"`, `url="/team/asks"+url`, `import('/team/asks/ui/list-select.mjs')`, `return "/team/traces/"+'?session='`} {
		if !strings.Contains(w.Body.String(), s) {
			t.Fatal("missing prefix", s)
		}
	}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		for _, p := range []string{"/", "/api/asks", "/api/asks/11111111111111111111111111111111"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(method, "/team/asks"+p, strings.NewReader(`{"principal":"forged"}`)))
			if w.Code != 405 {
				t.Fatal(method, p, w.Code)
			}
		}
	}
	if w := get(h, "/api/asks"); w.Code != 404 {
		t.Fatal("prefix bypass", w.Code)
	}
}

type failingReader struct{ *serve.MemoryReader }

func (r failingReader) ListSessionRows(int, any) ([]asks.SessionRow, error) {
	return nil, errors.New("/home/private hk_secret wire-secret-team http://localhost:1234")
}
func TestHostedAsksHidesBackendErrors(t *testing.T) {
	h := hosted(t, failingReader{fixture("private", "11111111111111111111111111111111")}, ada)
	w := get(h, "/team/asks/api/asks?window=all")
	if w.Code != 502 || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "hk_secret") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestHostedAsksIgnoresLocalDispatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if _, err := tracefixture.Write(home, time.Now()); err != nil {
		t.Fatal(err)
	}
	h := hosted(t, fixture("private A", "11111111111111111111111111111111"), ada)
	w := get(h, "/team/asks/api/asks?window=all&pretrace=1")
	if w.Code != 200 || strings.Contains(w.Body.String(), tracefixture.OrderNav) {
		t.Fatal(w.Code, w.Body.String())
	}
}
