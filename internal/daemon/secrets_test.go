package daemon

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/testenv"
)

func TestSecretsEndpointFailureIsNonfatal(t *testing.T) {
	home := testenv.SocketDir(t) // holds the default endpoint, ~/.hev/secrets/agent.sock
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
