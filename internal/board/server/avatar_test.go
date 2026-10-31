package server

import (
	"github.com/hev/kit/internal/board/settings"
	"testing"
)

func TestEveryAvatarIsEmbedded(t *testing.T) {
	if _, err := ui.ReadFile("ui/avatars/agent.svg"); err != nil {
		t.Fatal(err)
	}
}
func TestRoleOfTrustsConfiguredName(t *testing.T) {
	old := settings.Current
	t.Cleanup(func() { settings.Current = old })
	settings.Current.Roles = []settings.Role{{Pattern: "^coordinator-(.+)$", Role: "coordinator", Instance: "$1"}}
	if got := roleOf("coordinator-project", "human"); got != "coordinator" {
		t.Fatal(got)
	}
	if got := roleOf("guest", "human"); got != "human" {
		t.Fatal(got)
	}
	if got := initial("coordinator-project", "human"); got != "P" {
		t.Fatal(got)
	}
}
