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
				"plain-text": fmt.Sprintf("request rejected for credential %s on node mini", tc.token),
				"json":       fmt.Sprintf(`{"error":{"message":"invalid key %s"},"token":"%s"}`, tc.token, tc.token),
				"cli-flag":   fmt.Sprintf(`curl -s --api-key=%s -H "Authorization: Bearer %s" https://kit.hevmind.com/v1/inference`, tc.token, tc.token),
				"url":        fmt.Sprintf("https://kit.hevmind.com/v1/inference?api_key=%s&debug=1", tc.token),
				"error":      fmt.Sprintf("POST /v1/inference/chat/completions: 401 Unauthorized (token=%s)", tc.token),
				"env":        fmt.Sprintf("HEV_API_KEY=%s HEVLAYER_API_KEY=%s", tc.token, tc.token),
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
		})
	}

	// Permitted/preserved side: snake_case metrics, tool names, env var names, and wordmark prose.
	preserved := []string{
		"hev_cluster_agents_working",
		"hev_dispatch_runs_total",
		"hev_dispatch",
		"hev_board_post",
		"HEV_API_KEY",
		"HEVLAYER_API_KEY",
		"hev_ kit",
		"Welcome to hev_ kit v0.7",
		"hev_cluster_agents_working{host=\"mini\"} 3",
		"hev_abcdefghijklmnopqrstuvwxyz_total",
		"hvl_abcdefghijklmnopqrstuvwxyz_metric",
		"set HEV_API_KEY or HEVLAYER_API_KEY before calling hev_dispatch and hev_board_post",
		"hk_short_id",
	}
	for _, keep := range preserved {
		out, counts := s.Text(keep)
		if out != keep || len(counts) != 0 {
			t.Errorf("permitted input %q was mangled to %q (counts=%v)", keep, out, counts)
		}
	}
}
