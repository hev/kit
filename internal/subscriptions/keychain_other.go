//go:build !darwin

package subscriptions

import (
	"context"
	"errors"
)

// systemKeychain: Claude Code keeps its login in a file off macOS.
func systemKeychain(context.Context) ([]byte, error) {
	return nil, errors.New("no keychain on this platform")
}
