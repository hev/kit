package board_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hev/kit/pkg/board"
)

func TestHandlerStampsAuthenticatedPoster(t *testing.T) {
	st, err := board.OpenFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h, err := board.Handler(st, board.HandlerOptions{Author: func(r *http.Request) (board.Poster, error) {
		if r.Header.Get("Authorization") != "Bearer ok" {
			return board.Poster{}, errors.New("no")
		}
		return board.Poster{Author: "alice", Role: "human", Host: "laptop"}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	do := func(auth, method, path, body string) *http.Response {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	if resp := do("", "GET", "/api/threads", ""); resp.StatusCode != 401 {
		t.Fatalf("unauthenticated = %d", resp.StatusCode)
	}
	resp := do("ok", "POST", "/api/threads", `{"board":"floor","title":"hello","body":"first post","author":"mallory"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	var th board.Thread
	_ = json.NewDecoder(resp.Body).Decode(&th)
	if th.Author != "alice" {
		t.Fatalf("thread author = %q, want the caller", th.Author)
	}
	resp = do("ok", "POST", "/api/threads/1/posts", `{"body":"a reply","author":"mallory"}`)
	var p board.Post
	_ = json.NewDecoder(resp.Body).Decode(&p)
	if resp.StatusCode != 201 || p.Author != "alice" {
		t.Fatalf("reply = %d by %q", resp.StatusCode, p.Author)
	}
	resp = do("ok", "GET", "/api/threads/1", "")
	var got struct {
		Thread board.Thread
		Posts  []board.Post
	}
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if len(got.Posts) != 2 || got.Thread.Title != "hello" {
		t.Fatalf("thread = %+v", got)
	}
}

func TestHandlerReadOnly(t *testing.T) {
	st, _ := board.OpenFiles(t.TempDir())
	h, _ := board.Handler(st, board.HandlerOptions{ReadOnly: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/threads", strings.NewReader(`{}`)))
	if rec.Code != 403 {
		t.Fatalf("write on read-only = %d", rec.Code)
	}
}
