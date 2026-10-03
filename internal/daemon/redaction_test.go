package daemon

import (
	"encoding/json"
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
	rows := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			io.WriteString(w, `{}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if strings.HasSuffix(r.URL.Path, "/query") {
			if !strings.Contains(r.URL.Path, "-sessions/") {
				io.WriteString(w, `{"rows":[]}`)
				return
			}
			out := []any{}
			if f, ok := body["filters"].([]any); ok && f[0] == "id" && f[1] == "In" {
				for _, id := range f[2].([]any) {
					if row := rows[id.(string)]; row != nil {
						out = append(out, row)
					}
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"rows": out})
			return
		}
		if list, ok := body["upsert_rows"].([]any); ok && strings.HasSuffix(r.URL.Path, "-sessions") {
			for _, raw := range list {
				row := raw.(map[string]any)
				rows[row["id"].(string)] = row
			}
		}
		io.WriteString(w, `{"rows_upserted":1,"rows_affected":1}`)
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
