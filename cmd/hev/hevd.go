package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// hevd, the capture daemon, drawn. `up` prints it once, beside the summary,
// after every step has succeeded. It goes to a terminal only: piped output,
// logs and the tests see the tick lines and nothing else.
var hevdArt = []string{
	"   ▄▀▄   ▄▀▄",
	"  ▐████████▌",
	"  ▐█ ▀  ▀ █▌",
	"   ▀██▄▄██▀ ψ",
	"    ▐█  █▌",
}

// hevdWidth is the column the text beside the art starts at: the widest art
// line plus a gutter.
const hevdWidth = 16

var (
	hevdStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("160"))
	forkStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	leadStyle  = lipgloss.NewStyle().Bold(true)
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	pufferMark = lipgloss.NewStyle().Foreground(lipgloss.Color("116")).Render("<(°O°)>")
)

// hevdLine is one line of text beside the art: a label in the dim column and
// its value, or, with no label, a line on its own.
type hevdLine struct{ label, value string }

// renderHevd lays the text out beside the art, one art line per text line,
// top-aligned. Extra text lines run on below the art in the same column.
func renderHevd(lines []hevdLine) string {
	var b strings.Builder
	b.WriteString("\n")
	n := max(len(hevdArt), len(lines))
	for i := range n {
		art := ""
		if i < len(hevdArt) {
			art = hevdArt[i]
		}
		pad := strings.Repeat(" ", max(0, hevdWidth-len([]rune(art))))
		if body, fork, ok := strings.Cut(art, "ψ"); ok {
			art = hevdStyle.Render(body) + forkStyle.Render("ψ") + fork
		} else {
			art = hevdStyle.Render(art)
		}
		text := ""
		if i < len(lines) {
			if l := lines[i]; l.label == "" {
				text = leadStyle.Render(l.value)
			} else {
				text = labelStyle.Render(fmt.Sprintf("%-11s", l.label)) + l.value
			}
		}
		b.WriteString(strings.TrimRight(art+pad+text, " ") + "\n")
	}
	return b.String()
}

// printHevd writes the art to out when out is a terminal.
func printHevd(out io.Writer, lines ...hevdLine) {
	if isTerminal(out) {
		io.WriteString(out, renderHevd(lines))
	}
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
