package compat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/hev/kit/internal/hevpaths"
	"github.com/hev/kit/internal/workspace"
)

type Scope struct {
	Workspace  workspace.Selection
	Workspaces map[string]workspace.Workspace
	Enabled    bool
}

func (s Scope) AllowsLocal(row *Row) bool {
	if name := row.Dispatch; name != "" {
		var meta struct {
			Workspace string `json:"workspace"`
		}
		raw, err := os.ReadFile(filepath.Join(hevpaths.Dir(hevpaths.Orders), name, "meta.json"))
		if err == nil && json.Unmarshal(raw, &meta) == nil && meta.Workspace != "" {
			return meta.Workspace == s.Workspace.Name
		}
	}
	var owner string
	for name := range s.Workspaces {
		if len(name) > len(owner) && fohSession(name, row.Session) {
			owner = name
		}
	}
	if owner == "" {
		owner = (workspace.Config{Workspaces: s.Workspaces}).DirectoryOwner(row.Cwd, true)
	}
	if owner == "" {
		owner = workspace.Default
	}
	return owner == s.Workspace.Name
}

func fohSession(name, session string) bool {
	prefix := "foh-" + name
	return session == prefix || strings.HasPrefix(session, prefix+"-")
}
