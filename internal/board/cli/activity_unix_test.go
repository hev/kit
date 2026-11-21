//go:build !windows

package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/board/server"
	"github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/hooks"
)

func TestBoardPostActivityHook(t *testing.T) {
	const body = "token ghp_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"
	for _, mode := range []string{"capture", "off", "client", "hang", "fail", "rejected", "reply", "reply-client"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			t.Setenv("PATH", dir)
			t.Setenv("HEV_HOOKS", "")
			t.Setenv("HEV_CONFIG", filepath.Join(dir, "config.toml"))
			capture := filepath.Join(dir, "capture")
			t.Setenv("HOOK_CAPTURE", capture)
			reply := mode == "reply" || mode == "reply-client"
			mount := ""
			if reply {
				mount = "/engineering"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				request := strings.TrimPrefix(r.URL.RequestURI(), mount)
				if mount != "" && request == r.URL.RequestURI() {
					t.Errorf("request missed the board mount: %s", r.URL)
				}
				switch {
				case mode == "rejected":
					http.Error(w, `{"error":"rejected"}`, http.StatusForbidden)
				case r.Method == "GET" && request == "/api/threads/42?peek=1":
					_, _ = w.Write([]byte(`{"thread":{"id":42,"board":"watercooler","title":"Recorded title","tags":["test"]}}`))
				case r.Method == "POST" && request == "/api/threads/42/posts":
					_, _ = w.Write([]byte(`{"id":"p:42:2","n":2,"author":"recorded","body":"` + body + `"}`))
				case r.Method == "POST" && r.URL.Path == "/api/threads":
					_, _ = w.Write([]byte(`{"id":42,"board":"watercooler","title":"Recorded title","tags":["test"],"author":"recorded"}`))
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			t.Setenv("FACTORY_BOARD_URL", server.URL+mount)
			script := "#!/bin/sh\nprintf '%s %s %s\\n' \"$1\" \"$HEV_HOOK_EVENT\" \"$HEV_HOOK_HARNESS\" > \"$HOOK_CAPTURE\"\n/bin/cat >> \"$HOOK_CAPTURE\"\n"
			// A realistic budget: a forked shell handler can take well over
			// 150ms under a loaded full-suite run. Only the hang case needs
			// the budget to expire, and it gets a short one.
			config := "[hooks]\ntimeout = \"10s\"\n"
			switch mode {
			case "off":
				t.Setenv("HEV_HOOKS", "off")
			case "client", "reply-client":
				config += "[notify]\nclient = true\n"
			case "hang":
				config = "[hooks]\ntimeout = \"300ms\"\n"
				script = "#!/bin/sh\nexec /bin/sleep 60\n"
			case "fail":
				script = "#!/bin/sh\nexit 23\n"
			}
			for path, content := range map[string]string{filepath.Join(dir, "hev-hook-capture"): script, os.Getenv("HEV_CONFIG"): config} {
				if err := os.WriteFile(path, []byte(content), 0700); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"post", "--board", "watercooler", "--title", "Test post", "-m", body}
			entryURL, kind, id := server.URL+"/t/42", "post", "42"
			if reply {
				args = []string{"reply", "42", "-m", body, "--json"}
				entryURL, kind, id = entryURL+"#p2", "reply", "p:42:2"
			}
			start := time.Now()
			out, err := run(t, args...)
			if (err != nil) != (mode == "rejected") || (mode == "hang" && time.Since(start) > 10*time.Second) {
				t.Fatalf("board: %q, %v after %s", out, err, time.Since(start))
			}
			hookDir := filepath.Join(dir, ".hev", "hooks")
			logPath := filepath.Join(hookDir, "events.jsonl")
			if mode == "off" || mode == "rejected" {
				for _, path := range []string{capture, logPath} {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("unexpected hook output %s: %v", path, err)
					}
				}
				return
			}
			raw, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if reply {
				var logged hooks.Event
				decode(t, string(raw), &logged)
				want := "board-reply"
				if logged.Event != want || logged.Data["thread_id"] != float64(42) || logged.Data["body"] != nil {
					t.Fatalf("unexpected hook log: %s", raw)
				}
				raw, err = os.ReadFile(capture)
				if err != nil {
					t.Fatal(err)
				}
				header, payload, _ := strings.Cut(string(raw), "\n")
				var event hooks.Event
				decode(t, payload, &event)
				if header != want+" "+want+" kit" || event.Schema != hooks.Schema || event.ID != logged.ID {
					t.Fatalf("unexpected handler event: %s", raw)
				}
				if reply && (event.Data["board_origin"] != server.URL+mount ||
					logged.Data["board_origin"] != event.Data["board_origin"] || event.Data["entry_id"] != id || event.Data["author"] != "recorded" ||
					event.Data["board"] != "watercooler" || event.Data["body_bytes"] != float64(len(body))) {
					t.Fatalf("unexpected reply metadata: %s", raw)
				}
				if mode == "reply" && event.Data["body"] == nil || mode != "reply" && event.Data["body"] != nil ||
					strings.Contains(string(raw), "AbCdEfGh") {
					t.Fatalf("unexpected reply body: %s", raw)
				}
				return
			}
			var record map[string]any
			decode(t, string(raw), &record)
			if _, ok := record["body"]; ok || record["body_bytes"] != float64(len(body)) {
				t.Fatalf("unexpected hook log: %s", raw)
			}
			if mode == "hang" || mode == "fail" {
				warning, err := os.ReadFile(filepath.Join(hookDir, "warnings.log"))
				want := map[string]string{"hang": "timed out", "fail": "exit status 23"}[mode]
				if err != nil || !strings.Contains(string(warning), want) {
					t.Fatalf("warning: %s, %v", warning, err)
				}
				return
			}
			raw, err = os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			header, payload, _ := strings.Cut(string(raw), "\n")
			var ev hooks.BoardPost
			decode(t, payload, &ev)
			if header != "board-post board-post kit" || ev.Schema != "hev.board.post/v1" || ev.Event != "board-post" ||
				ev.ID != id || ev.Kind != kind || ev.Author != "recorded" || ev.Title != "Recorded title" ||
				ev.Board != "watercooler" || strings.Join(ev.Tags, ",") != "test" || ev.URL != entryURL ||
				ev.BodyBytes != len(body) || (mode == "reply" && ev.ThreadID != 42) {
				t.Fatalf("unexpected hook capture: %s", raw)
			}
			if mode == "client" {
				if ev.Body != nil {
					t.Fatalf("client content reached the handler: %s", raw)
				}
			} else if ev.Body == nil || !strings.HasPrefix(*ev.Body, "token ") || strings.Contains(*ev.Body, "AbCdEfGh") {
				t.Fatalf("missing or unredacted body: %s", raw)
			}
		})
	}
}

func TestBoardPinHookOnlyOnPinTransition(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "hooks.toml"))
	t.Setenv("HEV_HOOKS", "")
	t.Setenv("PATH", home)
	st, err := store.NewFiles(filepath.Join(home, "board"))
	if err != nil {
		t.Fatal(err)
	}
	thread, err := st.CreateThread(context.Background(), store.NewThread{
		Board: "watercooler", Title: "Thread to pin", Body: "example", Poster: store.Poster{Author: "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	board, err := server.New(st)
	if err != nil {
		t.Fatal(err)
	}
	httpBoard := httptest.NewServer(board.Handler())
	defer httpBoard.Close()
	t.Setenv("FACTORY_BOARD_URL", httpBoard.URL)
	logPath := filepath.Join(home, ".hev", "hooks", "events.jsonl")
	id := strconv.FormatInt(thread.ID, 10)
	for _, tc := range []struct {
		cmd    string
		count  int
		pinned bool
	}{
		{"pin", 1, true}, {"pin", 1, true}, {"unpin", 1, false},
		{"unpin", 1, false}, {"pin", 2, true},
	} {
		if _, err := run(t, tc.cmd, id); err != nil {
			t.Fatal(err)
		}
		got, err := st.Thread(context.Background(), thread.ID)
		if err != nil || got.Pinned != tc.pinned {
			t.Fatalf("%s: %+v (%v)", tc.cmd, got, err)
		}
		raw, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		if len(lines) != tc.count {
			t.Fatalf("%s: expected %d events, got %s", tc.cmd, tc.count, raw)
		}
		var event hooks.Event
		decode(t, lines[len(lines)-1], &event)
		if event.Event != "board-pin" || event.Data["thread_id"] != float64(thread.ID) {
			t.Fatalf("unexpected event: %+v", event)
		}
	}
	if _, err := run(t, "pin", "9999"); err == nil {
		t.Fatal("pin succeeded for nonexistent thread")
	}
	raw, err := os.ReadFile(logPath)
	if err != nil || strings.Count(string(raw), "\n") != 2 {
		t.Fatalf("failed pin changed event log: %s (%v)", raw, err)
	}
}
