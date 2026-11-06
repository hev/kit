//go:build !windows

package peers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
)

func TestHEVDRunInboxLiveAndEnded(t *testing.T) {
	e, _ := fixture(t)
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := filepath.Join(bin, "tmux")
	dir := filepath.Join(dispatch.Root(), "example")
	os.MkdirAll(filepath.Join(dir, "wt"), 0700)
	dispatch.WriteJSON(filepath.Join(dir, "meta.json"), dispatch.Meta{Order: dispatch.Order{Name: "example"}})
	for i, live := range []bool{true, true, false} {
		script := "#!/bin/sh\nexit 0\n"
		want := "held for ended run"
		if live {
			script = "#!/bin/sh\necho d-example\n"
			want = "delivered to run inbox"
		}
		if i == 1 {
			want = "queued for next round"
			dispatch.WriteJSON(filepath.Join(dir, "schedule.json"), dispatch.Schedule{Rounds: 1, Next: time.Date(2026, 1, 1, 12, 34, 0, 0, time.UTC)})
		}
		os.WriteFile(path, []byte(script), 0700)
		b, _ := json.Marshal(enqueue{Target: "example", From: "remote/sender", Message: want})
		req := httptest.NewRequest("POST", "/v1/peers/enqueue", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		e.Handler("secret").ServeHTTP(w, req)
		var d delivery
		json.Unmarshal(w.Body.Bytes(), &d)
		if i == 1 && !strings.Contains(d.Detail, "queued for next round at 12:34") {
			t.Fatal(d)
		}
		if w.Code != 200 || d.Status != want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "inbox.md"))
	if err != nil || !strings.Contains(string(b), "delivered to run inbox") || !strings.Contains(string(b), "held for ended run") {
		t.Fatal(string(b), err)
	}
}

func TestHEVDFallbackNoticeOnce(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\ncat >/dev/null\nprintf '{\"log\":\"\"}\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	var notices bytes.Buffer
	tr := HEVDTransport{Hosts: map[string]Endpoint{"remote": {URL: "http://" + addr, Token: "secret"}}, Fallback: SSHTransport{}}
	err = tr.Execute(context.Background(), "remote", []string{"send", "desk", "body", "--from", "local/sender"}, strings.NewReader(""), io.Discard, &notices)
	if err != nil || strings.Count(notices.String(), "using ssh") != 1 || strings.Count(notices.String(), "\n") != 1 {
		t.Fatal(err, notices.String())
	}
}
