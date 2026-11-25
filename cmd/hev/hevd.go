package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// hevd, the capture daemon, drawn. `up` prints it once, with the summary,
// after every step has succeeded. It goes to a terminal only: piped output,
// logs and the tests see the tick lines and nothing else. The drawing is
// Adam's: the shade characters carry its tones, so it is not to be redrawn,
// rescaled or coloured by shade.
var hevdArt = []string{
	"                 █████████ ███",
	"            ██████▓▓███▓██████████",
	"         ████▓▓██████▓▓▓▓▓████▓▓█████",
	"      ███████████▓█████▓█████████▓▓▓██",
	"    ███▓▓█▓▓██████▓▓███▓▓██▓▓██▓▓▓▓▓████",
	"   █████▓▓▓██████▓██▓█████▓▓▓▓█▓▓███▓▓▓██",
	" ██████▓▓█████▓████▓████████████████▓▓████",
	" ███████▓▓████████████████████████▓████▓▓███",
	"██▓▓█████▓███████▓▒██▒░░░░▒██████████████▓███",
	"███▓▓███▓███▓▓▒░░░░░░░░░░░░░░░███████▓██████",
	"███▓▓███████▒░▒▓▓▓▓▓░░░░░▓▓▓▓▓▒░████████▓████",
	"██████▓▓█████████████▒░▒████████████████████",
	"███▓▓▓██████░▓░▒█████▒░▒█▓  ██████████▓▓█████",
	"████████▒▒████░▒█████░░░▓█▒░█ ████▓▒▓████▓███",
	" ███▓███▒▒█▒▒▒▒▒▒▒▒░░░░░░░░░░░░▒░█▒░▓██▓▓██",
	" ████████▒██▒░░░░░░░░░░░▒▒░░░░░░▓█▒▓██▓▓▓██",
	" ███████████▒░░░░░▒▒▓▓▓▓▓▓▒░░░░░▓██████████",
	"   █████████▓▓▒░▒▓▒▒▒▒▒▒▒▒▓▓░░▓▓▓████████",
	"    ▓████████▓▓▓▓▒░░▒▒▒░░░▒▓▓▓▓▓████████",
	"      ████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓█████████",
	"         ████████▓▓▓▓▓▓▓▓▓▓██████████",
	"       █████████████▓▓▓▓▓██████████████",
	"     ██▒▒▒▒▒▒▓▓█▓░░▒██████░░░█░░░░▒▒▒▒▓█",
}

const (
	// hevdArtWidth is the widest art line, in columns.
	hevdArtWidth = 45
	// hevdGutter separates the art from the text beside it.
	hevdGutter = 3
	// hevdFallbackWidth stands in when the terminal will not report its size.
	hevdFallbackWidth = 80
)

var (
	hevdStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("160"))
	leadStyle  = lipgloss.NewStyle().Bold(true)
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// hevdLine is one line of text beside the art: a label in the dim column and
// its value, or, with no label, a line on its own.
type hevdLine struct{ label, value string }

func archiveLine(endpoint, namespace string) hevdLine {
	return hevdLine{"archive", endpoint + " · " + namespace}
}

func (l hevdLine) render() string {
	if l.label == "" {
		return leadStyle.Render(l.value)
	}
	return labelStyle.Render(fmt.Sprintf("%-11s", l.label)) + l.value
}

// renderHevd lays the summary out for a terminal width columns wide.
//
// Where the art, the gutter and the longest text line fit side by side, the
// text sits in one column to the right of the art, centred vertically against
// it. Where they do not, the art comes first, then a blank line, then the text
// flush left, so nothing wraps mid-art. A terminal narrower than the art gets
// the text alone.
func renderHevd(lines []hevdLine, width int) string {
	text := make([]string, len(lines))
	textWidth := 0
	for i, l := range lines {
		text[i] = l.render()
		textWidth = max(textWidth, ansi.StringWidth(text[i]))
	}

	var b strings.Builder
	b.WriteString("\n")
	switch {
	case width < hevdArtWidth:
		for _, t := range text {
			b.WriteString(t + "\n")
		}
	case width < hevdArtWidth+hevdGutter+textWidth:
		for _, art := range hevdArt {
			b.WriteString(hevdStyle.Render(art) + "\n")
		}
		b.WriteString("\n")
		for _, t := range text {
			b.WriteString(t + "\n")
		}
	default:
		// Text taller than the art starts at the top and runs on below it.
		top := max(0, (len(hevdArt)-len(text))/2)
		for i := range max(len(hevdArt), top+len(text)) {
			art, artWidth := "", 0
			if i < len(hevdArt) {
				art, artWidth = hevdStyle.Render(hevdArt[i]), len([]rune(hevdArt[i]))
			}
			if j := i - top; j >= 0 && j < len(text) {
				art += strings.Repeat(" ", hevdArtWidth+hevdGutter-artWidth) + text[j]
			}
			b.WriteString(art + "\n")
		}
	}
	return b.String()
}

// printHevd writes the art to out when out is a terminal, laid out for that
// terminal's width.
func printHevd(out io.Writer, lines ...hevdLine) {
	f, ok := out.(*os.File)
	if !ok || !isTerminal(out) {
		return
	}
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil || width <= 0 {
		width = hevdFallbackWidth
	}
	io.WriteString(out, renderHevd(lines, width))
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
