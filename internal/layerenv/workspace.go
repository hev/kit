package layerenv

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/workspace"
)

// WorkspaceKey resolves only the workspace's reference. Shared environment
// keys and the legacy Layer config are never consulted here.
func WorkspaceKey(w workspace.Selection) (string, error) {
	ref := w.TraceSettings().KeyRef
	if ref == "" {
		return "", nil
	}
	var key string
	if env, ok := strings.CutPrefix(ref, "env://"); ok {
		key = os.Getenv(env)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		values, err := secrets.ResolveWith(ctx, []secrets.Ref{{Name: "HEVLAYER_API_KEY", URI: ref, TTL: KeyTTL}}, secrets.Options{Caller: secrets.DefaultCaller("kit workspace layer")})
		if err == nil {
			key = values["HEVLAYER_API_KEY"]
		}
	}
	key = strings.TrimSpace(key)
	if key == "" || key == PlaceholderKey {
		return "", fmt.Errorf("could not resolve Layer key for workspace %q", w.Name)
	}
	return key, nil
}
