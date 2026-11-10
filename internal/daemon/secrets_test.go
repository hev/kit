package daemon

import (
	"context"
	"github.com/hev/kit/internal/secrets"
	"io"
	"log/slog"
	"os"
	"runtime"
	"testing"
)

func TestSecretsEndpointFailureIsNonfatal(t *testing.T) {
	home := t.TempDir()
	// Darwin Unix socket paths are limited to 104 bytes. Its default test
	// temporary directory can exceed that before the endpoint suffix is added.
	if runtime.GOOS == "darwin" {
		var err error
		home, err = os.MkdirTemp("/tmp", "hev-secrets-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(home) })
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	close, err := secrets.Serve(ctx, secrets.New())
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	// A second endpoint cannot start; Capture still receives a safe cleanup.
	cleanup := startSecrets(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cleanup()
}
