package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
)

// VisitWorkspaceOrders visits the current metadata of locally owned orders.
// Readers of the dashboard don't need logs, reports, or prior attempts.
func VisitWorkspaceOrders(ctx context.Context, name string, visit func(string, Meta) bool) error {
	root, err := os.OpenRoot(Root())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	dirs, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dirs.Close()
	for {
		entries, readErr := dirs.ReadDir(128)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !entry.IsDir() || !ValidName(entry.Name()) {
				continue
			}
			var meta Meta
			if !readOrderMetadata(root, entry.Name(), &meta) || meta.Imported || workspaceOfRun(meta.WorkspaceName) != workspaceOfRun(name) {
				continue
			}
			if !visit(entry.Name(), meta) {
				return nil
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func readOrderMetadata(root *os.Root, name string, meta *Meta) bool {
	file, err := root.Open(name + "/meta.json")
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<10 {
		return false
	}
	return json.NewDecoder(io.LimitReader(file, 256<<10)).Decode(meta) == nil
}
