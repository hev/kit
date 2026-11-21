package dispatch

import (
	"os"
	"path/filepath"
)

func tryWorkspaceDispatchLock() (func(), error) {
	path := filepath.Join(Root(), ".lockdir")
	if err := os.Mkdir(path, 0700); err != nil {
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}
