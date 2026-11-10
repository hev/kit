package daemon

import (
	"context"
	"github.com/hev/kit/internal/secrets"
	"io"
	"log/slog"
	"os"
	"testing"
)

func TestSecretsEndpointFailureIsNonfatal(t *testing.T) {
	// Unix socket paths must fit sockaddr_un; t.TempDir includes the long test name.
	home, err := os.MkdirTemp("", "hev-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
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
