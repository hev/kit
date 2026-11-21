//go:build !darwin && !windows

package subscriptions

import (
	"context"
	"errors"
)

// systemKeychain: Claude Code keeps its login in a file off macOS.
func systemKeychain(context.Context) ([]byte, error) {
	return nil, errors.New("no keychain on this platform")
}

// systemAgyKeychain: headless Linux stores Antigravity OAuth state in
// ~/.gemini/antigravity-cli/antigravity-oauth-token; desktop keyrings are
// checked silently via `agy models` without reading secret values.
func systemAgyKeychain(context.Context) (bool, error) {
	return false, nil
}
