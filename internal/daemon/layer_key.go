package daemon

import "github.com/hev/kit/internal/layerenv"

// LayerKey is the CLI resolution path. Daemon bootstrap uses primaryCredential.
func LayerKey() (string, error) { return layerenv.ReadKey() }
