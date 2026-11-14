package peersview_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hev/kit/pkg/kitapi"
	"github.com/hev/kit/pkg/peersview"
)

type reader struct {
	q    url.Values
	fail bool
}

func (r *reader) PeerPresence(context.Context) (kitapi.PresencePage, error) {
	if r.fail {
		return kitapi.PresencePage{}, errors.New("/home/private/key hk_secret http://localhost:8888")
	}
	return kitapi.PresencePage{Machines: []kitapi.PeerMachine{{PresenceIn: kitapi.PresenceIn{Host: "machine", Sessions: []kitapi.PeerSession{{Project: "/home/private/repo", Name: "<script>bad()</script>"}}}}}}, nil
}
func (r *reader) PeerMessages(_ context.Context, q url.Values) (kitapi.PeerMessagesPage, error) {
	r.q = q
	return kitapi.PeerMessagesPage{Messages: []kitapi.PeerMessage{{PeerMessageIn: kitapi.PeerMessageIn{Body: "<script>alert(1)</script> /home/private/token http://localhost:9999 hk_secret /etc/passwd ~/work/repo C:\\private\\token", Sender: "machine/session"}}}, NextCursor: "opaque"}, nil
}
func TestHostedPeersEscapesScriptAndHasNoWriteRoute(t *testing.T) {
	r := &reader{}
	h, e := peersview.NewHosted(r, peersview.HostedOptions{BasePath: "/team/peers", Principal: func(*http.Request) (peersview.Principal, error) { return peersview.Principal{ID: "viewer"}, nil }})
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/team/peers/?machine=machine&session=machine%2Fsession&chat=thread&limit=1", nil))
	body := w.Body.String()
	if w.Code != 200 || strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt;") || strings.Contains(body, "/home/private") || strings.Contains(body, "localhost") || strings.Contains(body, "hk_secret") || strings.Contains(body, "/etc/passwd") || strings.Contains(body, "~/work/repo") || strings.Contains(body, "private\\token") {
		t.Fatal(w.Code, body)
	}
	if !strings.Contains(body, "Older messages") || r.q.Get("session") != "machine/session" || r.q.Get("chat") != "thread" {
		t.Fatal(body, r.q)
	}
	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		for _, path := range []string{"/team/peers/", "/team/peers/send", "/team/peers/messages"} {
			w = httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader("message=bad")))
			if w.Code != 405 {
				t.Fatalf("%s %s: %d", method, path, w.Code)
			}
		}
	}
	r.fail = true
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/team/peers/", nil))
	if w.Code != 502 || strings.Contains(w.Body.String(), "private") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestHostedPeersRequiresPrincipalAndRejectsBadBase(t *testing.T) {
	r := &reader{}
	for _, base := range []string{"http://localhost", "/../key", "/x?key=a", "/x%2F.."} {
		if _, e := peersview.NewHosted(r, peersview.HostedOptions{BasePath: base, Principal: func(*http.Request) (peersview.Principal, error) { return peersview.Principal{}, nil }}); e == nil {
			t.Fatal(base)
		}
	}
	h, e := peersview.NewHosted(r, peersview.HostedOptions{Principal: func(*http.Request) (peersview.Principal, error) { return peersview.Principal{}, nil }})
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

type emptyReader struct{}

func (emptyReader) PeerPresence(context.Context) (kitapi.PresencePage, error) {
	return kitapi.PresencePage{}, nil
}
func (emptyReader) PeerMessages(context.Context, url.Values) (kitapi.PeerMessagesPage, error) {
	return kitapi.PeerMessagesPage{}, nil
}
func TestHostedPeersEmptyShellAndPrivateResponses(t *testing.T) {
	h, e := peersview.NewHosted(emptyReader{}, peersview.HostedOptions{BasePath: "/peers", Principal: func(*http.Request) (peersview.Principal, error) { return peersview.Principal{ID: "viewer"}, nil }})
	if e != nil {
		t.Fatal(e)
	}
	for _, target := range []string{"/peers", "/peers/"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "No live machines") || !strings.Contains(w.Body.String(), "No messages") || w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("Location") != "" {
			t.Fatal(w.Code, w.Body.String(), w.Header())
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/outside", nil))
	if w.Code != 404 || w.Header().Get("Location") != "" {
		t.Fatal(w.Code, w.Header())
	}
}
