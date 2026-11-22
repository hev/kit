// Package foh is kit's optional native front-of-house harness: a lean,
// YOLO-by-default agent loop routed through OpenRouter, a Bubble Tea
// meta-harness TUI and first-class hev toolchain tools. See RFC 0029.
package foh

import (
	"fmt"
	"strings"
)

// mascotPalette is the sprite's 16-color palette, indexed by hex digit.
var mascotPalette = [16][3]uint8{
	{0x00, 0x00, 0x00}, // 0
	{0x24, 0x17, 0x11}, // 1
	{0x30, 0x1e, 0x13}, // 2
	{0x34, 0x1e, 0x13}, // 3
	{0x39, 0x25, 0x18}, // 4
	{0x55, 0x37, 0x23}, // 5
	{0x56, 0x3b, 0x28}, // 6
	{0x5f, 0x3e, 0x27}, // 7
	{0x75, 0x47, 0x31}, // 8
	{0x51, 0x86, 0xa5}, // 9
	{0xeb, 0x57, 0x68}, // a
	{0xb3, 0x91, 0x7c}, // b
	{0xe3, 0xc2, 0xa1}, // c
	{0xfb, 0xe4, 0x76}, // d
	{0xf8, 0xdd, 0xae}, // e
	{0xff, 0xff, 0xff}, // f
}

// mascotSprite is the deal-with-it mascot, 40x40, verbatim from the order
// that introduced it. '.' is transparent; 0-f index mascotPalette.
const mascotSprite = `
................01..40..................
...............07600671.400.............
...........1111077506501067111..........
.........106660673555556665660..........
........2667556722375555336560001.......
......6006556555622355332233375571......
.....26502525335652265222563565570......
....20665652322355255555554255567001....
...1666775322227565555555555555455560...
...0353877265255536254355555553355560...
..62236772255533465533563535555255630...
.2655565325375520652012235535535556601..
.02244555252353202320000223224553537561.
..052255555223200210b0c00232265553565566
8b65523355222000c00beeec0002345272455570
0675526552200cccecceeeeeccc0323256233500
03654255330cceeeeeeeeeeeeee000223752530.
.1655535530ce88888eeee88888bc03556327528
66555233220e888777eeee877887e02337627440
0535356230000000001ee8000000100224623200
.155665330f0f000000000f0f00000003656320.
b4555532200f0f00000ee10ff000000024555228
023254223000f0f0000ee10f0f00000222333550
0235553bc0000f00000eec00f0f0000cc3222650
.155542cc0c0000001eeece000000c0cc335541.
.025422cc0ceeeeeeeeeeceeeeeeec0cc236720.
.0263238c08ceeeeeeeeecceeeeee80c8355720.
.0343525807cccceeccccbbeeeccc7072247430.
..102524307cccceb8888888ecccc502252411..
...02422207bbccb88888888bccbb70354220...
...0222220788cc8bccccccb8cc8870363320...
...e143220588888ceccceec8b8884134340f...
.....12221148887ce888cec88785132221f....
......11231158788b878bbb8775113321......
......cc1221147777877878775013211c......
.........001f14577777778541f100f........
.........ccefc1155777775310feee.........
......cbbcccbb0812555551800cccbbbbb.....
.....b999999990c86111118c80bbbb8888b....
....b9999999990ccc8bbbbccc0ddddaaaa8b...`

const (
	mascotSize    = 40
	transparentPx = -1
)

// mascotPixels parses the sprite into palette indexes (transparentPx for '.').
func mascotPixels() [][]int {
	lines := strings.Split(strings.TrimSpace(mascotSprite), "\n")
	px := make([][]int, len(lines))
	for y, line := range lines {
		px[y] = make([]int, len(line))
		for x, c := range line {
			switch {
			case c == '.':
				px[y][x] = transparentPx
			case c >= '0' && c <= '9':
				px[y][x] = int(c - '0')
			case c >= 'a' && c <= 'f':
				px[y][x] = int(c-'a') + 10
			default:
				panic(fmt.Sprintf("mascot: bad pixel %q", c))
			}
		}
	}
	return px
}

// downsample halves the grid: each 2x2 block becomes its most common opaque
// color, or transparent when fewer than two of its pixels are opaque.
func downsample(px [][]int) [][]int {
	out := make([][]int, len(px)/2)
	for y := range out {
		out[y] = make([]int, len(px[0])/2)
		for x := range out[y] {
			var counts [16]int
			opaque, best := 0, transparentPx
			for _, p := range []int{px[2*y][2*x], px[2*y][2*x+1], px[2*y+1][2*x], px[2*y+1][2*x+1]} {
				if p == transparentPx {
					continue
				}
				opaque++
				counts[p]++
				if best == transparentPx || counts[p] > counts[best] {
					best = p
				}
			}
			if opaque < 2 {
				best = transparentPx
			}
			out[y][x] = best
		}
	}
	return out
}

func rgb(p int) string {
	c := mascotPalette[p]
	return fmt.Sprintf("%d;%d;%d", c[0], c[1], c[2])
}

// halfBlocks renders two pixel rows per text row. Transparent pixels never
// paint a background: the cell uses the lower half-block, the upper
// half-block with the default background, or a space.
func halfBlocks(px [][]int) string {
	var b strings.Builder
	for y := 0; y+1 < len(px); y += 2 {
		for x := range px[y] {
			top, bot := px[y][x], px[y+1][x]
			switch {
			case top == transparentPx && bot == transparentPx:
				b.WriteString("\x1b[0m ")
			case bot == transparentPx:
				b.WriteString("\x1b[49;38;2;" + rgb(top) + "m▀")
			case top == transparentPx:
				b.WriteString("\x1b[49;38;2;" + rgb(bot) + "m▄")
			default:
				b.WriteString("\x1b[38;2;" + rgb(top) + ";48;2;" + rgb(bot) + "m▀")
			}
		}
		b.WriteString("\x1b[0m\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// RenderMascotTruecolor renders the mascot as 40 columns by 20 rows of
// 24-bit half-blocks.
func RenderMascotTruecolor() string { return halfBlocks(mascotPixels()) }

// RenderMascotCompact renders a 20 column by 10 row half-block mascot for
// terminals narrower than 90 columns.
func RenderMascotCompact() string { return halfBlocks(downsample(mascotPixels())) }

// asciiRamp runs from darkest to lightest pixel.
const asciiRamp = "@%#*+=-:."

// RenderMascotASCII renders 40 columns by 20 rows of plain ASCII with no
// escape sequences, for NO_COLOR and terminals without color.
func RenderMascotASCII() string {
	px := mascotPixels()
	var b strings.Builder
	for y := 0; y+1 < len(px); y += 2 {
		for x := range px[y] {
			sum, n := 0, 0
			for _, p := range []int{px[y][x], px[y+1][x]} {
				if p == transparentPx {
					continue
				}
				c := mascotPalette[p]
				sum += (299*int(c[0]) + 587*int(c[1]) + 114*int(c[2])) / 1000
				n++
			}
			if n == 0 {
				b.WriteByte(' ')
				continue
			}
			b.WriteByte(asciiRamp[(sum/n)*len(asciiRamp)/256])
		}
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// RenderMascot picks the renderer for a terminal: ASCII when color is off,
// compact under 90 columns, truecolor otherwise.
func RenderMascot(width int, color bool) string {
	switch {
	case !color:
		return RenderMascotASCII()
	case width > 0 && width < 90:
		return RenderMascotCompact()
	default:
		return RenderMascotTruecolor()
	}
}
