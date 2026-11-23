package foh

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMascotSpriteShape(t *testing.T) {
	px := mascotPixels()
	if len(px) != mascotSize {
		t.Fatalf("rows = %d", len(px))
	}
	for y, row := range px {
		if len(row) != mascotSize {
			t.Fatalf("row %d has %d pixels", y, len(row))
		}
	}
	// Spot-check the verbatim sprite: first opaque pixels and the last row.
	if px[0][16] != 0 || px[0][17] != 1 || px[0][15] != transparentPx {
		t.Fatalf("row 0 = %v", px[0][14:20])
	}
	if got := strings.Split(strings.TrimSpace(mascotSprite), "\n")[39]; got != "....b9999999990ccc8bbbbccc0ddddaaaa8b..." {
		t.Fatalf("last row = %q", got)
	}
}

var sgr = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// cells splits a rendered row into (sgr, glyph) pairs.
func cells(t *testing.T, row string) [][2]string {
	t.Helper()
	var out [][2]string
	for len(row) > 0 {
		loc := sgr.FindStringIndex(row)
		if loc == nil || loc[0] != 0 {
			t.Fatalf("expected SGR before glyph in %q", row)
		}
		params := sgr.FindStringSubmatch(row)[1]
		row = row[loc[1]:]
		if row == "" {
			break // trailing reset
		}
		r := []rune(row)[0]
		out = append(out, [2]string{params, string(r)})
		row = row[len(string(r)):]
	}
	return out
}

func checkHalfBlocks(t *testing.T, out string, px [][]int, rows, cols int) {
	t.Helper()
	lines := strings.Split(out, "\n")
	if len(lines) != rows {
		t.Fatalf("rows = %d, want %d", len(lines), rows)
	}
	for y, line := range lines {
		if w := ansi.StringWidth(line); w != cols {
			t.Fatalf("row %d width = %d, want %d", y, w, cols)
		}
		cs := cells(t, line)
		if len(cs) != cols {
			t.Fatalf("row %d cells = %d", y, len(cs))
		}
		for x, c := range cs {
			top, bot := px[2*y][x], px[2*y+1][x]
			params, glyph := c[0], c[1]
			paintsBG := strings.Contains(params, "48;2;")
			switch {
			case top == transparentPx && bot == transparentPx:
				// Refused: any color on a fully transparent cell.
				if glyph != " " || paintsBG || strings.Contains(params, "38;2;") {
					t.Fatalf("(%d,%d) transparent cell = %q %q", x, y, params, glyph)
				}
			case top == transparentPx || bot == transparentPx:
				// Refused: an opaque background behind a transparent half.
				want := "▀"
				if top == transparentPx {
					want = "▄"
				}
				if glyph != want || paintsBG || !strings.HasPrefix(params, "49;38;2;") {
					t.Fatalf("(%d,%d) half-transparent cell = %q %q", x, y, params, glyph)
				}
			default:
				// Kept: both halves opaque paint fg (top) and bg (bottom).
				wantParams := "38;2;" + rgb(top) + ";48;2;" + rgb(bot)
				if glyph != "▀" || params != wantParams {
					t.Fatalf("(%d,%d) opaque cell = %q %q, want %q", x, y, params, glyph, wantParams)
				}
			}
		}
	}
}

func TestRenderMascotTruecolor(t *testing.T) {
	checkHalfBlocks(t, RenderMascotTruecolor(), mascotPixels(), 20, 40)
}

func TestRenderMascotCompact(t *testing.T) {
	checkHalfBlocks(t, RenderMascotCompact(), downsample(mascotPixels()), 10, 20)
}

func TestRenderMascotASCII(t *testing.T) {
	out := RenderMascotASCII()
	if strings.ContainsRune(out, '\x1b') {
		t.Fatal("ASCII mascot has escape sequences")
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 20 {
		t.Fatalf("rows = %d", len(lines))
	}
	px := mascotPixels()
	for y, l := range lines {
		if len(l) != 40 {
			t.Fatalf("row %d width %d", y, len(l))
		}
		for x, c := range l {
			if c > 127 {
				t.Fatalf("non-ASCII %q", c)
			}
			transparent := px[2*y][x] == transparentPx && px[2*y+1][x] == transparentPx
			if transparent != (c == ' ') {
				t.Fatalf("(%d,%d) = %q, transparent=%v", x, y, c, transparent)
			}
		}
	}
}

func TestRenderMascotSelection(t *testing.T) {
	if RenderMascot(200, false) != RenderMascotASCII() {
		t.Fatal("no color must use ASCII")
	}
	if RenderMascot(80, true) != RenderMascotCompact() {
		t.Fatal("narrow terminal must use compact")
	}
	if RenderMascot(90, true) != RenderMascotTruecolor() || RenderMascot(0, true) != RenderMascotTruecolor() {
		t.Fatal("wide or unknown width must use truecolor")
	}
}
