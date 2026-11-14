package daemon

import (
	"encoding/json"
	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/secrets"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFactsCollectionSilentAndReferenceUnknown(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "absent"))
	config := filepath.Join(home, "dispatch.toml")
	if err := os.WriteFile(config, []byte("[hosts]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", config)
	t.Setenv("HEVLAYER_API_KEY_REF", "op://private/reference")
	t.Setenv("HEVD_PROJECT_ALLOW", "fixture")
	captureHealth.Lock()
	captureHealth.layer = cluster.CredentialFact{}
	captureHealth.Unlock()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()
	cache := secrets.New()
	for i := 0; i < 2; i++ {
		if i == 1 {
			t.Setenv("HEVLAYER_API_KEY_REF", "")
			t.Setenv("HEVLAYER_API_KEY_OP_REF", "op://private/reference")
		}
		f := collectNodeFacts(cache, true, time.Now())
		if f.Credentials.Items["layer"].State != "unknown" {
			t.Error(f.Credentials)
		}
	}
	w.Close()
	os.Stderr = old
	b, _ := io.ReadAll(r)
	r.Close()
	if len(b) != 0 {
		t.Fatalf("collector emitted %q", b)
	}
}

func TestFactsTimesUTCAndMidSampleObservations(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.FixedZone("private", 3600))
	f := cluster.NodeFacts{Version: 1, At: now, Dispatch: &cluster.DispatchFacts{At: now}, Sessions: &cluster.SessionFacts{At: now}, Build: &cluster.BuildFacts{At: now, LastCheck: now.Add(time.Second), Outcome: "ok"}, Credentials: &cluster.CredentialFacts{At: now, Items: map[string]cluster.CredentialFact{"layer": {At: now, State: "ok"}}}, Capture: &cluster.CaptureFacts{At: now, LastUpload: now.Add(2 * time.Second)}}
	finalizeFacts(&f)
	if !f.At.Equal(now.Add(2 * time.Second)) {
		t.Fatal(f.At)
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "+01:00") {
		t.Fatal(string(raw))
	}
	if f.Build.LastCheck.Location() != time.UTC || f.Capture.LastUpload.Location() != time.UTC {
		t.Fatal("not UTC")
	}
}

func TestCaptureRunningMeansRecentCompletedCycle(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	o := captureObservation{interval: time.Minute}
	if o.recent(now) {
		t.Fatal("no cycle")
	}
	o.completed(now)
	if !o.recent(now.Add(3*time.Minute)) || o.recent(now.Add(3*time.Minute+time.Second)) {
		t.Fatal("freshness boundary")
	}
	if factsSampleInterval != 30*time.Second {
		t.Fatal(factsSampleInterval)
	}
}

func BenchmarkCollectNodeFacts(b *testing.B) {
	c := secrets.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collectNodeFacts(c, false, time.Now().UTC())
	}
}

func TestCredentialCooldownIsRateLimited(t *testing.T) {
	now := time.Now().UTC()
	f := credentialFacts(secrets.Status{Providers: []secrets.ProviderStatus{{Scheme: "op", Configured: true, LastAt: now, LastOutcome: "rate_limited"}}}, now)
	if f.Items["op"].State != "rate-limited" {
		t.Fatal(f)
	}
}
