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

func TestKitAndLayerTokensBothSides(t *testing.T) {
	s, err := redact.New(bytes.Repeat([]byte("k"), 32))
	if err != nil {
		t.Fatal(err)
	}
	hevPayload := strings.Repeat("a1B2", 8)
	hvlPayload := strings.Repeat("Z9y8", 8)
	hkPayload := strings.Repeat("Ab9_-1", 7) + "A"
	skOrPayload := strings.Repeat("deadbeef", 8)

	hevToken := "hev_" + hevPayload
	hvlToken := "hvl_" + hvlPayload
	hkToken := "hk_" + hkPayload
	hk21 := "hk_abCD0123_sample-value"
	skToken := "sk-or-v1-" + skOrPayload

	// Refused/scrubbed side across Text, Session, Block, and Chunk (including %20/%3D/%26 and 32-char _-adjacent).
	for _, tc := range []struct {
		name, token, payload string
	}{
		{"hev", hevToken, hevPayload},
		{"hvl", hvlToken, hvlPayload},
		{"hk-43", hkToken, hkPayload},
		{"hk-21", hk21, "abCD0123_sample-value"},
		{"sk-or-v1", skToken, skOrPayload},
	} {
		t.Run("scrub/"+tc.name, func(t *testing.T) {
			clean, c := redact.Text(s, tc.token)
			if strings.Contains(clean, tc.token) || strings.Contains(clean, tc.payload) || len(c) == 0 {
				t.Fatalf("Text(%s) = %q counts=%v", tc.name, clean, c)
			}
			pctIn := "auth=Bearer%20" + tc.token + "&q=a%3D" + tc.token + "%26b=1"
			pctOut, cPct := redact.Text(s, pctIn)
			if strings.Contains(pctOut, tc.token) || strings.Contains(pctOut, tc.payload) || len(cPct) == 0 {
				t.Fatalf("Text(pct %s) leaked: %q", tc.name, pctOut)
			}
			if !strings.Contains(pctOut, "%20") || !strings.Contains(pctOut, "%3D") || !strings.Contains(pctOut, "%26") {
				t.Fatalf("Text(pct %s) corrupted percent-escapes: %q", tc.name, pctOut)
			}
			if again, c2 := redact.Text(s, pctOut); again != pctOut || len(c2) != 0 {
				t.Fatalf("not idempotent: %q %v", again, c2)
			}

			sess := archive.SessionRow{
				Summary:          "Bearer%20" + tc.token,
				FirstPrompt:      "q=a%3D" + tc.token + "%26b=1",
				FirstPromptShort: tc.token,
			}
			block := archive.BlockRow{
				Text:        "error: " + tc.token,
				RequestText: "q=" + tc.token + "%26" + tc.token,
			}
			chunk := archive.Row{Text: "token=" + tc.token}
			redact.Session(s, &sess)
			redact.Block(s, &block)
			redact.Chunk(s, &chunk)
			for _, field := range []string{sess.Summary, sess.FirstPrompt, sess.FirstPromptShort, block.Text, block.RequestText, chunk.Text} {
				if strings.Contains(field, tc.token) || strings.Contains(field, tc.payload) {
					t.Fatalf("%s leaked in row field: %q", tc.name, field)
				}
			}
		})
	}

	// Underscore-adjacent and back-to-back 32-char hev_/hvl_ tokens preserve underscores.
	hevClean, _ := redact.Text(s, hevToken)
	hvlClean, _ := redact.Text(s, hvlToken)
	for _, uc := range []struct{ in, want string }{
		{"X_" + hevToken + "_x", "X_" + hevClean + "_x"},
		{hevToken + "_" + hevToken, hevClean + "_" + hevClean},
		{hevToken + "_" + hvlToken, hevClean + "_" + hvlClean},
	} {
		if got, _ := redact.Text(s, uc.in); got != uc.want {
			t.Fatalf("Text(%q) = %q, want %q", uc.in, got, uc.want)
		}
	}

	// Permitted/preserved side across Text, Session, Block, and Chunk.
	kept := "hk_word hev_dispatch hev_cluster_agents_working my_hev_metric hev_abcdefghijklmnopqrstuvwxyz_total hev_ kit"
	if got, c := redact.Text(s, kept); got != kept || len(c) != 0 {
		t.Fatalf("Text mangled preserved identifiers: %q (%v)", got, c)
	}
	sess := archive.SessionRow{Summary: kept, FirstPrompt: kept, FirstPromptShort: kept}
	block := archive.BlockRow{Text: kept, RequestText: kept}
	chunk := archive.Row{Text: kept}
	if len(redact.Session(s, &sess)) != 0 || len(redact.Block(s, &block)) != 0 || len(redact.Chunk(s, &chunk)) != 0 {
		t.Fatalf("row scrubbers mangled preserved identifiers")
	}
	for _, field := range []string{sess.Summary, sess.FirstPrompt, sess.FirstPromptShort, block.Text, block.RequestText, chunk.Text} {
		if field != kept {
			t.Fatalf("row field changed: %q", field)
		}
	}
}
