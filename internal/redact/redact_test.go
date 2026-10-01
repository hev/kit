package redact

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hev/kit/internal/trace"
)

func testScrubber(t *testing.T) *Scrubber {
	t.Helper()
	s, err := New(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestCoverage(t *testing.T) {
	s := testScrubber(t)
	cases := map[string]string{
		"aws":                 "AKIAABCDEFGHIJKLMNOP",
		"github":              "ghp_" + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789",
		"github-fine-grained": "github_pat_" + strings.Repeat("AbCdEfGhIjKlMnOpQrStUvWxYz0123456789", 2) + "AbCdEfGhIjKlMn",
		"google":              "AIza" + "AbCdEfGhIjKlMnOpQrStUvWxYz012345678",
		"slack":               strings.Join([]string{"xoxb", "123456789012", "123456789012", "AbCdEfGhIjKlMnOpQrStUvWx"}, "-"),
		"slack-other":         "xoxe-1234567890-AbCdEfGhIjKlMnOpQrSt",
		"sk":                  "sk-proj-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789",
		"tpuf":                "tpuf_AbCdEfGhIjKlMnOpQrStUvWx01234567",
		"pem":                 "-----BEGIN RSA PRIVATE KEY-----\nAbCdEf12345\n-----END RSA PRIVATE KEY-----",
		"jwt":                 "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.AbCdEfGhIjKlMnOpQrStUvWxYz",
		"url":                 "https://user:correct-horse-battery@example.com/path",
		"authorization":       "Authorization: Bearer custom-token-value",
		"json-auth":           `"Authorization": "Basic dXNlcjpwYXNzd29yZA=="`,
		"entropy":             "UNUSUAL_KEY=AbCdEfGhIjKlMnOpQrStUvWx0123456789",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			out, c := s.Text(input)
			if out == input || len(c) != 1 || !strings.Contains(out, "[REDACTED:") {
				t.Fatalf("not scrubbed: %q counts=%v", out, c)
			}
			again, c2 := s.Text(out)
			if again != out || len(c2) != 0 {
				t.Fatalf("not idempotent: %q %v", again, c2)
			}
		})
	}
	for _, input := range []string{"hello world", "COUNT=12345", "VERSION=1.2.3", "VALUE=" + strings.Repeat("a", 32)} {
		out, _ := s.Text(input)
		if out != input {
			t.Errorf("benign changed: %q", out)
		}
	}
}
func TestFingerprintsAndOverlap(t *testing.T) {
	s := testScrubber(t)
	secret := "tpuf_AbCdEfGhIjKlMnOpQrStUvWx01234567"
	token, _ := s.Text(secret)
	assignment, c := s.Text("API_KEY=" + secret)
	if assignment != "API_KEY="+token || len(c) != 1 {
		t.Fatalf("overlap %q versus %q (%v)", assignment, token, c)
	}
	auth, _ := s.Text("Authorization: Bearer " + secret)
	if strings.Split(token, "#")[1] != strings.Split(auth, "#")[1] {
		t.Fatal("same secret has different fingerprint across rules")
	}
	other, err := New(bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	different, _ := other.Text(secret)
	if different == token {
		t.Fatal("salt does not key fingerprint")
	}
	s.rules = []rule{{id: "first", re: regexp.MustCompile(`abcdef`)}, {id: "second", re: regexp.MustCompile(`defghi`)}}
	out, c := s.Text("abcdefghi!")
	if strings.Contains(out, "ghi") || !strings.HasSuffix(out, "!") || c["first"] != 1 {
		t.Fatalf("partial overlap leaks: %q %v", out, c)
	}
}
func TestTurnsMetadataAndBlocks(t *testing.T) {
	s := testScrubber(t)
	secret := "sk-ABCDEFGHIJKLMNOP0123456789"
	turns := []trace.Turn{{Summary: secret, Workdir: "https://u:password@example.com", Blocks: []trace.Block{{Text: secret, ToolName: secret}}}}
	c := s.Turns(turns)
	if c["sk-token"] != 3 || strings.Contains(turns[0].Summary, secret) || strings.Contains(turns[0].Blocks[0].Text, secret) {
		t.Fatalf("turns=%+v counts=%v", turns, c)
	}
}
func TestConfigDefaultOptOutPersistenceAndConcurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "# keep comment\n[layer]\napi_key = 'keep-key'\n[capture]\nscan_interval = '1m'\n[unknown]\nvalue = 4\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	outputs := make(chan string, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := LoadFile(path)
			if err != nil {
				t.Error(err)
				return
			}
			out, _ := s.Text("sk-ABCDEFGHIJKLMNOP0123456789")
			outputs <- out
		}()
	}
	wg.Wait()
	close(outputs)
	expected := ""
	for out := range outputs {
		if expected != "" && out != expected {
			t.Fatal("concurrent first use rotated salt")
		}
		expected = out
	}
	b, _ := os.ReadFile(path)
	for _, part := range []string{"# keep comment", "api_key = 'keep-key'", "[unknown]", "redact_salt"} {
		if !strings.Contains(string(b), part) {
			t.Fatalf("lost %q", part)
		}
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%v", info.Mode())
	}
	s, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := s.Text("sk-ABCDEFGHIJKLMNOP0123456789")
	if out != expected {
		t.Fatal("fingerprint changed after reload")
	}
	opt := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(opt, []byte("[capture]\nredact = false\n"), 0600)
	disabled, err := LoadFile(opt)
	if err != nil || disabled != nil {
		t.Fatalf("opt-out: %v %v", disabled, err)
	}
	b, _ = os.ReadFile(opt)
	if strings.Contains(string(b), "salt") {
		t.Fatal("opt-out persisted salt")
	}
	for _, value := range []string{"'bad'", "''"} {
		os.WriteFile(opt, []byte("[capture]\nredact_salt = "+value+"\n"), 0600)
		if _, err := LoadFile(opt); err == nil {
			t.Fatal("invalid salt accepted")
		}
	}
	missing := filepath.Join(t.TempDir(), ".hev", "config.toml")
	if _, err := LoadFile(missing); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "config.toml")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(link); err == nil {
		t.Fatal("config symlink accepted")
	}
}

func TestExistingMarkerDoesNotExemptAdjacentSecret(t *testing.T) {
	s := testScrubber(t)
	marker := "[REDACTED:authorization#0123456789abcdef]"
	for _, input := range []string{"Authorization: Bearer raw-secret " + marker, "Authorization: Bearer " + marker + " raw-secret"} {
		out, c := s.Text(input)
		if strings.Contains(out, "raw-secret") || !strings.Contains(out, marker) || c["authorization"] != 1 {
			t.Fatalf("marker exemption leaked: %q %v", out, c)
		}
		again, c := s.Text(out)
		if again != out || len(c) != 0 {
			t.Fatalf("not idempotent: %q %v", again, c)
		}
	}
}
func TestDottedAndInlineCaptureConfig(t *testing.T) {
	for _, text := range []string{"capture.redact=true\nlayer.endpoint='http://localhost'\n", "capture={redact=true}\n", "# capture.redact documentation\n"} {
		path := filepath.Join(t.TempDir(), "config.toml")
		os.WriteFile(path, []byte(text), 0600)
		s, err := LoadFile(path)
		if err != nil || s == nil {
			t.Fatalf("config %q: %v", text, err)
		}
		if _, err := LoadFile(path); err != nil {
			t.Fatal(err)
		}
	}
}
