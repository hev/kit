package daemon

import "github.com/hev/kit/internal/layerenv"

// LayerKey resolves the Layer API key from the environment or 1Password
// (layerenv.ReadKey).
func LayerKey() (string, error) { return layerenv.ReadKey() }
