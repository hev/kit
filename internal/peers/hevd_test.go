package peers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHEVDStatusesAndOutboundOnly(t *testing.T) {
	for _, status := range []string{"delivered to session inbox", "delivered to run inbox", "queued for next round", "held for ended run", "failed"} {
		t.Run(status, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer secret" {
					t.Error("missing authentication")
				}
				var q enqueue
				if json.NewDecoder(r.Body).Decode(&q) != nil || q.Message != "body" || q.From != "sender" || q.ID != "retry" {
					t.Errorf("bad envelope: %+v", q)
				}
				json.NewEncoder(w).Encode(delivery{Status: status, Detail: "detail"})
			}))
			defer srv.Close()
			fallback := &recordingTransport{}
			tr := HEVDTransport{Hosts: map[string]Endpoint{"remote": {URL: srv.URL, Token: "secret"}}, Fallback: fallback}
			// Sender has no listener and no inbound route; only an outbound HTTP client.
			var out bytes.Buffer
			err := tr.Execute(context.Background(), "remote", []string{"send", "desk", "-", "--from", "sender", "--id", "retry"}, strings.NewReader("body"), &out, io.Discard)
			if (err != nil) != (status == "failed") {
				t.Fatal(err)
			}
			if status != "failed" && !strings.Contains(out.String(), status) {
				t.Fatal(out.String())
			}
			if fallback.host != "" {
				t.Fatal("unexpected SSH")
			}
		})
	}
}
func TestHEVDAuthFailureNoFallback(t *testing.T) {
	e, _ := fixture(t)
	srv := httptest.NewServer(e.Handler("correct"))
	defer srv.Close()
	fallback := &recordingTransport{}
	tr := HEVDTransport{Hosts: map[string]Endpoint{"remote": {URL: srv.URL, Token: "wrong"}}, Fallback: fallback}
	err := tr.Execute(context.Background(), "remote", []string{"send", "desk", "body", "--from", "sender"}, strings.NewReader(""), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "401") || fallback.host != "" {
		t.Fatal(err, fallback)
	}
}
func TestHEVDUnreachableFallbackReplaysStdin(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	fallback := &recordingTransport{}
	tr := HEVDTransport{Hosts: map[string]Endpoint{"remote": {URL: "http://" + addr, Token: "secret"}}, Fallback: fallback}
	if err := tr.Execute(context.Background(), "remote", []string{"send", "desk", "-", "--from", "sender"}, strings.NewReader("body"), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if fallback.host != "remote" || fallback.body != "body" {
		t.Fatal(fallback)
	}
}
func TestHEVDRealInboxAndRetry(t *testing.T) {
	e, call := fixture(t)
	srv := httptest.NewServer(e.Handler("secret"))
	defer srv.Close()
	tr := HEVDTransport{Hosts: map[string]Endpoint{"remote": {URL: srv.URL, Token: "secret"}}, Fallback: &recordingTransport{}}
	for range 2 {
		var out bytes.Buffer
		err := tr.Execute(context.Background(), "remote", []string{"send", "desk", "body", "--from", "sender", "--id", "retry"}, strings.NewReader(""), &out, io.Discard)
		if err != nil || !strings.Contains(out.String(), "delivered to session inbox") {
			t.Fatal(out.String(), err)
		}
	}
	s, err := call("", "inbox", "--count")
	if err != nil || strings.TrimSpace(s) != "1" {
		t.Fatal(s, err)
	}
}
func TestHEVDLostResponseNeverFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { c, _, _ := w.(http.Hijacker).Hijack(); c.Close() }))
	defer srv.Close()
	fallback := &recordingTransport{}
	tr := HEVDTransport{Hosts: map[string]Endpoint{"remote": {URL: srv.URL, Token: "secret"}}, Fallback: fallback}
	err := tr.Execute(context.Background(), "remote", []string{"send", "desk", "body", "--from", "sender"}, strings.NewReader(""), io.Discard, io.Discard)
	if err == nil || fallback.host != "" || !strings.Contains(err.Error(), "unknown") {
		t.Fatal(err, fallback)
	}
}

func TestHEVDNeverPastesIntoUnregisteredTerminal(t *testing.T) {
	e, _ := fixture(t)
	old := e.Command
	e.Command = func(ctx context.Context, name string, in io.Reader, args ...string) (string, error) {
		if name == "tmux" && args[0] == "list-panes" {
			return "desk\t%7\t700\tcodex\t/work\t\t\n", nil
		}
		if name == "tmux" && (args[0] == "load-buffer" || args[0] == "paste-buffer" || args[0] == "send-keys") {
			t.Fatal("HTTP triggered terminal input")
		}
		return old(ctx, name, in, args...)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/peers/enqueue", strings.NewReader(`{"target":"desk","from":"sender","message":"body"}`))
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	e.Handler("secret").ServeHTTP(w, req)
	var d delivery
	json.Unmarshal(w.Body.Bytes(), &d)
	if d.Status != "failed" || !strings.Contains(d.Detail, "registered inbox") {
		t.Fatal(w.Body.String())
	}
}

func TestHEVDRejectsInvalidRequests(t *testing.T) {
	e, _ := fixture(t)
	for _, body := range []string{`{"target":"other/desk","from":"sender","message":"body"}`, `{"target":"desk","from":"sender","message":"body","extra":true}`, `{} {}`, strings.Repeat("x", maxWire+1)} {
		req := httptest.NewRequest(http.MethodPost, "/v1/peers/enqueue", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		e.Handler("secret").ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatal(w.Code)
		}
	}
}

func TestHEVDEngineConfiguredSelection(t *testing.T) {
	e, call := fixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q enqueue
		json.NewDecoder(r.Body).Decode(&q)
		if q.Message != "multiple words" {
			t.Fatal(q)
		}
		json.NewEncoder(w).Encode(delivery{Status: "delivered to session inbox"})
	}))
	defer srv.Close()
	e.Transport = HEVDTransport{Hosts: map[string]Endpoint{"remote": {URL: srv.URL, Token: "secret"}}, Fallback: &recordingTransport{}}
	out, err := call("", "send", "remote/desk", "multiple", "words", "--from", "sender")
	if err != nil || !strings.Contains(out, "via hevd") {
		t.Fatal(out, err)
	}
}
