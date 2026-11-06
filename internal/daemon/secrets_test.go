package daemon

import (
	"context"
	"github.com/hev/kit/internal/secrets"
	"io"
	"log/slog"
	"testing"
)

func TestSecretsEndpointFailureIsNonfatal(t *testing.T) {
	home := t.TempDir()
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
