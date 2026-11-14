package peersview_test

import (
	"fmt"
	"github.com/hev/kit/pkg/peersview"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func deltaRender(t *testing.T, v string) string {
	t.Helper()
	h, e := peersview.NewHosted(privacyReader{v}, peersview.HostedOptions{BasePath: "/peers", Principal: func(*http.Request) (peersview.Principal, error) { return peersview.Principal{ID: "v"}, nil }})
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	q := url.Values{"machine": {v}, "session": {v}, "chat": {v}}
	h.ServeHTTP(w, httptest.NewRequest("GET", "/peers?"+q.Encode(), nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	return html.UnescapeString(w.Body.String())
}
func TestDeltaPrivacyVariants(t *testing.T) {
	for i, v := range []string{`z:/delta-private.txt`, `q:\delta-private.txt`, `p=./delta-private.txt`, `{"p":"../delta-private.txt"}`, `{"p":"FiLe://HOST/delta-private.txt"}`, `p=%USERPROFILE%\delta-private.txt`, `p=$HOME/delta-private.txt`, strings.Repeat("x", 100000) + `=C:/delta-private.txt`, `file%3Ahome%2Fdelta-private.txt`, `FILE%3a%2f%2fhost%2fdelta-private.txt`, `file%253Ahome%252Fdelta-private.txt`, `%252Fhome%252Fdelta-private.txt`, `C%253a%255cUsers%255cdelta-private.txt`, `%257E%252fdelta-private.txt`, `%2fhome%2Fdelta-private.txt`, `C%3A%5cdelta-private.txt`, `%7e%2Fdelta-private.txt`} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			body := deltaRender(t, v)
			n := strings.Count(body, "delta-private.txt")
			if n != 0 {
				t.Fatalf("private marker remains %d times; input %.140s", n, v)
			}
		})
	}
}
func TestDeltaOrdinaryTextDiagnostic(t *testing.T) {
	for _, v := range []string{"repo/file", "a/b", "https://example.com/docs", "12:30", "profile: ready", "a/b/c", "x=repo/file", "http://host/a/b?x=1", "ssh://git@host/repo", "file: notes", "abcC:/ordinary", `abcC:\ordinary`} {
		b := deltaRender(t, v)
		if n := strings.Count(b, v); n != 18 {
			t.Errorf("input=%q occurrences=%d want 18", v, n)
		}
	}
}
