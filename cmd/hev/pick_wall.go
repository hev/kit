package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hev/kit/internal/workspace"
)

type wallTick struct{}
type pickerWall struct {
	cfg                     workspace.Config
	context                 string
	width, height, selected int
	panes                   []pickerPane
	screens                 []string
	target                  string
}

func (m pickerWall) Init() tea.Cmd { return func() tea.Msg { return wallTick{} } }

func (m pickerWall) Update(event tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := event.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case wallTick:
		queuePickerRefresh()
		m.panes, m.screens = nil, nil
		live := map[string]string{}
		for _, pane := range livePickerPanes() {
			live[pane.Pane] = pane.ID
		}
		for _, pane := range readPickerSnapshot().Panes {
			if pane.Harness == "" || live[pane.Pane] != pane.ID || belongsToPickerWorkspace(m.cfg, pane) != m.context {
				continue
			}
			screen, _ := tmuxOutput("capture-pane", "-p", "-t", pane.Pane)
			m.panes = append(m.panes, pane)
			m.screens = append(m.screens, screen)
		}
		m.selected = min(m.selected, max(0, len(m.panes)-1))
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return wallTick{} })
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "ctrl-c":
			return m, tea.Quit
		case "tab", "right", "j", "down":
			m.selected = min(m.selected+1, max(0, len(m.panes)-1))
		case "shift+tab", "left", "k", "up":
			m.selected = max(0, m.selected-1)
		case "enter":
			if len(m.panes) > 0 {
				m.target = m.panes[m.selected].Pane
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m pickerWall) View() string {
	header := m.context + " agent wall · arrows select · Enter opens · q exits\n"
	if len(m.panes) == 0 {
		return header + "No live agents"
	}
	cols := max(1, min(3, m.width/32))
	cellWidth := max(8, m.width/cols-2)
	rows := max(1, (m.height-1)/7)
	pageSize := cols * rows
	first := m.selected / pageSize * pageSize
	var grid, row []string
	for i := first; i < min(first+pageSize, len(m.panes)); i++ {
		pane := m.panes[i]
		lines := strings.Split(m.screens[i], "\n")
		lines = lines[max(0, len(lines)-4):]
		color := lipgloss.Color("8")
		switch pane.State {
		case "working":
			color = "11"
		case "idle":
			color = "10"
		}
		prefix := " "
		if i == m.selected {
			prefix = ">"
		}
		title := fmt.Sprintf("%s %s · %s", prefix, pane.Name, pane.State)
		box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(color).Width(cellWidth).Height(5)
		row = append(row, box.Render(cleanPickerText(title)+"\n"+strings.Join(lines, "\n")))
		if len(row) == cols || i+1 == len(m.panes) {
			grid = append(grid, lipgloss.JoinHorizontal(lipgloss.Top, row...))
			row = nil
		}
	}
	return header + strings.Join(grid, "\n")
}

func openPickerWall(cfg workspace.Config, context string) error {
	result, err := tea.NewProgram(pickerWall{cfg: cfg, context: context}, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	if target := result.(pickerWall).target; target != "" {
		return attachPickerSession(target)
	}
	return nil
}
