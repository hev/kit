package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/kitcapture"
	"github.com/hev/kit/pkg/kitapi"
)

// `hev d status` (and `hev login status`, which repeats it) says in one
// line that a 402 left the team read-only for billing, and mentions batches
// from before outboxes were bound to a team.
func TestKitNoticeBillingAndUnbound(t *testing.T) {
	// The kit API of a team read-only for billing (the kit API fake
	// imports this package, so a minimal one here).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/v1/whoami" {
			w.Write([]byte(`{"key_id":"k","team":{"id":"team_1","slug":"acme"}}`))
			return
		}
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"error":"billing_read_only"}`))
	}))
	defer srv.Close()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "00000000000000000001-000001-1-sessions.json"), []byte(`{"rows":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	o, err := kitcapture.OpenOutbox(root, "team_1")
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Enqueue(kitcapture.KindSessions, []byte(`{"rows":[]}`)); err != nil {
		t.Fatal(err)
	}
	d := &kitcapture.Drainer{Outbox: o, Client: kitapi.New(srv.URL, "hk_notice"), Interval: time.Hour, MaxBackoff: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { d.Run(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := d.Last(); kitapi.ReadOnly(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("drainer never saw the 402")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	n := kitNotice(d, o)
	for _, want := range []string{"read-only for billing", "billing_read_only", "1 batches kept queued", "1 batches queued before outboxes were bound to a team are not sent"} {
		if !strings.Contains(n, want) {
			t.Fatalf("notice %q lacks %q", n, want)
		}
	}
}
