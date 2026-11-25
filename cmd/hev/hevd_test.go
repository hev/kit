package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The lines `up` passes on its fullest path, with stand-in values where up
// reads them from the stack.
func upHevdLines() []hevdLine {
	return []hevdLine{
		{value: "hevd is up: sending traces and taking dispatched work on this machine."},
		{"dashboard", "http://127.0.0.1:8099"},
		{"search", `hev traces search "why did the preflight fail"`},
		{"dispatch", "hev dispatch ls"},
		archiveLine("http://127.0.0.1:8787", "hev-traces"),
		{"edition", "community"},
		{"stop", "hev down"},
	}
}

// plainHevd renders at width and returns the lines with the styling stripped
// and the blank line that leads the block dropped.
func plainHevd(t *testing.T, lines []hevdLine, width int) []string {
	t.Helper()
	out := ansi.Strip(renderHevd(lines, width))
	if !strings.HasPrefix(out, "\n") || !strings.HasSuffix(out, "\n") {
		t.Fatalf("width %d: want a leading blank line and a trailing newline: %q", width, out)
	}
	return strings.Split(out[1:len(out)-1], "\n")
}

func plainText(lines []hevdLine) []string {
	text := make([]string, len(lines))
	for i, l := range lines {
		text[i] = ansi.Strip(l.render())
	}
	return text
}

// The drawing is fixed: 23 lines, 45 columns at its widest, nothing trailing,
// and only block and shade characters. The checked-in copy under testdata is
// the drawing as it was ordered.
func TestHevdArtIsTheDrawing(t *testing.T) {
	if len(hevdArt) != 23 {
		t.Fatalf("art has %d lines, want 23", len(hevdArt))
	}
	widest := 0
	for i, l := range hevdArt {
		if strings.TrimRight(l, " \t") != l {
			t.Errorf("art line %d has trailing whitespace: %q", i, l)
		}
		if strings.Trim(l, " █▓▒░") != "" {
			t.Errorf("art line %d has a character outside the drawing's set: %q", i, l)
		}
		widest = max(widest, len([]rune(l)))
		if w := ansi.StringWidth(l); w != len([]rune(l)) {
			t.Errorf("art line %d: display width %d differs from rune count %d", i, w, len([]rune(l)))
		}
	}
	if widest != 45 {
		t.Fatalf("widest art line is %d runes, want 45", widest)
	}
	if hevdArtWidth != widest {
		t.Fatalf("hevdArtWidth is %d, the art is %d wide", hevdArtWidth, widest)
	}
	want, err := os.ReadFile("testdata/hevd.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(hevdArt, "\n") + "\n"; got != string(want) {
		t.Fatalf("hevdArt differs from testdata/hevd.txt:\n%s", got)
	}
}

// On a wide terminal the art is on the left, every text line starts in one
// column to its right, and the text block is centred against the art.
func TestRenderHevdWideSitsTextBesideArt(t *testing.T) {
	lines := upHevdLines()
	text := plainText(lines)
	got := plainHevd(t, lines, 160)
	if len(got) != len(hevdArt) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(hevdArt), strings.Join(got, "\n"))
	}
	col := hevdArtWidth + hevdGutter
	first, next := -1, 0
	for i, l := range got {
		if !strings.HasPrefix(l, hevdArt[i]) {
			t.Fatalf("line %d does not start with art line %d: %q", i, i, l)
		}
		if w := ansi.StringWidth(l); w > 160 {
			t.Errorf("line %d is %d columns wide", i, w)
		}
		r := []rune(l)
		if len(r) <= hevdArtWidth {
			if l != hevdArt[i] {
				t.Fatalf("line %d: art line with something after it: %q", i, l)
			}
			continue
		}
		if next == len(text) {
			t.Fatalf("line %d: text past the last text line: %q", i, l)
		}
		if strings.TrimSpace(string(r[len([]rune(hevdArt[i])):col])) != "" || string(r[col:]) != text[next] {
			t.Fatalf("line %d: want %q at column %d: %q", i, text[next], col, l)
		}
		if first < 0 {
			first = i
		} else if i != first+next {
			t.Fatalf("line %d: text block is not contiguous", i)
		}
		next++
	}
	if next != len(text) {
		t.Fatalf("%d of %d text lines rendered", next, len(text))
	}
	above, below := first, len(hevdArt)-first-len(text)
	if d := above - below; d < -1 || d > 1 {
		t.Fatalf("text block not centred: %d rows above, %d below", above, below)
	}
}

// Where the art and the text do not fit side by side, the whole art comes
// first, then a blank line, then the text flush left, and nothing is wider
// than the terminal.
func TestRenderHevdNarrowStacksTextUnderArt(t *testing.T) {
	lines := upHevdLines()
	want := append(append(append([]string{}, hevdArt...), ""), plainText(lines)...)
	got := plainHevd(t, lines, 80)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("stacked layout wrong:\n%s", strings.Join(got, "\n"))
	}
	for i, l := range got {
		if w := ansi.StringWidth(l); w > 80 {
			t.Errorf("line %d is %d columns wide: %q", i, w, l)
		}
	}
}

// The layout changes exactly where the art, the gutter and the longest text
// line stop fitting, and again where the art itself stops fitting.
func TestRenderHevdBreakpoints(t *testing.T) {
	lines := upHevdLines()
	longest := 0
	for _, l := range plainText(lines) {
		longest = max(longest, ansi.StringWidth(l))
	}
	fit := hevdArtWidth + hevdGutter + longest
	if got := plainHevd(t, lines, fit); len(got) != len(hevdArt) {
		t.Errorf("width %d: want text beside the art, got %d lines", fit, len(got))
	}
	for _, width := range []int{fit - 1, hevdArtWidth} {
		if got := plainHevd(t, lines, width); len(got) != len(hevdArt)+1+len(lines) || got[0] != hevdArt[0] {
			t.Errorf("width %d: want art above the text, got %d lines", width, len(got))
		}
	}
	if got := plainHevd(t, lines, hevdArtWidth-1); len(got) != len(lines) {
		t.Errorf("width %d: want text only, got %d lines", hevdArtWidth-1, len(got))
	}
}

// A terminal narrower than the art gets the text and no art.
func TestRenderHevdTooNarrowForArtIsTextOnly(t *testing.T) {
	lines := upHevdLines()
	got := plainHevd(t, lines, 40)
	if strings.Join(got, "\n") != strings.Join(plainText(lines), "\n") {
		t.Fatalf("want the text lines alone:\n%s", strings.Join(got, "\n"))
	}
	if strings.ContainsAny(strings.Join(got, ""), "█▓▒░") {
		t.Fatal("art printed to a terminal narrower than the art")
	}
}

// More text than the art has rows starts at the top and runs on below the art
// in the same column.
func TestRenderHevdTextTallerThanArt(t *testing.T) {
	var lines []hevdLine
	for range len(hevdArt) + 2 {
		lines = append(lines, hevdLine{"label", "value"})
	}
	got := plainHevd(t, lines, 160)
	if len(got) != len(lines) {
		t.Fatalf("got %d lines, want %d", len(got), len(lines))
	}
	for i, l := range got {
		r := []rune(l)
		if len(r) < hevdArtWidth+hevdGutter || !strings.HasPrefix(string(r[hevdArtWidth+hevdGutter:]), "label") {
			t.Fatalf("line %d: text not at column %d: %q", i, hevdArtWidth+hevdGutter, l)
		}
	}
}

// The drawing is one colour: no art line carries more than one colour
// sequence, and the pitchfork that came with the old art is gone.
func TestRenderHevdArtIsOneColour(t *testing.T) {
	out := renderHevd(upHevdLines(), 160)
	if strings.Contains(out, "ψ") {
		t.Fatal("the old pitchfork is still drawn")
	}
	for i, art := range hevdArt {
		if !strings.Contains(out, hevdStyle.Render(art)) {
			t.Fatalf("art line %d is not rendered whole in hevdStyle", i)
		}
	}
}

// A buffer is not a terminal, so up's output to anything but a TTY carries no art.
func TestPrintHevdSkipsNonTerminal(t *testing.T) {
	var out bytes.Buffer
	printHevd(&out, hevdLine{value: "hevd is up"})
	if out.Len() != 0 {
		t.Fatalf("printed to a buffer: %q", out.String())
	}
	// Nor is a regular file or a pipe, though both are *os.File.
	f, err := os.Create(t.TempDir() + "/out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	printHevd(f, hevdLine{value: "hevd is up"})
	if st, err := f.Stat(); err != nil || st.Size() != 0 {
		t.Fatalf("printed to a regular file: %v, %v", st, err)
	}
}

func TestHEVLayerArchiveLineNamesGateway(t *testing.T) {
	for _, endpoint := range []string{"http://127.0.0.1:8787", "https://gateway.example.net"} {
		line := archiveLine(endpoint, "archive-ns")
		if line.label != "archive" || line.value != endpoint+" · archive-ns" {
			t.Fatalf("archive must name gateway and namespace: %+v", line)
		}
		for _, width := range []int{160, 80, 40} {
			if got := renderHevd([]hevdLine{line}, width); !strings.Contains(got, endpoint+" · archive-ns") || strings.Contains(got, "<(°O°)>") || strings.Contains(got, "turbopuffer") {
				t.Fatalf("wrong archive rendering at width %d: %s", width, got)
			}
		}
	}
}
