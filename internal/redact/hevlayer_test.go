package redact

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestHEVLayerKeyRedaction(t *testing.T) {
	s, err := New(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	secret := "J7u9Bq2Lx5Nw8Rf0Vh3Cs6Ma1Zk4Dp9T"
	for _, name := range []string{"HEVLAYER_API_KEY", "LAYER_API_KEY", "TURBOPUFFER_API_KEY", "TPUF_API_KEY"} {
		text, counts := s.Text("export " + name + "=" + secret)
		if strings.Contains(text, secret) || len(counts) == 0 {
			t.Fatalf("%s was not scrubbed", name)
		}
	}
}

func TestKitAndLayerTokenRedactionBothSides(t *testing.T) {
	s, err := New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}

	// Construct synthetic tokens dynamically so static secret scanners do not flag test literals.
	hevPayload := strings.Repeat("a1B2", 8)
	hvlPayload := strings.Repeat("Z9y8", 8)
	hkPayload := strings.Repeat("Ab9_-1", 7) + "A"
	skOrPayload := strings.Repeat("deadbeef", 8)

	tokens := []struct {
		name    string
		token   string
		payload string
		ruleID  string
	}{
		{"hev-key", "hev_" + hevPayload, hevPayload, "layer-key"},
		{"hvl-key", "hvl_" + hvlPayload, hvlPayload, "layer-key"},
		{"hk-key", "hk_" + hkPayload, hkPayload, "kit-key"},
		{"hk-21-char", "hk_abCD0123_sample-value", "abCD0123_sample-value", "kit-key"},
		{"sk-or-v1", "sk-or-v1-" + skOrPayload, skOrPayload, "sk-token"},
	}

	// Refused/scrubbed side: standalone and embedded tokens across every context.
	for _, tc := range tokens {
		t.Run("scrub/"+tc.name, func(t *testing.T) {
			// Standalone must match the specific high-priority rule and be idempotent.
			clean, counts := s.Text(tc.token)
			if strings.Contains(clean, tc.token) || strings.Contains(clean, tc.payload) {
				t.Fatalf("standalone leaked: %q", clean)
			}
			if counts[tc.ruleID] != 1 {
				t.Fatalf("standalone rule counts = %v, want %s=1", counts, tc.ruleID)
			}
			again, c2 := s.Text(clean)
			if again != clean || len(c2) != 0 {
				t.Fatalf("not idempotent: %q %v", again, c2)
			}

			contexts := map[string]string{
				"plain-text":       fmt.Sprintf("request rejected for credential %s on node mini", tc.token),
				"json":             fmt.Sprintf(`{"error":{"message":"invalid key %s"},"token":"%s"}`, tc.token, tc.token),
				"cli-flag":         fmt.Sprintf(`curl -s --api-key=%s -H "Authorization: Bearer %s" https://kit.hevmind.com/v1/inference`, tc.token, tc.token),
				"url":              fmt.Sprintf("https://kit.hevmind.com/v1/inference?api_key=%s&debug=1", tc.token),
				"error":            fmt.Sprintf("POST /v1/inference/chat/completions: 401 Unauthorized (token=%s)", tc.token),
				"env":              fmt.Sprintf("HEV_API_KEY=%s HEVLAYER_API_KEY=%s", tc.token, tc.token),
				"pct-bearer":       "auth=Bearer%20" + tc.token,
				"pct-eq-amp":       "q=a%3D" + tc.token + "%26b=1",
				"pct-amp-only":     "GET /x?a=1%26" + tc.token,
				"pct-back-to-back": "q=" + tc.token + "%26" + tc.token,
			}
			for ctxName, input := range contexts {
				out, c := s.Text(input)
				if strings.Contains(out, tc.token) || strings.Contains(out, tc.payload) {
					t.Errorf("%s leaked %s: %q", ctxName, tc.name, out)
				}
				if len(c) == 0 || !strings.Contains(out, "[REDACTED:") {
					t.Errorf("%s had no redaction: %q counts=%v", ctxName, out, c)
				}
				re, c3 := s.Text(out)
				if re != out || len(c3) != 0 {
					t.Errorf("%s not idempotent: %q %v", ctxName, re, c3)
				}
			}
			// Percent-encoded prefix/suffix delimiters must stay intact and not pollute the HMAC fingerprint.
			pctOut, pctCounts := s.Text("q=a%3D" + tc.token + "%26b=1")
			if pctOut != "q=a%3D"+clean+"%26b=1" || pctCounts[tc.ruleID] != 1 {
				t.Errorf("pct delimiter/fingerprint mismatch: got %q (%v), want %q", pctOut, pctCounts, "q=a%3D"+clean+"%26b=1")
			}
		})
	}

	// Underscore-adjacent and back-to-back exact 32-char hev_/hvl_ tokens must scrub cleanly and idempotently.
	hevToken := "hev_" + hevPayload
	hvlToken := "hvl_" + hvlPayload
	hevClean, _ := s.Text(hevToken)
	hvlClean, _ := s.Text(hvlToken)
	underscoreCases := []struct {
		name string
		in   string
		want string
	}{
		{"prefix-underscore-hev", "X_" + hevToken, "X_" + hevClean},
		{"suffix-underscore-hev", hevToken + "_x", hevClean + "_x"},
		{"both-underscore-hev", "X_" + hevToken + "_x", "X_" + hevClean + "_x"},
		{"prefix-underscore-hvl", "X_" + hvlToken, "X_" + hvlClean},
		{"suffix-underscore-hvl", hvlToken + "_x", hvlClean + "_x"},
		{"back-to-back-hev", hevToken + "_" + hevToken, hevClean + "_" + hevClean},
		{"back-to-back-mixed", hevToken + "_" + hvlToken, hevClean + "_" + hvlClean},
		{"back-to-back-space", hevToken + " " + hvlToken, hevClean + " " + hvlClean},
	}
	for _, uc := range underscoreCases {
		t.Run("underscore/"+uc.name, func(t *testing.T) {
			out, c := s.Text(uc.in)
			if out != uc.want {
				t.Fatalf("Text(%q) = %q (counts=%v), want %q", uc.in, out, c, uc.want)
			}
			if strings.Contains(out, hevPayload) || strings.Contains(out, hvlPayload) {
				t.Fatalf("leaked payload in %q", out)
			}
			again, c2 := s.Text(out)
			if again != out || len(c2) != 0 {
				t.Fatalf("not idempotent: %q %v", again, c2)
			}
		})
	}

	// Permitted/preserved side: snake_case metrics, tool names, env var names, and wordmark prose.
	preserved := []string{
		"hev_cluster_agents_working",
		"hev_dispatch_runs_total",
		"hev_dispatch",
		"hev_board_post",
		"my_hev_metric",
		"HEV_API_KEY",
		"HEVLAYER_API_KEY",
		"hev_ kit",
		"Welcome to hev_ kit v0.7",
		"hev_cluster_agents_working{host=\"mini\"} 3",
		"hev_abcdefghijklmnopqrstuvwxyz_total",
		"hvl_abcdefghijklmnopqrstuvwxyz_metric",
		"hev_abcdefghijklmnopqrstuvwxyz_suffix",
		"foo%20hev_abcdefghijklmnopqrstuvwxyz_total",
		"foo%20hev_dispatch",
		"set HEV_API_KEY or HEVLAYER_API_KEY before calling hev_dispatch and hev_board_post",
		"hk_short_id",
		"hk_word",
	}
	for _, keep := range preserved {
		out, counts := s.Text(keep)
		if out != keep || len(counts) != 0 {
			t.Errorf("permitted input %q was mangled to %q (counts=%v)", keep, out, counts)
		}
	}
}

func TestHideKitKeysBothSides(t *testing.T) {
	hevPayload := strings.Repeat("a1B2", 8)
	hvlPayload := strings.Repeat("Z9y8", 8)
	hkPayload := strings.Repeat("Ab9_-1", 7) + "A"
	skOrPayload := strings.Repeat("deadbeef", 8)

	hevToken := "hev_" + hevPayload
	hvlToken := "hvl_" + hvlPayload
	hkToken := "hk_" + hkPayload
	hkShort21 := "hk_abCD0123_sample-value"
	skToken := "sk-or-v1-" + skOrPayload

	// Refused/scrubbed side without passing known key.
	for _, tc := range []struct {
		name, token, payload string
	}{
		{"hev", hevToken, hevPayload},
		{"hvl", hvlToken, hvlPayload},
		{"hk-43", hkToken, hkPayload},
		{"hk-21", hkShort21, "abCD0123_sample-value"},
		{"sk-or-v1", skToken, skOrPayload},
	} {
		t.Run("scrub/"+tc.name, func(t *testing.T) {
			for _, in := range []string{
				tc.token,
				"error with " + tc.token + " inside",
				`{"error":"bad ` + tc.token + `"}`,
				"auth=Bearer%20" + tc.token,
				"q=a%3D" + tc.token + "%26b=1",
				"q=" + tc.token + "%26" + tc.token,
			} {
				out := HideKitKeys(in, "")
				if strings.Contains(out, tc.token) || strings.Contains(out, tc.payload) {
					t.Errorf("HideKitKeys(%q) = %q leaked %s", in, out, tc.name)
				}
				if !strings.Contains(out, "[redacted]") {
					t.Errorf("HideKitKeys(%q) = %q missing [redacted]", in, out)
				}
				if HideKitKeys(out, "") != out {
					t.Errorf("HideKitKeys(%q) not idempotent: %q", in, out)
				}
			}
			if got := HideKitKeys("q=a%3D"+tc.token+"%26b=1", ""); got != "q=a%3D[redacted]%26b=1" {
				t.Errorf("pct delimiter mismatch for %s: got %q", tc.name, got)
			}
			if got := HideKitKeys("q="+tc.token+"%26"+tc.token, ""); got != "q=[redacted]%26[redacted]" {
				t.Errorf("pct back-to-back mismatch for %s: got %q", tc.name, got)
			}
		})
	}

	// Exact 32-char underscore-adjacent hev_/hvl_ tokens preserve surrounding underscores.
	for _, uc := range []struct{ in, want string }{
		{"X_" + hevToken, "X_[redacted]"},
		{hevToken + "_x", "[redacted]_x"},
		{"X_" + hevToken + "_x", "X_[redacted]_x"},
		{"X_" + hvlToken + "_y", "X_[redacted]_y"},
		{hevToken + "_" + hevToken, "[redacted]_[redacted]"},
		{hevToken + "_" + hvlToken, "[redacted]_[redacted]"},
	} {
		if got := HideKitKeys(uc.in, ""); got != uc.want {
			t.Errorf("HideKitKeys(%q) = %q, want %q", uc.in, got, uc.want)
		}
	}

	// Explicit known short legacy key is also replaced.
	if got := HideKitKeys("err hk_short123 and "+hevToken, "hk_short123"); got != "err [redacted] and [redacted]" {
		t.Errorf("known + pattern HideKitKeys = %q", got)
	}

	// Permitted/preserved side: snake_case identifiers, metrics, short hk_ words, and wordmark prose.
	for _, keep := range []string{
		"hk_word",
		"hk_short_id",
		"hev_dispatch",
		"hev_board_post",
		"hev_cluster_agents_working",
		"hev_dispatch_runs_total",
		"my_hev_metric",
		"hev_abcdefghijklmnopqrstuvwxyz_total",
		"hvl_abcdefghijklmnopqrstuvwxyz_metric",
		"hev_ kit",
		"Welcome to hev_ kit v0.7",
		"foo%20hev_dispatch",
	} {
		if got := HideKitKeys(keep, ""); got != keep {
			t.Errorf("HideKitKeys(%q) = %q, want unchanged", keep, got)
		}
	}
}
