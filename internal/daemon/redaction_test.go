package daemon

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/redact"
)

func TestCyclePersistsRedactionCounts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, ".hev", "config.toml"))
	root := filepath.Join(home, ".claude", "projects", "project")
	os.MkdirAll(root, 0700)
	secret := "tpuf_AbCdEfGhIjKlMnOpQrStUvWx01234567"
	raw := fmt.Sprintf(`{"type":"user","uuid":"u","sessionId":"s","message":{"role":"user","content":%q}}`, secret) + "\n"
	os.WriteFile(filepath.Join(root, "session.jsonl"), []byte(raw), 0600)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/query") {
			io.WriteString(w, `{"rows":[]}`)
		} else {
			io.WriteString(w, `{"rows_upserted":1}`)
		}
	}))
	defer srv.Close()
	state := &index.State{Units: map[string]string{}}
	status := runIndexCycle(layer.New(srv.URL, "key", "ns", ""), state, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if status.LastError != "" || status.Redactions["turbopuffer"] != 1 {
		t.Fatalf("status=%+v", status)
	}
	if err := writeStatus(status); err != nil {
		t.Fatal(err)
	}
	saved, err := ReadStatus()
	if err != nil || saved.Redactions["turbopuffer"] != 1 {
		t.Fatalf("persisted=%+v err=%v", saved, err)
	}
	skipped := runIndexCycle(layer.New(srv.URL, "key", "ns", ""), state, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if len(skipped.Redactions) != 0 {
		t.Fatal("unchanged unit counted again")
	}
}
func TestConfigWritersPreserveRedactionIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", path)
	os.WriteFile(path, []byte("[[buckets]]\nname='one'\n[capture]\nredact=true\n"), 0600)
	scrubber, err := redact.Load()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := scrubber.Text("sk-ABCDEFGHIJKLMNOP0123456789")
	if _, err := SetActiveBucket("one"); err != nil {
		t.Fatal(err)
	}
	after, err := redact.Load()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := after.Text("sk-ABCDEFGHIJKLMNOP0123456789")
	if got != want {
		t.Fatal("bucket selection rotated salt")
	}
	if _, err := WriteDefaultConfig(true); err != nil {
		t.Fatal(err)
	}
	after, err = redact.Load()
	if err != nil {
		t.Fatal(err)
	}
	got, _ = after.Text("sk-ABCDEFGHIJKLMNOP0123456789")
	if got != want {
		t.Fatal("default config reset rotated salt")
	}
	cfg, err := LoadConfig()
	if err != nil || !cfg.CaptureRedact {
		t.Fatalf("default=%v err=%v", cfg, err)
	}
	os.WriteFile(path, []byte("[capture]\nredact=false\n"), 0600)
	cfg, err = LoadConfig()
	if err != nil || cfg.CaptureRedact {
		t.Fatalf("optout=%v err=%v", cfg, err)
	}
}
