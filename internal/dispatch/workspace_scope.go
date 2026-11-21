package dispatch

import (
	"context"
	"fmt"
	"os"

	"github.com/hev/kit/internal/hostlabel"
	"github.com/hev/kit/internal/workspace"
)

// ScopeWorkspace filters before inventory or queue selection can contact a host.
func (c *Config) ScopeWorkspace(w workspace.Selection) {
	c.SelectedWorkspace = w
	for name, host := range c.Hosts {
		if !w.AllowsHost(name) || (host.Local && !w.AllowsHost(hostlabel.Self(os.Hostname))) {
			delete(c.Hosts, name)
		}
	}
	if !w.AllowsHost(c.DefaultHost) {
		c.DefaultHost = ""
	}
}

type workspaceContextKey struct{}

func WorkspaceContext(ctx context.Context, w workspace.Selection) context.Context {
	if w.Name == "" {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, workspaceContextKey{}, w.Name)
}

func workspaceOfRun(name string) string {
	if name == "" {
		return workspace.Default
	}
	return name
}

func checkRunWorkspace(selected string, m Meta) error {
	if selected != "" && selected != workspaceOfRun(m.WorkspaceName) {
		return fmt.Errorf("workspace boundary: run belongs to workspace %q", workspaceOfRun(m.WorkspaceName))
	}
	return nil
}
