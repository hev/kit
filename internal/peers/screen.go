package peers

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"github.com/hev/kit/internal/harness"
)

var ansi = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var sgr = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)
var dialog = regexp.MustCompile(`(?i)^\s*[❯›>]\s*1\.|Enter to confirm|Press enter to continue|Do you want to (proceed|make|create|allow)|Esc to cancel|allow this|permission|trust this|yes, proceed`)
var progress = regexp.MustCompile(`^. [\p{L}]+… \([0-9]+[smh]`)

// Dim input placeholders aren't user drafts. Preserve real text across SGR resets.
func undimmed(line string) string {
	var out strings.Builder
	dim, pos := false, 0
	for _, match := range sgr.FindAllStringSubmatchIndex(line, -1) {
		if !dim {
			out.WriteString(line[pos:match[0]])
		}
		codes := strings.FieldsFunc(line[match[2]:match[3]], func(r rune) bool { return r == ';' || r == ':' })
		if len(codes) == 0 {
			codes = []string{"0"}
		}
		for _, c := range codes {
			if c == "2" {
				dim = true
			}
			if c == "0" || c == "22" {
				dim = false
			}
		}
		pos = match[1]
	}
	if !dim {
		out.WriteString(line[pos:])
	}
	return ansi.ReplaceAllString(out.String(), "")
}

func (e *Engine) paneState(ctx context.Context, pane, agent string, headless bool) string {
	if agent == "" {
		return "-"
	}
	if headless {
		return "working"
	}
	if !harness.CanPaste(agent) {
		return "?"
	}
	screen, err := e.tmux(ctx, "capture-pane", "-e", "-p", "-t", pane)
	if err != nil {
		return "?"
	}
	return screenState(agent, screen)
}

// agy (Antigravity CLI) screens. Its running footer reads "esc to cancel", which
// on Claude and Codex closes a dialog, and its feedback survey takes one digit.
var agySurvey = regexp.MustCompile(`(?i)How's the CLI experience so far|\[1\]\s*Good\s+\[2\]\s*Fine\s+\[3\]\s*Bad`)
var agyCancel = regexp.MustCompile(`(?i)\besc to cancel\b`)
var agySpinner = regexp.MustCompile(`^[\x{2801}-\x{28FF}] \S`)
var agyDialog = regexp.MustCompile(`(?i)^\s*[❯›>]\s*1\.|Enter to confirm|Press enter to continue|Do you want to (proceed|make|create|allow)`)

func screenState(agent, screen string) string {
	raw, plain := []string{}, []string{}
	for _, line := range strings.Split(screen, "\n") {
		clean := strings.ReplaceAll(ansi.ReplaceAllString(line, ""), "\u00a0", " ")
		if strings.TrimSpace(clean) != "" {
			raw = append(raw, line)
			plain = append(plain, clean)
		}
	}
	if agent == harness.Agy {
		return agyState(raw, plain)
	}
	start := len(plain) - 12
	if start < 0 {
		start = 0
	}
	for _, line := range plain[start:] {
		if dialog.MatchString(line) {
			return "prompt"
		}
	}
	for i := len(plain) - 1; i >= 0 && i >= len(plain)-20; i-- {
		s := strings.TrimSpace(plain[i])
		if strings.HasPrefix(s, "❯") || strings.HasPrefix(s, "›") {
			typed := strings.TrimSpace(strings.ReplaceAll(undimmed(raw[i]), "\u00a0", " "))
			typed = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(typed, "❯"), "›"))
			if typed != "" {
				return "draft"
			}
			break
		}
	}
	for _, line := range plain[start:] {
		if working(line) {
			return "working"
		}
	}
	return "idle"
}

func working(line string) bool {
	return strings.Contains(strings.ToLower(line), "esc to interrupt") || progress.MatchString(strings.TrimLeftFunc(line, unicode.IsSpace))
}

// agyState reads the last 20 non-blank lines like the compat CLI: the editor
// is framed by the bottom two rules, and a screen without it is unknown.
func agyState(raw, plain []string) string {
	if n := len(plain) - 20; n > 0 {
		raw, plain = raw[n:], plain[n:]
	}
	start := len(plain) - 12
	if start < 0 {
		start = 0
	}
	inputStart, inputEnd := agyInput(plain)
	spinning := false
	for _, line := range plain[start:] {
		if agySurvey.MatchString(line) {
			return "prompt"
		}
		spinning = spinning || agySpinner.MatchString(strings.TrimLeftFunc(line, unicode.IsSpace))
	}
	running := inputStart >= 0 || spinning
	for i := start; i < len(plain); i++ {
		if i >= inputStart && i < inputEnd {
			continue
		}
		if agyDialog.MatchString(plain[i]) || !running && dialog.MatchString(plain[i]) {
			return "prompt"
		}
	}
	if inputStart >= 0 {
		typed := strings.TrimSpace(undimmed(raw[inputStart]))
		if strings.TrimSpace(strings.TrimPrefix(typed, ">")) != "" {
			return "draft"
		}
		for _, line := range raw[inputStart+1 : inputEnd] {
			if strings.TrimSpace(undimmed(line)) != "" {
				return "draft"
			}
		}
	}
	for i := start; i < len(plain); i++ {
		if working(plain[i]) || running && (i < inputStart || i >= inputEnd) && agyCancel.MatchString(plain[i]) {
			return "working"
		}
	}
	if spinning {
		return "working"
	}
	if inputStart < 0 {
		return "?"
	}
	return "idle"
}

// agyInput locates agy's editor between the bottom two frame rules.
func agyInput(lines []string) (start, end int) {
	end = -1
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.Trim(line, "─") != "" {
			continue
		}
		if end < 0 {
			end = i
			continue
		}
		if i+1 < end {
			prompt := strings.TrimLeftFunc(lines[i+1], unicode.IsSpace)
			if prompt == ">" || strings.HasPrefix(prompt, "> ") {
				return i + 1, end
			}
		}
		break
	}
	return -1, -1
}
