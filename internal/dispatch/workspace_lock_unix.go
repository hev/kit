//go:build !windows

package dispatch

import (
	"path/filepath"

	"github.com/hev/kit/internal/platformops"
)

func tryWorkspaceDispatchLock() (func(), error) {
	return platformops.LockPath(filepath.Join(Root(), ".lock"), true)
}
