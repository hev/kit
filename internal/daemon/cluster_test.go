package daemon

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/secrets/fakeop"
)

func TestClusterKeyFailureKeepsSecretsServing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket fixture")
	}
	fake := fakeop.Install(t)
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-token")
	root := t.TempDir()
	t.Setenv(secrets.EndpointEnv, filepath.Join(root, "secrets", "agent.sock"))
	t.Setenv(cluster.EndpointEnv, filepath.Join(root, "cluster", "agent.sock"))
	t.Setenv("HEV_SECRETS_COOLDOWN", filepath.Join(root, "cooldown.json"))
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(root, "reads.jsonl"))
	t.Setenv("FAKE_OP_RATE_LIMIT", time.Now().Add(4*time.Hour).UTC().Format(time.RFC3339))
	cfg := filepath.Join(root, "config.toml")
	if e := os.WriteFile(cfg, []byte("[cluster]\nbind=\"127.0.0.1:0\"\nname=\"example\"\nkey_ref=\"op://test/item/key\"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HEV_CONFIG", cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := startSecrets(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer stop()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		r, e := cluster.Query(ctx, cluster.Request{Action: "status"})
		if e == nil && r.Status != nil && r.Status.Error != "" && !r.Status.KeyRetry.IsZero() {
			if r.Status.Running || r.Status.KeyLoaded {
				t.Fatal("cluster unexpectedly running")
			}
			break
		}
		select {
		case <-timer.C:
			t.Fatal("missing failed key status")
		case <-tick.C:
		}
	}
	if e := secrets.Ping(ctx); e != nil {
		t.Fatalf("secrets stopped with cluster: %v", e)
	}
	if fake.Count(t) != 1 {
		t.Fatalf("unexpected provider calls %d", fake.Count(t))
	}
}
