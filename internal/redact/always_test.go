package redact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAlwaysScrubsWhenCaptureRedactionIsOff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	salt := strings.Repeat("ab", 32)
	for name, text := range map[string]string{
		"off-with-salt": "[capture]\nredact = false\nredact_salt = \"" + salt + "\"\n",
		"off-no-salt":   "[capture]\nredact = false\n",
		"missing":       "",
	} {
		t.Run(name, func(t *testing.T) {
			if text == "" {
				_ = os.Remove(path)
			} else if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			s, err := loadAlwaysFile(path)
			if err != nil || s == nil {
				t.Fatalf("scrubber %v, %v", s, err)
			}
			out, counts := s.Text("token " + "xo" + "xb-1234567890-1234567890123-abcdefghijklmnopqrstuvwx here")
			if strings.Contains(out, "xoxb-") || len(counts) == 0 {
				t.Fatalf("not scrubbed: %q %v", out, counts)
			}
			if text != "" {
				after, _ := os.ReadFile(path)
				if string(after) != text {
					t.Fatal("LoadAlways must not rewrite the config")
				}
			}
		})
	}
}
