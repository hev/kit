package redact_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hev/kit/pkg/archive"
	"github.com/hev/kit/pkg/redact"
)

func TestRowsAreScrubbedInPlace(t *testing.T) {
	s, err := redact.New(bytes.Repeat([]byte("s"), 32))
	if err != nil {
		t.Fatal(err)
	}
	secret := "ghp_" + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"
	sess := archive.SessionRow{Summary: "used " + secret, FirstPrompt: "token " + secret, FirstPromptShort: "t " + secret}
	block := archive.BlockRow{Text: "export T=" + secret, RequestText: secret}
	chunk := archive.Row{Text: "x " + secret}
	n := 0
	for _, c := range []redact.Counts{redact.Session(s, &sess), redact.Block(s, &block), redact.Chunk(s, &chunk)} {
		for _, v := range c {
			n += v
		}
	}
	if n != 6 {
		t.Fatalf("redactions = %d, want 6", n)
	}
	for _, f := range []string{sess.Summary, sess.FirstPrompt, sess.FirstPromptShort, block.Text, block.RequestText, chunk.Text} {
		if strings.Contains(f, secret) {
			t.Fatalf("secret survived: %q", f)
		}
	}
	if _, err := redact.New([]byte("short")); err == nil {
		t.Fatal("short salt accepted")
	}
}
