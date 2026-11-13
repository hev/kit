package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/secrets"
)

func TestSecretsAttributionQueriesEveryHostWithoutValues(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "hosts.toml")
	if e := os.WriteFile(cfg, []byte("[hosts.alpha]\nssh='alpha.example'\n[hosts.beta]\nssh='beta.example'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	old := secretsAttributionRPC
	t.Cleanup(func() { secretsAttributionRPC = old })
	seen := map[string]bool{}
	secretsAttributionRPC = func(_ context.Context, h dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
		if q.Action != "secrets-summary" || q.Order != "" {
			t.Fatalf("unsafe request %+v", q)
		}
		seen[h.SSH] = true
		return dispatch.Response{SecretSummary: []secrets.SummaryRow{{Caller: "worker", Provider: "op", Requests: 7, CacheHits: 8, Throttles: 9}}}, nil
	}
	c, _, e := rootCmd.Find([]string{"secrets", "attribution"})
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Flags().Set("config", cfg); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	c.SetOut(&out)
	t.Cleanup(func() { c.SetOut(nil); _ = c.Flags().Set("config", "") })
	if e = c.RunE(c, nil); e != nil {
		t.Fatal(e)
	}
	if len(seen) != 2 {
		t.Fatalf("hosts queried %v", seen)
	}
	for _, n := range []string{"alpha", "beta", "worker", "op", "7", "8", "9"} {
		if !strings.Contains(out.String(), n) {
			t.Fatalf("missing %s in %s", n, &out)
		}
	}
	if strings.Contains(out.String(), "op://") {
		t.Fatal("reference exposed")
	}
	secretsAttributionRPC = func(context.Context, dispatch.Host, dispatch.Request) (dispatch.Response, error) {
		return dispatch.Response{}, fmt.Errorf("host unavailable")
	}
	out.Reset()
	if e = c.RunE(c, nil); e == nil || !strings.Contains(out.String(), "alpha") || !strings.Contains(out.String(), "beta") {
		t.Fatalf("missing host errors %v %s", e, &out)
	}
}
