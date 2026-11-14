package peersview_test

import (
	"context"
	"fmt"
	"github.com/hev/kit/pkg/kitapi"
	"github.com/hev/kit/pkg/peersview"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type probeReader struct{}

func (probeReader) PeerPresence(context.Context) (kitapi.PresencePage, error) {
	return kitapi.PresencePage{}, nil
}
func (probeReader) PeerMessages(context.Context, url.Values) (kitapi.PeerMessagesPage, error) {
	return kitapi.PeerMessagesPage{Messages: []kitapi.PeerMessage{{PeerMessageIn: kitapi.PeerMessageIn{Body: `C:/Users/adam/private.txt file:///home/adam/private.txt http://127.1:8888 javascript:alert(1) </pre><script>alert(1)</script>`, Title: `"><img src=x onerror=alert(1)>`}}}, NextCursor: `javascript:alert(1)`}, nil
}
func TestReviewViewContexts(t *testing.T) {
	auth := func(*http.Request) (peersview.Principal, error) { return peersview.Principal{ID: "viewer"}, nil }
	h, e := peersview.NewHosted(probeReader{}, peersview.HostedOptions{BasePath: "/team/peers", Principal: auth})
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{"/team/peers/?machine=%22%20onfocus%3Dalert(1)", "//outside", "/team/peers/./", "/team/peers/../", "/team/peers/%2e%2e/", "/team/peers/%2f%2foutside"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		t.Log(p, w.Code, w.Header())
		if w.Header().Get("Location") != "" {
			t.Error("redirect")
		}
		if w.Code == 200 {
			t.Log(w.Body.String())
			if strings.Contains(w.Body.String(), "C:/Users/adam/private.txt") || strings.Contains(w.Body.String(), "file:///home/adam/private.txt") {
				t.Error("private path displayed")
			}
			if strings.Contains(w.Body.String(), "<script>") || strings.Contains(w.Body.String(), `href="javascript:`) {
				t.Error("active payload")
			}
		}
	}
}

// Every displayed text context must suppress paths, not merely HTML-escape.
type privacyReader struct{ value string }

func (r privacyReader) PeerPresence(context.Context) (kitapi.PresencePage, error) {
	s := kitapi.PeerSession{Name: r.value, Harness: r.value, Model: r.value, Project: r.value, Branch: r.value, State: r.value}
	return kitapi.PresencePage{Machines: []kitapi.PeerMachine{{PresenceIn: kitapi.PresenceIn{Host: r.value, OS: r.value, Version: r.value, Sessions: []kitapi.PeerSession{s}}}}}, nil
}
func (r privacyReader) PeerMessages(context.Context, url.Values) (kitapi.PeerMessagesPage, error) {
	return kitapi.PeerMessagesPage{Messages: []kitapi.PeerMessage{{PeerMessageIn: kitapi.PeerMessageIn{Sender: r.value, Recipients: []string{r.value}, Kind: r.value, ChatID: r.value, Title: r.value, Body: r.value}}}}, nil
}
func TestReviewPrivacyAllDisplayedFields(t *testing.T) {
	paths := []string{
		"/home/adam/private.txt", "C:/Users/adam/private.txt", `C:\Users\adam\private.txt`,
		"file:/home/adam/private.txt", "file://host/home/adam/private.txt", "file:///home/adam/private.txt", "file:////host/home/adam/private.txt", "file:home/adam/private.txt",
		`\\server\share\private.txt`, "//server/share/private.txt", "~/private.txt", "~adam/private.txt",
		"$HOME/private.txt", "${HOME}/private.txt", "%USERPROFILE%/private.txt", "%HOMEPATH%/private.txt",
		"./private.txt", "../private.txt",
	}
	for _, p := range paths {
		for _, wrap := range []string{"%s", "[%s]", "(%s)", "'%s'", `"%s"`, "x:%s", "x,%s", "{%s}"} {
			v := fmt.Sprintf(wrap, p)
			t.Run(v, func(t *testing.T) {
				h, e := peersview.NewHosted(privacyReader{v}, peersview.HostedOptions{BasePath: "/peers", Principal: func(*http.Request) (peersview.Principal, error) { return peersview.Principal{ID: "viewer"}, nil }})
				if e != nil {
					t.Fatal(e)
				}
				q := url.Values{"machine": {v}, "session": {v}, "chat": {v}}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("GET", "/peers?"+q.Encode(), nil))
				if w.Code != 200 || strings.Contains(w.Body.String(), "private.txt") || strings.Contains(w.Body.String(), "adam") || strings.Contains(w.Body.String(), "server") {
					t.Fatalf("private path displayed: %s", w.Body.String())
				}
			})
		}
	}
}
