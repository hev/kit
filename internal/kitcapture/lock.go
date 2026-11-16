package kitcapture

import "github.com/hev/kit/internal/platformops"

func lockFile(path string) (func(), error) { return platformops.LockPath(path, false) }
