package foh

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var (
	hevCmdStyle   = lipgloss.NewStyle().Bold(true).Foreground(brandInk)
	hevBadgeStyle = lipgloss.NewStyle().Bold(true).Foreground(hlCyan)

	// hevCmdRe matches plain-text `hev <subcommand>...` tokens inside rendered
	// Markdown or tool lines so hev CLI invocations share one consistent style.
	hevCmdRe = regexp.MustCompile(`\bhev\s+(?:up|d|s|dispatch|orchestrate|steer|secrets|cluster|peers|board|traces|watch|workspace|foh|cron|service|preview|evidence|pro|review|skills|hooks|update|config|context|swarm|climb|login|status|down|stop)\b(?:[ \t]+(?:--?[a-zA-Z0-9_-]+|[a-zA-Z0-9_./:@-]+))*`)
)

type mdCacheKey struct {
	width int
	color bool
}

var (
	mdMu        sync.Mutex
	mdRenderers = map[mdCacheKey]*glamour.TermRenderer{}
)

func getMarkdownRenderer(width int, color bool) *glamour.TermRenderer {
	if width < 20 {
		width = 20
	}
	key := mdCacheKey{width: width, color: color}
	mdMu.Lock()
	defer mdMu.Unlock()
	if r, ok := mdRenderers[key]; ok {
		return r
	}
	// Cap cache size so terminal resizes never grow unbounded.
	if len(mdRenderers) >= 16 {
		clear(mdRenderers)
	}
	style := "notty"
	if color {
		style = "dark"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}
	mdRenderers[key] = r
	return r
}

// renderMarkdown formats assistant prose with Glamour and applies consistent
// hev-command styling. Falls back cleanly to wrapped plain text on error.
func renderMarkdown(text string, width int, color bool) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	r := getMarkdownRenderer(width, color)
	if r == nil {
		out := lipgloss.NewStyle().Width(width).Render(text)
		if color {
			return styleHevCommands(out)
		}
		return out
	}
	rendered, err := r.Render(trimmed)
	if err != nil {
		return lipgloss.NewStyle().Width(width).Render(text)
	}
	rendered = strings.Trim(rendered, "\n")
	if color {
		rendered = styleHevCommands(rendered)
	}
	return rendered
}

// styleHevCommands highlights `hev <subcommand> ...` invocations consistently
// across assistant prose and tool lines.
func styleHevCommands(s string) string {
	return hevCmdRe.ReplaceAllStringFunc(s, func(match string) string {
		return hevCmdStyle.Render(match)
	})
}

var (
	brailleFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	asciiFrames   = []string{"|", "/", "-", `\`}

	// alpineVerbs are the shimmering status phrases shown while hev kit is
	// thinking or working on a turn.
	alpineVerbs = []string{
		// Adam's originals
		"working on the crux…",
		"sticking the landing…",
		"fresh tracks in the trees…",
		"focusing on exit speed…",
		"making it lighter…",
		"figuring out what we actually need…",
		"don't bother me…",
		"getting coffee…",
		"getting to the top…",
		"bonking…",
		"hopefully not bonking…",
		"locked in…",
		"clicked…",
		"locked…",
		"vibing…",
		"checkout hevmind.com…",
		"normal people don't do this…",
		// Alpine / ski / climb additions
		"skinning up the ridge…",
		"reading the snowpack…",
		"clipping the chains…",
		"sending the pitch…",
		"checking the beta…",
		"shaving grams off the pack…",
		"waxing the bases…",
		"dropping into the chute…",
		"finding the fall line…",
		"chalking up…",
		"dialing the DIN…",
		"traversing the bowl…",
		"brewing at basecamp…",
		"spotting the line…",
		"earning the turns…",
	}
)

func pickAlpineVerb() string {
	return alpineVerbs[rand.IntN(len(alpineVerbs))]
}

// renderShimmer draws a sweeping highlight band across text for active
// thinking and tool-call states. Under color=false it returns plain text with
// an ASCII spinner.
func renderShimmer(text string, frame int, color bool) string {
	if !color {
		sp := asciiFrames[frame%len(asciiFrames)]
		return sp + " " + text
	}
	sp := lipgloss.NewStyle().Foreground(lipgloss.Color("#FBE476")).Bold(true).Render(brailleFrames[frame%len(brailleFrames)])
	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return sp
	}
	// Sweep peak across the string with a short pause between passes.
	span := n + 8
	peak := frame % span
	var b strings.Builder
	b.WriteString(sp)
	b.WriteByte(' ')
	for i, ch := range runes {
		dist := i - peak
		if dist < 0 {
			dist = -dist
		}
		var r, g, bl uint8
		switch {
		case dist == 0:
			r, g, bl = 255, 250, 220 // bright highlight
		case dist == 1:
			r, g, bl = 251, 228, 118 // gold (#fbe476)
		case dist == 2:
			r, g, bl = 235, 87, 104 // coral (#eb5768)
		case dist == 3:
			r, g, bl = 81, 134, 165 // steel (#5186a5)
		default:
			r, g, bl = 138, 138, 138 // dim (#8a8a8a)
		}
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm%c", r, g, bl, ch)
	}
	b.WriteString("\x1b[0m")
	return b.String()
}
