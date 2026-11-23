package foh

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hev/kit/internal/cluster"
)

// ChromeSnapshot holds the live cluster/host summary shown in the bottom
// status bar beneath the input rule (matching the operator's shell chrome).
type ChromeSnapshot struct {
	Members []cluster.Member
}

type chromeMsg struct {
	snap ChromeSnapshot
}

type chromeTickMsg struct{}

func chromeTick() time.Duration { return 15 * time.Second }

func loadChrome() ChromeSnapshot {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, err := cluster.Query(ctx, cluster.Request{Action: "ls"})
	if err != nil {
		return ChromeSnapshot{}
	}
	return ChromeSnapshot{Members: r.Members}
}

func osGlyph(goos string, color bool) string {
	if !color {
		return ""
	}
	switch strings.ToLower(goos) {
	case "darwin", "apple", "mac", "macos":
		return "\uf179 "
	case "linux":
		return "\uf17c "
	case "windows":
		return "\uf17a "
	default:
		return "\uf233 "
	}
}

var (
	chromeOnline  = lipgloss.NewStyle().Foreground(lipgloss.Color("#59C9A5"))
	chromeOffline = lipgloss.NewStyle().Foreground(lipgloss.Color("#F3AE35"))
	chromeName    = lipgloss.NewStyle().Foreground(lipgloss.Color("#E0DEF4"))
	chromeMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))
	ruleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#3B3E4F"))
)

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// renderChrome formats the cluster host badges in the same style as the
// operator's tmux status bar: `peers N  🍎 mini 0  🐧 oven 0  🐧 tower online  🪟 win-oven 0`.
func renderChrome(snap ChromeSnapshot, color bool) string {
	if len(snap.Members) == 0 {
		return ""
	}
	totalPeers := 0
	var parts []string
	for _, m := range snap.Members {
		if m.Activity != nil && !m.Activity.Stale {
			totalPeers += derefInt(m.Activity.AgentsWorking) + derefInt(m.Activity.AgentsIdle) + derefInt(m.Activity.HeadlessRuns) + derefInt(m.Activity.Orchestrators)
		}
		if m.Self {
			continue
		}
		alive := strings.EqualFold(m.State, "alive")
		status := "offline"
		if alive {
			if m.Activity != nil && !m.Activity.Stale {
				active := derefInt(m.Activity.HeadlessRuns) + derefInt(m.Activity.Orchestrators)
				status = fmt.Sprintf("%d", active)
			} else if m.Dispatch {
				status = "0"
			} else {
				status = "online"
			}
		}
		if !color {
			parts = append(parts, fmt.Sprintf("%s %s", m.Name, status))
			continue
		}
		iconStyle := chromeOnline
		if !alive {
			iconStyle = chromeOffline
		}
		parts = append(parts, iconStyle.Render(osGlyph(m.OS, true))+chromeName.Render(m.Name)+" "+chromeMuted.Render(status))
	}
	if len(parts) == 0 {
		return ""
	}
	prefix := fmt.Sprintf("peers %d", totalPeers)
	if color {
		prefix = chromeName.Render(prefix)
	}
	return prefix + "  " + strings.Join(parts, "  ")
}
