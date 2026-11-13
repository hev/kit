package dispatch

import (
	"github.com/hev/kit/internal/secrets"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSecretsSummaryOwnerOnly(t *testing.T) {
	t.Setenv("HEV_SECRETS_POLICY", filepath.Join(t.TempDir(), "secrets-policy.json"))
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(t.TempDir(), "reads.log"))
	for _, allow := range [][]string{nil, {RepoCapability}} {
		if _, e := HandleGrant(Request{Action: "secrets-summary", Meta: Meta{Sender: "local:owner"}}, "member:guest", allow); e == nil || !strings.Contains(e.Error(), "owner action") {
			t.Fatalf("member result %v", e)
		}
	}
	secrets.LogReadProvider(time.Now(), "op", "worker", "cache", nil, "ok")
	r, e := Handle(Request{Action: "secrets-summary"})
	if e != nil {
		t.Fatal(e)
	}
	if len(r.SecretSummary) != 1 || r.SecretSummary[0].Caller != "worker" || r.SecretSummary[0].CacheHits != 1 {
		t.Fatalf("summary %+v", r.SecretSummary)
	}
	b, _ := os.ReadFile(os.Getenv("HEV_SECRETS_READ_LOG"))
	if strings.Contains(string(b), "op://") {
		t.Fatal("value or URI logged")
	}
}
