package layerenv

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/workspace"
)

// WorkspaceKey resolves only the workspace's reference. Shared environment
// keys and the legacy Layer config are never consulted here.
func WorkspaceKey(w workspace.Selection) (string, error) {
	if w.Layer.KeyRef == "" {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	values, err := secrets.ResolveWith(ctx, []secrets.Ref{{Name: "HEVLAYER_API_KEY", URI: w.Layer.KeyRef, TTL: KeyTTL}}, secrets.Options{Caller: secrets.DefaultCaller("kit workspace layer")})
	key := strings.TrimSpace(values["HEVLAYER_API_KEY"])
	if err != nil || key == "" || key == PlaceholderKey {
		return "", fmt.Errorf("could not resolve Layer key for workspace %q", w.Name)
	}
	return key, nil
}
